// Package validator validates MSF (draft-ietf-moq-msf) and CMSF
// (draft-ietf-moq-cmsf) catalogs against version-specific CUE schemas and
// reports, field by field, which parts are not compliant.
//
// Validation has two layers:
//
//   - A CUE schema enforces structure: required fields, value types, allowed
//     enum values, and conditional requirements (for example, an audio track
//     MUST carry samplerate and channelConfig).
//   - A set of Go semantic checks cover rules that CUE expresses poorly:
//     cross-references (initRef -> initDataList, contentProtectionRefIDs ->
//     contentProtections), mutually-exclusive fields, unique track names, and
//     a handful of SHOULD-level recommendations surfaced as warnings.
package validator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"

	"github.com/Eyevinn/msf-catalog-validator/schemas"
)

// Engine validates catalogs. It is safe for concurrent use after construction.
type Engine struct {
	ctx     *cue.Context
	schemas map[string]*schemaSet // canonical schema version -> compiled schema
	aliases map[string]string     // document version string -> canonical schema version
}

// schemaSet holds the compiled CUE values for one schema version.
type schemaSet struct {
	version string
	root    cue.Value // #Catalog  (independent catalog)
	delta   cue.Value // #DeltaCatalog
	track   cue.Value // #Track    (used for per-track validation)
}

// defaultSchemaVersion is used for delta updates, which carry no version field
// of their own, and as the fallback when callers force a version.
const defaultSchemaVersion = "draft-01"

// Recurring field names, factored out to keep references consistent.
const (
	fieldVersion       = "version"
	fieldTracks        = "tracks"
	fieldPublishTracks = "publishTracks"
	fieldDeltaUpdate   = "deltaUpdate"

	ruleInitData = "MSF 5.1.7"

	fieldInitRef = "initRef"
	fieldCPRefs  = "contentProtectionRefIDs"
)

// New builds an Engine with all embedded schemas compiled. It returns an error
// if an embedded schema fails to compile (which would be a build-time bug).
func New() (*Engine, error) {
	ctx := cuecontext.New()
	e := &Engine{
		ctx:     ctx,
		schemas: map[string]*schemaSet{},
		aliases: map[string]string{
			defaultSchemaVersion: defaultSchemaVersion,
			// Every example in draft-01 uses "version": "1". Accept it as an
			// alias so real-world catalogs validate, while still recommending
			// the "draft-XX" convention from MSF Section 5.1.1.
			"1": defaultSchemaVersion,
		},
	}

	ss, err := compileSchema(ctx, defaultSchemaVersion, schemas.Draft01)
	if err != nil {
		return nil, fmt.Errorf("compiling %s schema: %w", defaultSchemaVersion, err)
	}
	e.schemas[defaultSchemaVersion] = ss

	return e, nil
}

func compileSchema(ctx *cue.Context, version, src string) (*schemaSet, error) {
	v := ctx.CompileString(src, cue.Filename(version+"/catalog.cue"))
	if err := v.Err(); err != nil {
		return nil, err
	}
	ss := &schemaSet{
		version: version,
		root:    v.LookupPath(cue.ParsePath("#Catalog")),
		delta:   v.LookupPath(cue.ParsePath("#DeltaCatalog")),
		track:   v.LookupPath(cue.ParsePath("#Track")),
	}
	for name, val := range map[string]cue.Value{"#Catalog": ss.root, "#DeltaCatalog": ss.delta, "#Track": ss.track} {
		if err := val.Err(); err != nil {
			return nil, fmt.Errorf("looking up %s: %w", name, err)
		}
	}
	return ss, nil
}

// Validate parses and validates a single catalog document. The returned Report
// is always non-nil; a non-nil error is returned only when the input cannot be
// parsed as JSON at all (in which case the Report records the parse error too).
//
// If forceVersion is non-empty it overrides the version dispatch and the
// catalog is validated against that schema version regardless of its own
// version string.
func (e *Engine) Validate(data []byte, forceVersion string) (*Report, error) {
	report := &Report{Findings: []Finding{}}

	// Parse once into a generic structure for version dispatch and for the Go
	// semantic checks.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		report.addError("", "", "document is not valid JSON: %v", err)
		report.finalize()
		return report, fmt.Errorf("parsing JSON: %w", err)
	}

	docVersion, _ := raw["version"].(string)
	report.Version = docVersion

	_, isDelta := raw[fieldDeltaUpdate]
	if isDelta {
		report.Kind = fieldDeltaUpdate
	} else {
		report.Kind = "catalog"
	}

	// Resolve which schema version to validate against.
	schemaVersion, ok := e.resolveSchema(report, forceVersion, docVersion, isDelta)
	if !ok {
		report.finalize()
		return report, nil
	}
	report.SchemaVersion = schemaVersion
	ss := e.schemas[schemaVersion]

	// Build a CUE value from the input JSON, preserving positions for messages.
	expr, err := cuejson.Extract("catalog.json", data)
	if err != nil {
		report.addError("", "", "could not read JSON document: %v", err)
		report.finalize()
		return report, nil
	}
	dataVal := e.ctx.BuildExpr(expr)

	// Layer 1: CUE structural validation.
	e.runCUE(report, ss, dataVal, isDelta)

	// Layer 2: Go semantic checks.
	runSemanticChecks(report, raw, isDelta)

	report.finalize()
	return report, nil
}

// resolveSchema picks the schema version. It records an info/error finding as
// appropriate and returns ok=false when validation cannot proceed.
func (e *Engine) resolveSchema(report *Report, forceVersion, docVersion string, isDelta bool) (string, bool) {
	if forceVersion != "" {
		if _, known := e.schemas[forceVersion]; !known {
			report.addError("", "", "forced schema version %q is not supported (known: %s)", forceVersion, e.knownVersions())
			return "", false
		}
		return forceVersion, true
	}

	if isDelta {
		// Delta updates carry no version of their own (MSF Section 5.3). If one
		// is (wrongly) present and known, validate against it anyway; the
		// forbidden-field rule is reported by the semantic checks.
		if docVersion != "" {
			if canonical, ok := e.aliases[docVersion]; ok {
				return canonical, true
			}
		}
		report.addInfo("", "MSF 5.3", "delta update has no version field; validating against %q", defaultSchemaVersion)
		return defaultSchemaVersion, true
	}

	if docVersion == "" {
		report.addError(fieldVersion, "MSF 5.1.1", "missing required \"version\" field; cannot select a schema")
		return "", false
	}

	canonical, ok := e.aliases[docVersion]
	if !ok {
		report.addError(fieldVersion, "MSF 5.1.1", "unsupported catalog version %q (supported: %s)", docVersion, e.knownVersions())
		return "", false
	}
	if canonical != docVersion {
		report.addInfo(fieldVersion, "MSF 5.1.1",
			"version %q is treated as %q; the recommended form is the \"draft-XX\" convention", docVersion, canonical)
	}
	return canonical, true
}

func (e *Engine) knownVersions() string {
	keys := make([]string, 0, len(e.aliases))
	for k := range e.aliases {
		keys = append(keys, strconv.Quote(k))
	}
	return strings.Join(keys, ", ")
}

// runCUE validates the document against the schema, then validates each track
// individually so that one catastrophic error (such as an invalid enum value)
// cannot mask required-field errors on sibling tracks.
func (e *Engine) runCUE(report *Report, ss *schemaSet, dataVal cue.Value, isDelta bool) {
	schemaVal := ss.root
	if isDelta {
		schemaVal = ss.delta
	}

	unified := schemaVal.Unify(dataVal)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		e.addCUEErrors(report, "", err)
	}

	if isDelta {
		// Per-op track validation mirrors the independent-catalog case below.
		e.validateDeltaTracks(report, ss, dataVal)
		return
	}

	// Independent catalog: validate each declared track in isolation.
	for _, field := range []string{fieldTracks, fieldPublishTracks} {
		list := dataVal.LookupPath(cue.ParsePath(field))
		if !list.Exists() {
			continue
		}
		e.validateTrackList(report, ss.track, list, field)
	}
}

func (e *Engine) validateTrackList(report *Report, trackSchema, list cue.Value, prefix string) {
	iter, err := list.List()
	if err != nil {
		return
	}
	for i := 0; iter.Next(); i++ {
		elem := iter.Value()
		unified := trackSchema.Unify(elem)
		if verr := unified.Validate(cue.Concrete(true)); verr != nil {
			e.addCUEErrors(report, fmt.Sprintf("%s[%d]", prefix, i), verr)
		}
	}
}

func (e *Engine) validateDeltaTracks(report *Report, ss *schemaSet, dataVal cue.Value) {
	ops := dataVal.LookupPath(cue.ParsePath(fieldDeltaUpdate))
	iter, err := ops.List()
	if err != nil {
		return
	}
	for i := 0; iter.Next(); i++ {
		op := iter.Value()
		opName, _ := op.LookupPath(cue.ParsePath("op")).String()
		// Only "add" tracks are full track declarations worth per-track checks.
		if opName != "add" {
			continue
		}
		tracks := op.LookupPath(cue.ParsePath("tracks"))
		if tracks.Exists() {
			e.validateTrackList(report, ss.track, tracks, fmt.Sprintf("deltaUpdate[%d].tracks", i))
		}
	}
}

// addCUEErrors converts CUE validation errors into findings, collapsing the
// noisy multi-line "empty disjunction" output that enum mismatches produce.
func (e *Engine) addCUEErrors(report *Report, pathPrefix string, err error) {
	for _, ce := range cueerrors.Errors(err) {
		path := joinPath(pathPrefix, ce.Path())
		rule := ruleForPath(path)

		// Reference fields (initRef, contentProtectionRefIDs) are constrained in
		// the schema to the ids declared in the catalog. A failure there means
		// the reference is dangling; give it a clear message regardless of the
		// underlying CUE error shape (disjunction mismatch or empty-set error).
		if msg, ok := referenceMessage(path); ok {
			report.add(Finding{Severity: SeverityError, Path: path, Rule: rule, Message: msg})
			continue
		}

		msg, enum := cueMessage(ce)
		if enum {
			report.add(Finding{Severity: SeverityError, Path: path, Rule: rule,
				Message: "value is not permitted here (does not match any allowed value for this field)"})
			continue
		}
		report.add(Finding{Severity: SeverityError, Path: path, Rule: rule, Message: msg})
	}
}

// referenceMessage returns a tailored message for catalog reference fields whose
// value does not resolve to a declared id, and ok=true when path is such a field.
func referenceMessage(path string) (msg string, ok bool) {
	switch leafField(path) {
	case fieldInitRef:
		return "initRef does not resolve to any id declared in initDataList", true
	case fieldCPRefs:
		return "contentProtectionRefIDs entry does not resolve to any refID declared in contentProtections", true
	}
	return "", false
}

// cueMessage renders a CUE error's message without position noise and flags the
// disjunction/enum case so the caller can emit a single clean finding.
func cueMessage(ce cueerrors.Error) (msg string, isEnum bool) {
	format, args := ce.Msg()
	m := fmt.Sprintf(format, args...)
	if strings.Contains(m, "empty disjunction") || strings.HasPrefix(m, "conflicting values") {
		return m, true
	}
	return m, false
}

// joinPath converts a CUE path (where list indices appear as their own
// segments) into bracketed notation: ["tracks","0","samplerate"] becomes
// "tracks[0].samplerate". A non-empty prefix is prepended.
func joinPath(prefix string, segs []string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for _, s := range segs {
		if strings.HasPrefix(s, "#") {
			// Skip CUE definition selectors (e.g. "#Catalog", "#Track") that
			// appear in paths when validating against a definition.
			continue
		}
		if isIndex(s) {
			b.WriteString("[")
			b.WriteString(s)
			b.WriteString("]")
			continue
		}
		if b.Len() > 0 {
			b.WriteString(".")
		}
		b.WriteString(s)
	}
	return b.String()
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
