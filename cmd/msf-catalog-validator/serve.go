package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/Eyevinn/msf-catalog-validator/examples"
	"github.com/Eyevinn/msf-catalog-validator/internal"
	"github.com/Eyevinn/msf-catalog-validator/internal/validator"
	"github.com/Eyevinn/msf-catalog-validator/schemas"
)

// repoURL is the source repository, linked from the web UI.
const repoURL = "https://github.com/Eyevinn/msf-catalog-validator"

// testdataURL points at the full set of example catalogs on GitHub.
const testdataURL = repoURL + "/tree/main/testdata"

// errNoCatalog is returned when a /validate request carries no catalog data.
var errNoCatalog = errors.New("no catalog provided")

// serve runs a small HTTP server that lets users paste or upload a catalog and
// get a validation report back. GET / serves the form; POST /validate accepts
// either a multipart file upload (field "catalog") or a raw JSON body and
// returns HTML, or JSON when Accept: application/json is requested.
func serve(engine *validator.Engine, addr string, w io.Writer) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		render(w, pageData{})
	})
	mux.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		data, version, err := extractCatalog(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		report, _ := engine.Validate(data, version)

		if wantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			_ = enc.Encode(report)
			return
		}
		render(w, pageData{Report: report, Input: string(data)})
	})

	fmt.Fprintf(w, "msf-catalog-validator listening on http://localhost%s\n", addr)
	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}

func wantsJSON(r *http.Request) bool {
	return r.Header.Get("Accept") == "application/json" || r.URL.Query().Get("format") == formatJSON
}

// extractCatalog pulls the catalog bytes from a multipart upload, a form field,
// or the raw request body.
func extractCatalog(r *http.Request) (data []byte, version string, err error) {
	ct := r.Header.Get("Content-Type")
	if len(ct) >= 19 && ct[:19] == "multipart/form-data" {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			return nil, "", err
		}
		version = r.FormValue("version")
		if f, _, ferr := r.FormFile("catalog"); ferr == nil {
			defer f.Close()
			b, err := io.ReadAll(f)
			return b, version, err
		}
		if text := r.FormValue("text"); text != "" {
			return []byte(text), version, nil
		}
		return nil, "", errNoCatalog
	}
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 16<<20))
	return b, r.URL.Query().Get("version"), err
}

type pageData struct {
	Report      *validator.Report
	Input       string
	Version     string
	RepoURL     string
	TestdataURL string
	Examples    []examples.Example
	Schema      string
}

func render(w http.ResponseWriter, d pageData) {
	d.Version = internal.GetVersion()
	d.RepoURL = repoURL
	d.TestdataURL = testdataURL
	d.Examples = examples.List()
	d.Schema = schemas.Draft01
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTmpl.Execute(w, d); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var pageTmpl = template.Must(template.New("page").Funcs(template.FuncMap{
	"sevClass": func(s validator.Severity) string { return "sev-" + string(s) },
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>MSF/CMSF catalog validator</title>
<link rel="stylesheet" media="(prefers-color-scheme: light)"
  href="https://cdnjs.cloudflare.com/ajax/libs/prism/1.29.0/themes/prism.min.css">
<link rel="stylesheet" media="(prefers-color-scheme: dark)"
  href="https://cdnjs.cloudflare.com/ajax/libs/prism/1.29.0/themes/prism-tomorrow.min.css">
<style>
  :root { color-scheme: light dark; }
  body { font: 15px/1.5 system-ui, sans-serif; max-width: 60rem; margin: 2rem auto; padding: 0 1rem; }
  h1 { font-size: 1.4rem; }
  h2 { font-size: 1.1rem; margin-top: 2rem; }
  textarea { width: 100%; min-height: 16rem; font-family: ui-monospace, monospace; font-size: 13px; }
  button { font-size: 1rem; padding: .5rem 1rem; margin-top: .5rem; cursor: pointer; }
  button.small { font-size: .8rem; padding: .2rem .6rem; margin: 0; }
  .summary { padding: .75rem 1rem; border-radius: .5rem; margin: 1rem 0; }
  .ok { background: #e6f4ea; color: #137333; }
  .bad { background: #fce8e6; color: #c5221f; }
  ul.findings { list-style: none; padding: 0; }
  ul.findings li { padding: .5rem .75rem; border-left: 4px solid #ccc; margin: .4rem 0; background: rgba(127,127,127,.08); }
  li.sev-error { border-color: #c5221f; }
  li.sev-warning { border-color: #f29900; }
  li.sev-info { border-color: #1a73e8; }
  .meta { font-size: .8rem; opacity: .7; }
  .refs { font-size: .9rem; }
  code { font-family: ui-monospace, monospace; }
  pre { max-height: 22rem; overflow: auto; border-radius: .5rem; border: 1px solid rgba(127,127,127,.25); }
  pre code { font-size: 12px; }
  .example { margin: .75rem 0; }
  .ex-head { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
  .ex-head .desc { font-size: .85rem; opacity: .75; }
  details summary { cursor: pointer; font-weight: 600; }
  footer { margin-top: 2rem; padding-top: 1rem; border-top: 1px solid rgba(127,127,127,.3); opacity: .8; }
  footer { font-size: .85rem; display: flex; gap: 1rem; flex-wrap: wrap; }
</style>
</head>
<body>
<h1>MSF / CMSF catalog validator</h1>
<p>Paste a catalog JSON document (or upload a file) to validate it against the
matching CUE schema. Validation dispatches on the catalog's <code>version</code> field.</p>
<p class="refs">
  <a href="{{.RepoURL}}" target="_blank" rel="noopener">Source on GitHub</a>
  &middot; Specifications:
  <a href="https://datatracker.ietf.org/doc/draft-ietf-moq-msf/" target="_blank" rel="noopener">MSF</a>
  &middot;
  <a href="https://datatracker.ietf.org/doc/draft-ietf-moq-cmsf/" target="_blank" rel="noopener">CMSF</a>
</p>

<form method="post" action="validate" enctype="multipart/form-data">
  <textarea name="text" placeholder='{ "version": "draft-01", "tracks": [ ... ] }'>{{.Input}}</textarea>
  <div>
    <input type="file" name="catalog" accept=".json,application/json">
    <button type="submit">Validate</button>
  </div>
</form>

{{with .Report}}
  {{if .Valid}}
    <div class="summary ok">COMPLIANT — validated against {{.SchemaVersion}} ({{.Kind}})</div>
  {{else}}
    <div class="summary bad">NOT COMPLIANT — validated against {{.SchemaVersion}} ({{.Kind}})</div>
  {{end}}
  {{if .Findings}}
  <ul class="findings">
    {{range .Findings}}
      <li class="{{sevClass .Severity}}">
        <strong>{{.Severity}}</strong>: {{.Message}}
        <div class="meta">
          {{if .Path}}at <code>{{.Path}}</code>{{end}}
          {{if .Rule}} &middot; {{.Rule}}{{end}}
        </div>
      </li>
    {{end}}
  </ul>
  {{else}}
    <p>No issues found.</p>
  {{end}}
{{end}}

<h2>Examples</h2>
<p>Click <em>Load into editor</em> to drop a sample into the box above, then
press Validate. Browse the full set in the
<a href="{{.TestdataURL}}" target="_blank" rel="noopener">testdata directory on GitHub</a>.</p>
{{range .Examples}}
  <div class="example">
    <div class="ex-head">
      <strong>{{.Title}}</strong>
      <button type="button" class="small" onclick="loadExample(this)">Load into editor</button>
      <span class="desc">{{.Desc}}</span>
    </div>
    <pre><code class="language-json">{{.Content}}</code></pre>
  </div>
{{end}}

<h2>CUE schema</h2>
<details>
  <summary>Show the draft-01 schema</summary>
  <pre><code class="language-cue">{{.Schema}}</code></pre>
</details>

<footer>
  <span>msf-catalog-validator {{.Version}}</span>
</footer>

<script src="https://cdnjs.cloudflare.com/ajax/libs/prism/1.29.0/components/prism-core.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/prism/1.29.0/plugins/autoloader/prism-autoloader.min.js"
  data-autoloader-path="https://cdnjs.cloudflare.com/ajax/libs/prism/1.29.0/components/"></script>
<script>
  // loadExample copies the JSON from a sample block into the editor.
  // textContent returns the plain source even after Prism wraps it in spans.
  function loadExample(btn) {
    var code = btn.closest('.example').querySelector('code');
    var ta = document.querySelector('textarea[name=text]');
    ta.value = code.textContent;
    ta.scrollIntoView({ behavior: 'smooth', block: 'center' });
    ta.focus();
  }
</script>
</body>
</html>`))
