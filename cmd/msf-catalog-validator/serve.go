package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/Eyevinn/msf-catalog-validator/internal"
	"github.com/Eyevinn/msf-catalog-validator/internal/validator"
)

// repoURL is the source repository, linked from the web UI.
const repoURL = "https://github.com/Eyevinn/msf-catalog-validator"

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
	Report  *validator.Report
	Input   string
	Version string
	RepoURL string
}

func render(w http.ResponseWriter, d pageData) {
	d.Version = internal.GetVersion()
	d.RepoURL = repoURL
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
<style>
  :root { color-scheme: light dark; }
  body { font: 15px/1.5 system-ui, sans-serif; max-width: 60rem; margin: 2rem auto; padding: 0 1rem; }
  h1 { font-size: 1.4rem; }
  textarea { width: 100%; min-height: 16rem; font-family: ui-monospace, monospace; font-size: 13px; }
  button { font-size: 1rem; padding: .5rem 1rem; margin-top: .5rem; cursor: pointer; }
  .summary { padding: .75rem 1rem; border-radius: .5rem; margin: 1rem 0; }
  .ok { background: #e6f4ea; color: #137333; }
  .bad { background: #fce8e6; color: #c5221f; }
  ul.findings { list-style: none; padding: 0; }
  ul.findings li { padding: .5rem .75rem; border-left: 4px solid #ccc; margin: .4rem 0; background: rgba(127,127,127,.08); }
  li.sev-error { border-color: #c5221f; }
  li.sev-warning { border-color: #f29900; }
  li.sev-info { border-color: #1a73e8; }
  .meta { font-size: .8rem; opacity: .7; }
  code { font-family: ui-monospace, monospace; }
  footer { margin-top: 2rem; padding-top: 1rem; border-top: 1px solid rgba(127,127,127,.3); opacity: .8; }
  footer { font-size: .85rem; display: flex; gap: 1rem; flex-wrap: wrap; }
</style>
</head>
<body>
<h1>MSF / CMSF catalog validator</h1>
<p>Paste a catalog JSON document (or upload a file) to validate it against the
matching CUE schema. Validation dispatches on the catalog's <code>version</code> field.</p>

<form method="post" action="/validate" enctype="multipart/form-data">
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

<footer>
  <span>msf-catalog-validator {{.Version}}</span>
  <a href="{{.RepoURL}}" target="_blank" rel="noopener">Source on GitHub</a>
</footer>
</body>
</html>`))
