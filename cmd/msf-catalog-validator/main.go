// Command msf-catalog-validator validates MSF (draft-ietf-moq-msf) and CMSF
// (draft-ietf-moq-cmsf) catalog documents against version-specific CUE schemas
// and prints a compliance report.
//
// Usage:
//
//	msf-catalog-validator [flags] [file]
//	msf-catalog-validator -serve :8080
//
// With no file argument (and without -serve) the catalog is read from stdin.
// The process exits with status 1 when the catalog has any error-severity
// findings, 2 on usage/IO errors, and 0 when compliant.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/msf-catalog-validator/internal"
	"github.com/Eyevinn/msf-catalog-validator/internal/validator"
)

const appName = "msf-catalog-validator"

// Output formats accepted by the -format flag.
const (
	formatText = "text"
	formatJSON = "json"
)

var usg = `%s validates MSF/CMSF catalogs against their CUE schema.

Usage of %s:
  %s [options] [file]   validate a file (or stdin)
  %s -serve :8080       run an HTTP upload server

The exit code is 0 when compliant, 1 when there are error-severity findings,
and 2 on usage or I/O errors.

options:
`

var (
	// errNotCompliant signals that the catalog had error-severity findings.
	errNotCompliant = errors.New("catalog is not compliant")
	// errTooManyArgs is returned when more than one file argument is given.
	errTooManyArgs = errors.New("expected at most one file argument")
	// errUnknownFormat is returned for an unsupported -format value.
	errUnknownFormat = errors.New("unknown output format")
)

type options struct {
	format       string
	forceVersion string
	serveAddr    string
	version      bool
}

func parseOptions(fs *flag.FlagSet, args []string) (*options, error) {
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), usg, appName, appName, appName, appName)
		fs.PrintDefaults()
	}

	opts := options{}
	fs.StringVar(&opts.format, "format", formatText, "output format: text or json")
	fs.StringVar(&opts.forceVersion, "schema", "",
		"force a schema version (e.g. draft-01) instead of dispatching on the catalog's version field")
	fs.StringVar(&opts.serveAddr, "serve", "",
		"run an HTTP upload server on this address (e.g. :8080) instead of validating a file")
	fs.BoolVar(&opts.version, "version", false, fmt.Sprintf("print %s version and exit", appName))

	err := fs.Parse(args[1:])
	return &opts, err
}

func main() {
	err := run(os.Args, os.Stdin, os.Stdout)
	switch {
	case err == nil:
		return
	case errors.Is(err, errNotCompliant):
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
}

// run is the testable entry point. It parses args, validates the catalog (or
// starts the server) and writes output to w. It returns errNotCompliant when
// the catalog is well-formed but has error findings, nil when compliant, and a
// different error for usage/IO problems.
func run(args []string, stdin io.Reader, w io.Writer) error {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(w)
	opts, err := parseOptions(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if opts.version {
		_, err := fmt.Fprintf(w, "%s %s\n", appName, internal.GetVersion())
		return err
	}

	if opts.format != formatText && opts.format != formatJSON {
		return fmt.Errorf("%w: %q (use text or json)", errUnknownFormat, opts.format)
	}

	engine, err := validator.New()
	if err != nil {
		return fmt.Errorf("initializing validator: %w", err)
	}

	if opts.serveAddr != "" {
		return serve(engine, opts.serveAddr, w)
	}

	data, source, err := readInput(fs.Args(), stdin)
	if err != nil {
		return err
	}

	report, _ := engine.Validate(data, opts.forceVersion)

	if err := writeReport(w, report, opts.format, source); err != nil {
		return err
	}

	if !report.Valid {
		return errNotCompliant
	}
	return nil
}

// writeReport renders the report to w in the requested format.
func writeReport(w io.Writer, report *validator.Report, format, source string) error {
	switch format {
	case formatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("encoding report: %w", err)
		}
	default: // "text"
		if source != "" && source != "stdin" {
			if _, err := fmt.Fprintf(w, "Source: %s\n", source); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, report.Text()); err != nil {
			return err
		}
	}
	return nil
}

// readInput reads the catalog bytes from the first positional argument or, if
// none is given, from stdin.
func readInput(args []string, stdin io.Reader) (data []byte, source string, err error) {
	if len(args) > 1 {
		return nil, "", fmt.Errorf("%w, got %d", errTooManyArgs, len(args))
	}
	if len(args) == 1 {
		b, err := os.ReadFile(args[0])
		if err != nil {
			return nil, "", err
		}
		return b, args[0], nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, "", err
	}
	return b, "stdin", nil
}
