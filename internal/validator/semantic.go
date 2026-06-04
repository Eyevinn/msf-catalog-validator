package validator

import (
	"fmt"
	"strings"
)

// ruleSections maps a leaf field name to its specification reference. It is
// used both to annotate CUE findings and the Go semantic findings below.
var ruleSections = map[string]string{
	// Root catalog fields.
	"version":            "MSF 5.1.1",
	"generatedAt":        "MSF 5.1.2",
	"isComplete":         "MSF 5.1.3",
	"tracks":             "MSF 5.1.4",
	"publishTracks":      "MSF 5.1.5",
	fieldDeltaUpdate:     "MSF 5.3",
	"op":                 "MSF 5.1.6",
	"initDataList":       ruleInitData,
	"contentProtections": "CMSF 4.1.1",
	// Track fields.
	"name":             "MSF 5.2.3",
	"namespace":        "MSF 5.2.2",
	"packaging":        "MSF 5.2.4",
	"eventType":        "MSF 5.2.5",
	"role":             "MSF 5.2.6",
	"isLive":           "MSF 5.2.7",
	"targetLatency":    "MSF 5.2.8",
	"buffers":          "MSF 5.2.9",
	fieldInitRef:       "MSF 5.2.13",
	"codec":            "MSF 5.2.18",
	"framerate":        "MSF 5.2.20",
	"bitrate":          "MSF 5.2.22",
	"width":            "MSF 5.2.26",
	"height":           "MSF 5.2.27",
	"samplerate":       "MSF 5.2.28",
	"channelConfig":    "MSF 5.2.29",
	"lang":             "MSF 5.2.32",
	"trackDuration":    "MSF 5.2.35",
	"encryptionScheme": "MSF 5.2.38",
	"cipherSuite":      "MSF 5.2.39",
	// InitData entry fields.
	"id":   ruleInitData,
	"type": ruleInitData,
	"data": ruleInitData,
	// CMSF fields.
	"maxGrpSapStartingType": "CMSF 3.5.2.1",
	"maxObjSapStartingType": "CMSF 3.5.2.2",
	fieldCPRefs:             "CMSF 4.1.2",
	"refID":                 "CMSF 4.1.1.1",
	"defaultKID":            "CMSF 4.1.1.2",
	"scheme":                "CMSF 4.1.1.3",
	"drmSystem":             "CMSF 4.1.1.4",
	"systemID":              "CMSF 4.1.1.4.1",
}

// leafField returns the trailing field name of a dotted path, stripping any
// trailing index, e.g. "tracks[0].contentProtectionRefIDs[1]" -> "contentProtectionRefIDs".
func leafField(path string) string {
	leaf := path
	if i := strings.LastIndexByte(leaf, '.'); i >= 0 {
		leaf = leaf[i+1:]
	}
	if i := strings.IndexByte(leaf, '['); i >= 0 {
		leaf = leaf[:i]
	}
	return leaf
}

// ruleForPath returns the spec reference for the leaf field of a dotted path,
// e.g. "tracks[0].samplerate" -> "MSF 5.2.28".
func ruleForPath(path string) string {
	return ruleSections[leafField(path)]
}

// rootFields is the set of recognized root-level catalog fields.
var rootFields = map[string]bool{
	"version": true, "generatedAt": true, "isComplete": true, "tracks": true,
	"publishTracks": true, "initDataList": true, "contentProtections": true,
	"deltaUpdate": true,
}

// trackFields is the set of recognized track-level fields (MSF + CMSF).
var trackFields = map[string]bool{
	"name": true, "namespace": true, "packaging": true, "role": true,
	"eventType": true, "isLive": true, "targetLatency": true, "buffers": true,
	"label": true, "renderGroup": true, "altGroup": true, "initRef": true,
	"depends": true, "temporalId": true, "spatialId": true, "codec": true,
	"mimeType": true, "mimetype": true, "framerate": true, "timescale": true,
	"bitrate": true, "avgBitrate": true, "maxGopDuration": true,
	"maxGroupDuration": true, "width": true, "height": true, "samplerate": true,
	"channelConfig": true, "displayWidth": true, "displayHeight": true,
	"lang": true, "parentName": true, "parentNamespace": true,
	"trackDuration": true, "connectionURI": true, "token": true,
	"encryptionScheme": true, "cipherSuite": true, "keyId": true,
	"trackBaseKey": true, "authInfo": true, "accessibility": true,
	"maxGrpSapStartingType": true, "maxObjSapStartingType": true,
	"contentProtectionRefIDs": true,
}

// lowerOf maps a lower-cased recognized name to its canonical spelling, used to
// detect case typos such as "sampleRate" for "samplerate".
func lowerOf(known map[string]bool) map[string]string {
	m := make(map[string]string, len(known))
	for k := range known {
		m[strings.ToLower(k)] = k
	}
	return m
}

var (
	rootFieldsLower  = lowerOf(rootFields)
	trackFieldsLower = lowerOf(trackFields)
)

// runSemanticChecks applies the cross-field and cross-reference rules that the
// CUE schema does not cover.
func runSemanticChecks(report *Report, raw map[string]any, isDelta bool) {
	if isDelta {
		// Delta updates are validated structurally by CUE; add the "MUST NOT"
		// field rules and per-add-track eventType checks here.
		checkUnknownFields(report, raw, "", rootFields, rootFieldsLower)
		if _, ok := raw[fieldVersion]; ok {
			report.addError(fieldVersion, "MSF 5.3", "a delta update MUST NOT contain a version field")
		}
		if _, ok := raw[fieldTracks]; ok {
			report.addError(fieldTracks, "MSF 5.3", "a delta update MUST NOT contain a tracks field; use deltaUpdate operations")
		}
		checkDeltaEventTypes(report, raw)
		return
	}

	checkUnknownFields(report, raw, "", rootFields, rootFieldsLower)
	checkInitDataDuplicates(report, raw)

	// Note: resolution of initRef -> initDataList.id and
	// contentProtectionRefIDs -> contentProtections.refID is enforced by the
	// CUE schema (#refCheck), not here.

	anyLiveTrack := false
	for _, field := range []string{fieldTracks, fieldPublishTracks} {
		tracks, ok := raw[field].([]any)
		if !ok {
			continue
		}
		seen := map[string]string{} // "namespace\x00name" -> path of first occurrence
		for i, t := range tracks {
			track, ok := t.(map[string]any)
			if !ok {
				continue
			}
			path := fmt.Sprintf("%s[%d]", field, i)
			if live, ok := track["isLive"].(bool); ok && live {
				anyLiveTrack = true
			}
			checkTrack(report, track, path)
			checkUniqueName(report, track, path, seen)
		}
	}

	checkGeneratedAt(report, raw, anyLiveTrack)
}

// checkDeltaEventTypes applies the eventType placement rule to tracks added by
// a delta update.
func checkDeltaEventTypes(report *Report, raw map[string]any) {
	ops, ok := raw[fieldDeltaUpdate].([]any)
	if !ok {
		return
	}
	for i, o := range ops {
		op, ok := o.(map[string]any)
		if !ok {
			continue
		}
		if op["op"] != "add" {
			continue
		}
		tracks, ok := op["tracks"].([]any)
		if !ok {
			continue
		}
		for j, t := range tracks {
			track, ok := t.(map[string]any)
			if !ok {
				continue
			}
			checkEventType(report, track, fmt.Sprintf("deltaUpdate[%d].tracks[%d]", i, j))
		}
	}
}

// checkEventType enforces that eventType appears only on eventtimeline tracks
// (Section 5.2.5). The required-when-eventtimeline half is handled by CUE.
func checkEventType(report *Report, track map[string]any, path string) {
	if _, ok := track["eventType"]; !ok {
		return
	}
	if pkg, _ := track["packaging"].(string); pkg != "eventtimeline" {
		report.addError(path+".eventType", "MSF 5.2.5",
			"eventType MUST NOT be present unless packaging is \"eventtimeline\" (packaging is %q)", pkg)
	}
}

// checkTrack runs the per-track semantic rules. Reference resolution (initRef,
// contentProtectionRefIDs) is handled by the CUE schema, not here.
func checkTrack(report *Report, track map[string]any, path string) {
	checkUnknownFields(report, track, path, trackFields, trackFieldsLower)
	checkEventType(report, track, path)

	_, hasTargetLatency := track["targetLatency"]
	_, hasBuffers := track["buffers"]
	if hasTargetLatency && hasBuffers {
		report.addError(path, "MSF 5.2.8",
			"targetLatency and buffers are mutually exclusive; a track MUST NOT carry both")
	}

	if dur, ok := track["trackDuration"]; ok {
		if live, ok := track["isLive"].(bool); ok && live {
			report.addError(path+".trackDuration", "MSF 5.2.35",
				"trackDuration MUST NOT be present when isLive is true (value: %v)", dur)
		}
	}

	// SHOULD-level recommendations.
	if role, _ := track["role"].(string); role == "video" {
		if _, ok := track["width"]; !ok {
			report.addWarning(path, "MSF 5.2.26", "video track SHOULD declare width")
		}
		if _, ok := track["height"]; !ok {
			report.addWarning(path, "MSF 5.2.27", "video track SHOULD declare height")
		}
	}
}

// checkUniqueName enforces that track names are unique per namespace.
func checkUniqueName(report *Report, track map[string]any, path string, seen map[string]string) {
	name, ok := track["name"].(string)
	if !ok {
		return // missing name is reported by CUE
	}
	ns, _ := track["namespace"].(string) // empty means "inherits the catalog namespace"
	key := ns + "\x00" + name
	if first, dup := seen[key]; dup {
		nsLabel := ns
		if nsLabel == "" {
			nsLabel = "(inherited)"
		}
		report.addError(path+".name", "MSF 5.2.3",
			"duplicate track name %q in namespace %s (first declared at %s); names MUST be unique per namespace",
			name, nsLabel, first)
		return
	}
	seen[key] = path
}

// checkGeneratedAt warns when generatedAt is present although nothing is live.
func checkGeneratedAt(report *Report, raw map[string]any, anyLiveTrack bool) {
	if _, ok := raw["generatedAt"]; !ok {
		return
	}
	if !anyLiveTrack {
		report.addWarning("generatedAt", "MSF 5.1.2",
			"generatedAt SHOULD NOT be present when no track is live")
	}
}

// checkUnknownFields warns about field names that are neither recognized nor
// namespaced custom fields. Recognized names misspelled by case are flagged
// with a "did you mean" hint.
func checkUnknownFields(report *Report, obj map[string]any, path string, known map[string]bool, knownLower map[string]string) {
	for key := range obj {
		if known[key] {
			continue
		}
		fieldPath := key
		if path != "" {
			fieldPath = path + "." + key
		}
		if canonical, ok := knownLower[strings.ToLower(key)]; ok {
			report.addWarning(fieldPath, ruleSections[canonical],
				"unrecognized field %q; did you mean %q?", key, canonical)
			continue
		}
		if strings.Contains(key, ".") {
			// Reverse-DNS / vendor-namespaced custom field: allowed, ignored.
			continue
		}
		report.addWarning(fieldPath, "MSF 5",
			"unrecognized field %q; parsers will ignore it (custom fields SHOULD use reverse-DNS names to avoid collisions)", key)
	}
}

// checkInitDataDuplicates reports duplicate ids in initDataList, which MUST be
// unique within the catalog (Section 5.1.7). Resolution of references to these
// ids is handled by the CUE schema.
func checkInitDataDuplicates(report *Report, raw map[string]any) {
	list, ok := raw["initDataList"].([]any)
	if !ok {
		return
	}
	seen := map[string]int{}
	for i, e := range list {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, ok := entry["id"].(string)
		if !ok {
			continue
		}
		seen[id]++
		if seen[id] == 2 {
			report.addError(fmt.Sprintf("initDataList[%d].id", i), ruleInitData,
				"duplicate initDataList id %q; ids MUST be unique within the catalog", id)
		}
	}
}
