// Package examples provides a curated set of catalog documents that the web UI
// offers as one-click samples. They are embedded into the binary.
package examples

import "embed"

//go:embed *.json
var files embed.FS

// Example is a named catalog sample for display in the web UI.
type Example struct {
	Title   string
	Desc    string
	Content string
}

// catalog lists the curated samples in display order.
var catalog = []struct {
	title, desc, file string
}{
	{
		"Time-aligned audio/video (LOC)",
		"A minimal compliant MSF catalog with one video and one audio LOC track.",
		"msf_simulcast.json",
	},
	{
		"Non-compliant example",
		"Shows how violations are reported (missing samplerate, invalid packaging).",
		"invalid.json",
	},
}

// List returns the curated examples in display order. Files that fail to load
// are skipped (this can only happen due to a build error).
func List() []Example {
	out := make([]Example, 0, len(catalog))
	for _, c := range catalog {
		b, err := files.ReadFile(c.file)
		if err != nil {
			continue
		}
		out = append(out, Example{Title: c.title, Desc: c.desc, Content: string(b)})
	}
	return out
}
