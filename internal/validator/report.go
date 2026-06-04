package validator

import (
	"fmt"
	"sort"
	"strings"
)

// Severity classifies a finding.
type Severity string

const (
	// SeverityError marks a MUST/MUST NOT violation: the catalog is not
	// compliant with the targeted specification version.
	SeverityError Severity = "error"
	// SeverityWarning marks a SHOULD/SHOULD NOT deviation or a likely mistake
	// (such as an unrecognized field name) that does not by itself make the
	// catalog non-compliant.
	SeverityWarning Severity = "warning"
	// SeverityInfo carries advisory notes (such as a version-string alias).
	SeverityInfo Severity = "info"
)

// Finding is a single observation about the catalog.
type Finding struct {
	Severity Severity `json:"severity"`
	// Path is the dotted location within the catalog, e.g. "tracks[0].samplerate".
	Path string `json:"path,omitempty"`
	// Message is a human-readable description of the problem.
	Message string `json:"message"`
	// Rule references the relevant specification section, e.g. "MSF 5.2.28".
	Rule string `json:"rule,omitempty"`
}

// Report is the result of validating one catalog.
type Report struct {
	// Version is the catalog version string that was read from the document.
	Version string `json:"version"`
	// SchemaVersion is the schema actually used to validate (after alias
	// resolution), e.g. "draft-01".
	SchemaVersion string `json:"schemaVersion"`
	// Kind is either "catalog" (independent) or "deltaUpdate".
	Kind string `json:"kind"`
	// Valid is true when there are no error-severity findings.
	Valid    bool      `json:"valid"`
	Findings []Finding `json:"findings"`
}

func (r *Report) add(f Finding) { r.Findings = append(r.Findings, f) }

func (r *Report) addError(path, rule, format string, args ...any) {
	r.add(Finding{Severity: SeverityError, Path: path, Rule: rule, Message: fmt.Sprintf(format, args...)})
}

func (r *Report) addWarning(path, rule, format string, args ...any) {
	r.add(Finding{Severity: SeverityWarning, Path: path, Rule: rule, Message: fmt.Sprintf(format, args...)})
}

func (r *Report) addInfo(path, rule, format string, args ...any) {
	r.add(Finding{Severity: SeverityInfo, Path: path, Rule: rule, Message: fmt.Sprintf(format, args...)})
}

// Counts returns the number of findings at each severity.
func (r *Report) Counts() (errors, warnings, infos int) {
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			infos++
		}
	}
	return
}

// finalize sorts findings (errors first, then by path) and sets Valid.
func (r *Report) finalize() {
	r.dedupe()
	sevRank := map[Severity]int{SeverityError: 0, SeverityWarning: 1, SeverityInfo: 2}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		fi, fj := r.Findings[i], r.Findings[j]
		if sevRank[fi.Severity] != sevRank[fj.Severity] {
			return sevRank[fi.Severity] < sevRank[fj.Severity]
		}
		if fi.Path != fj.Path {
			return fi.Path < fj.Path
		}
		return fi.Message < fj.Message
	})
	errs, _, _ := r.Counts()
	r.Valid = errs == 0
}

// dedupe removes findings that share a severity, path and message. CUE
// whole-document and per-track validation can surface the same problem twice.
func (r *Report) dedupe() {
	seen := make(map[string]struct{}, len(r.Findings))
	out := r.Findings[:0]
	for _, f := range r.Findings {
		key := string(f.Severity) + "\x00" + f.Path + "\x00" + f.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	r.Findings = out
}

// Text renders the report as a human-readable report.
func (r *Report) Text() string {
	var b strings.Builder
	errs, warns, infos := r.Counts()

	status := "COMPLIANT"
	if !r.Valid {
		status = "NOT COMPLIANT"
	}
	fmt.Fprintf(&b, "MSF/CMSF catalog validation report\n")
	fmt.Fprintf(&b, "  document version : %s\n", orDash(r.Version))
	fmt.Fprintf(&b, "  validated against: %s\n", orDash(r.SchemaVersion))
	fmt.Fprintf(&b, "  document kind    : %s\n", orDash(r.Kind))
	fmt.Fprintf(&b, "  result           : %s (%d error(s), %d warning(s), %d info)\n", status, errs, warns, infos)

	if len(r.Findings) == 0 {
		fmt.Fprintf(&b, "\nNo issues found.\n")
		return b.String()
	}

	b.WriteString("\n")
	for _, f := range r.Findings {
		marker := map[Severity]string{
			SeverityError:   "ERROR  ",
			SeverityWarning: "WARN   ",
			SeverityInfo:    "INFO   ",
		}[f.Severity]
		fmt.Fprintf(&b, "%s %s\n", marker, f.Message)
		if f.Path != "" {
			fmt.Fprintf(&b, "        at: %s\n", f.Path)
		}
		if f.Rule != "" {
			fmt.Fprintf(&b, "        ref: %s\n", f.Rule)
		}
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
