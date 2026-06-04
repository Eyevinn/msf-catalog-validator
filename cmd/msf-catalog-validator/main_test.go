package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

const (
	validFile     = "../../testdata/valid/msf_simulcast.json"
	invalidFile   = "../../testdata/invalid/bad_initref.json"
	flagFormat    = "-format"
	flagSchema    = "-schema"
	draft01String = "draft-01"
)

func TestRun(t *testing.T) {
	cases := []struct {
		desc             string
		args             []string
		stdin            io.Reader
		w                io.Writer
		wantErr          bool // any non-nil error
		wantNotCompliant bool // err is errNotCompliant specifically
	}{
		{desc: "help", args: []string{appName, "-h"}, w: io.Discard},
		{desc: "unknown flag", args: []string{appName, "-nope"}, w: io.Discard, wantErr: true},
		{desc: "too many args", args: []string{appName, "a.json", "b.json"}, w: io.Discard, wantErr: true},
		{desc: "non-existing file", args: []string{appName, "no-such-file.json"}, w: io.Discard, wantErr: true},
		{desc: "bad format", args: []string{appName, flagFormat, "yaml", validFile}, w: io.Discard, wantErr: true},
		{desc: "valid file (text)", args: []string{appName, validFile}, w: io.Discard},
		{desc: "valid file (json)", args: []string{appName, flagFormat, formatJSON, validFile}, w: io.Discard},
		{desc: "invalid file", args: []string{appName, invalidFile}, w: io.Discard, wantErr: true, wantNotCompliant: true},
		{desc: "force schema", args: []string{appName, flagSchema, draft01String, validFile}, w: io.Discard},
		{desc: "valid stdin", args: []string{appName}, stdin: strings.NewReader(`{"version":"draft-01","tracks":[]}`), w: io.Discard},
		{desc: "bad writer (json)", args: []string{appName, flagFormat, formatJSON, validFile}, w: &badWriter{}, wantErr: true},
		{desc: "serve bad address", args: []string{appName, "-serve", "bad-address-no-port"}, w: io.Discard, wantErr: true},
		{desc: "version", args: []string{appName, "-version"}, w: io.Discard},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			stdin := c.stdin
			if stdin == nil {
				stdin = strings.NewReader("")
			}
			err := run(c.args, stdin, c.w)
			if c.wantErr && err == nil {
				t.Fatal("expected an error but got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantNotCompliant && !errors.Is(err, errNotCompliant) {
				t.Fatalf("expected errNotCompliant, got %v", err)
			}
		})
	}
}

func TestRunTextOutput(t *testing.T) {
	var b strings.Builder
	err := run([]string{appName, invalidFile}, strings.NewReader(""), &b)
	if !errors.Is(err, errNotCompliant) {
		t.Fatalf("expected errNotCompliant, got %v", err)
	}
	out := b.String()
	for _, want := range []string{"NOT COMPLIANT", "initRef", "MSF 5.2.13"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRunVersion(t *testing.T) {
	var b strings.Builder
	if err := run([]string{appName, "-version"}, strings.NewReader(""), &b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(b.String(), appName) {
		t.Errorf("version output %q does not contain app name", b.String())
	}
}

func TestParseOptions(t *testing.T) {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts, err := parseOptions(fs, []string{appName, flagFormat, formatJSON, flagSchema, draft01String, "file.json"})
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if opts.format != formatJSON {
		t.Errorf("format = %q, want json", opts.format)
	}
	if opts.forceVersion != draft01String {
		t.Errorf("forceVersion = %q, want draft-01", opts.forceVersion)
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "file.json" {
		t.Errorf("positional args = %v, want [file.json]", got)
	}
}

// badWriter always fails, to exercise the output error paths.
type badWriter struct{}

func (w *badWriter) Write(p []byte) (int, error) { return 0, os.ErrClosed }
