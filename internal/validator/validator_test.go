package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func validateFile(t *testing.T, e *Engine, path string) *Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	r, _ := e.Validate(data, "")
	return r
}

// errorRules returns the set of spec references attached to error findings.
func errorRules(r *Report) map[string]bool {
	out := map[string]bool{}
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			out[f.Rule] = true
		}
	}
	return out
}

func TestValidFixtures(t *testing.T) {
	e := newEngine(t)
	files, _ := filepath.Glob("../../testdata/valid/*.json")
	if len(files) == 0 {
		t.Fatal("no valid fixtures found")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			r := validateFile(t, e, f)
			if !r.Valid {
				for _, fi := range r.Findings {
					if fi.Severity == SeverityError {
						t.Errorf("unexpected error: %s at %s (%s)", fi.Message, fi.Path, fi.Rule)
					}
				}
			}
		})
	}
}

func TestInvalidFixtures(t *testing.T) {
	// Each fixture must be reported NOT compliant and must contain at least the
	// listed rules among its error findings.
	cases := map[string][]string{
		"audio_missing_samplerate.json": {"MSF 5.2.28", "MSF 5.2.4", "MSF 5.2.18", "MSF 5.2.5"},
		"bad_initref.json":              {"MSF 5.2.13"},
		"bad_cpref.json":                {"CMSF 4.1.2"},
		"latency_and_buffers.json":      {"MSF 5.2.8"},
		"trackduration_when_live.json":  {"MSF 5.2.35"},
		"duplicate_name.json":           {"MSF 5.2.3"},
		"delta_with_version.json":       {"MSF 5.3"},
		"locmaf_bad_version.json":       {"LOCMAF"},
	}
	e := newEngine(t)
	for name, wantRules := range cases {
		t.Run(name, func(t *testing.T) {
			r := validateFile(t, e, filepath.Join("../../testdata/invalid", name))
			if r.Valid {
				t.Fatalf("expected NOT compliant, got compliant")
			}
			got := errorRules(r)
			for _, rule := range wantRules {
				if !got[rule] {
					t.Errorf("missing expected error rule %q; got findings:\n%s", rule, r.Text())
				}
			}
		})
	}
}

func TestLOCMAFCatalog(t *testing.T) {
	// A real catalog mixing cmaf and locmaf tracks (with locmafVersion) must
	// validate clean.
	e := newEngine(t)
	r := validateFile(t, e, "../../testdata/locmaf/catalog")
	if !r.Valid {
		t.Errorf("expected LOCMAF catalog to be compliant; got:\n%s", r.Text())
	}
}

func TestFieldTypoWarning(t *testing.T) {
	e := newEngine(t)
	r := validateFile(t, e, "../../testdata/invalid/field_typo.json")

	// The camelCase "sampleRate" should be flagged as a typo for "samplerate"...
	var sawTypo, sawCustom bool
	for _, f := range r.Findings {
		if f.Severity == SeverityWarning && strings.Contains(f.Message, `did you mean "samplerate"`) {
			sawTypo = true
		}
		if strings.Contains(f.Message, "com.example-tier") {
			sawCustom = true
		}
	}
	if !sawTypo {
		t.Errorf("expected a 'did you mean samplerate' warning; got:\n%s", r.Text())
	}
	// ...while the reverse-DNS custom field must not be flagged at all.
	if sawCustom {
		t.Errorf("namespaced custom field should not be flagged; got:\n%s", r.Text())
	}
	// And the missing correct samplerate is still an error.
	if !errorRules(r)["MSF 5.2.28"] {
		t.Errorf("expected missing-samplerate error; got:\n%s", r.Text())
	}
}

func TestVersionDispatch(t *testing.T) {
	e := newEngine(t)

	t.Run("legacy version 1 is now rejected", func(t *testing.T) {
		// The examples used to carry "version": "1"; per Section 5.1.1 the
		// normative form is "draft-XX", so "1" is no longer accepted.
		r, _ := e.Validate([]byte(`{"version":"1","tracks":[]}`), "")
		if r.Valid {
			t.Errorf("expected legacy version \"1\" to be rejected")
		}
		if !errorRules(r)["MSF 5.1.1"] {
			t.Errorf("expected MSF 5.1.1 error for legacy version; got:\n%s", r.Text())
		}
	})

	t.Run("draft-01 is accepted", func(t *testing.T) {
		r, _ := e.Validate([]byte(`{"version":"draft-01","tracks":[]}`), "")
		if r.SchemaVersion != defaultSchemaVersion {
			t.Errorf("schemaVersion = %q, want %s", r.SchemaVersion, defaultSchemaVersion)
		}
		if !r.Valid {
			t.Errorf("expected draft-01 empty catalog to be valid; got:\n%s", r.Text())
		}
	})

	t.Run("unknown version is an error", func(t *testing.T) {
		r, _ := e.Validate([]byte(`{"version":"draft-99","tracks":[]}`), "")
		if r.Valid {
			t.Errorf("expected unknown version to be invalid")
		}
		if !errorRules(r)["MSF 5.1.1"] {
			t.Errorf("expected MSF 5.1.1 error for unknown version; got:\n%s", r.Text())
		}
	})

	t.Run("missing version is an error", func(t *testing.T) {
		r, _ := e.Validate([]byte(`{"tracks":[]}`), "")
		if r.Valid {
			t.Errorf("expected missing version to be invalid")
		}
	})
}

func TestInvalidJSON(t *testing.T) {
	e := newEngine(t)
	r, err := e.Validate([]byte(`{not json`), "")
	if err == nil {
		t.Errorf("expected an error for malformed JSON")
	}
	if r.Valid {
		t.Errorf("malformed JSON must not be reported valid")
	}
}
