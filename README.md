![Test](https://github.com/Eyevinn/msf-catalog-validator/workflows/Go/badge.svg)
[![golangci-lint](https://github.com/Eyevinn/msf-catalog-validator/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/Eyevinn/msf-catalog-validator/actions/workflows/golangci-lint.yml)
[![Coverage Status](https://coveralls.io/repos/github/Eyevinn/msf-catalog-validator/badge.svg?branch=main)](https://coveralls.io/github/Eyevinn/msf-catalog-validator?branch=main)
[![GoDoc](https://godoc.org/github.com/Eyevinn/msf-catalog-validator?status.svg)](http://godoc.org/github.com/Eyevinn/msf-catalog-validator)
[![license](https://img.shields.io/github/license/Eyevinn/msf-catalog-validator.svg)](https://github.com/Eyevinn/msf-catalog-validator/blob/main/LICENSE)

# msf-catalog-validator

A validator for **MSF** ([draft-ietf-moq-msf](https://datatracker.ietf.org/doc/draft-ietf-moq-msf/))
and **CMSF** ([draft-ietf-moq-cmsf](https://datatracker.ietf.org/doc/draft-ietf-moq-cmsf/))
catalog documents. The `locmaf` packaging value and the `locmafVersion` track
field from **LOCMAF**
([draft-einarsson-moq-locmaf](https://datatracker.ietf.org/doc/draft-einarsson-moq-locmaf/))
are recognized as well.

Feed it a catalog JSON document and it returns a report describing exactly which
parts are not compliant, each finding pointing at the offending path and the
relevant specification section.

Validation is driven by a [CUE](https://cuelang.org) schema, so the structural
rules are declarative and easy to read. The schema is selected from the
catalog's own `version` string, so multiple draft revisions can be supported
side by side. Today the only supported version is **`draft-01`** (covering both
MSF draft-01 and CMSF draft-01, which share a catalog format).

A live instance of the web UI is available at
<https://moqlivemock.demo.osaas.io/msf-catalog-validator/>.

## Install

Install the CLI directly from GitHub with `go install`:

```sh
go install github.com/Eyevinn/msf-catalog-validator/cmd/msf-catalog-validator@latest
```

This puts the `msf-catalog-validator` binary in `$(go env GOBIN)` (or
`$(go env GOPATH)/bin`). Pin a specific release with `@v0.1.0` instead of
`@latest`.

## Build & test

The module is self-contained and pins `cuelang.org/go`. A local `go.work` makes
`go` resolve this module (and its dependencies) even though the directory lives
inside the larger `moq-workspace` go workspace.

```sh
make build      # -> out/msf-catalog-validator
make test       # run the unit tests and fixtures
make vet
```

## Usage

```sh
# Validate a file
out/msf-catalog-validator testdata/valid/msf_simulcast.json

# Validate from stdin
cat catalog.json | out/msf-catalog-validator

# JSON report (for tooling/CI)
out/msf-catalog-validator -format json catalog.json

# Force a schema version regardless of the document's version field
out/msf-catalog-validator -schema draft-01 catalog.json

# Print the tool version
out/msf-catalog-validator -version

# Browser UI: paste or upload a catalog and see the report
out/msf-catalog-validator -serve :8080
#   GET  /            upload/paste form, one-click example catalogs, the
#                     draft-01 CUE schema, and JSON/CUE syntax highlighting
#   POST /validate    multipart upload (field "catalog") or raw JSON body;
#                     send "Accept: application/json" for a JSON report
```

The process exit code is `0` when compliant, `1` when there are error-severity
findings, and `2` on usage/IO errors.

### Example

```
$ out/msf-catalog-validator testdata/invalid/audio_missing_samplerate.json
MSF/CMSF catalog validation report
  document version : draft-01
  validated against: draft-01
  document kind    : catalog
  result           : NOT COMPLIANT (6 error(s), 2 warning(s), 0 info)

ERROR   field is required but not present
        at: tracks[0].samplerate
        ref: MSF 5.2.28
ERROR   value is not permitted here (does not match any allowed value for this field)
        at: tracks[1].packaging
        ref: MSF 5.2.4
...
```

## How it works

Validation has two layers.

### 1. CUE schema (`schemas/draft-01/catalog.cue`)

The schema enforces everything that is naturally structural:

- **Required fields** — `version`, track `name`, `packaging`, ...
- **Value types and ranges** — `bitrate` is a positive integer, `framerate` a
  positive number, `maxGrpSapStartingType` is `0..3`, KIDs/system IDs are UUIDs,
  ...
- **Allowed enum values** — `packaging ∈ {loc, cmaf, locmaf, mediatimeline,
  eventtimeline, moqlog, moqmetrics}`, content-protection `scheme ∈ {cenc,
  cbcs}`, ...
- **Conditional requirements**, the interesting part:
  - an **audio** track (`role: "audio"`) MUST carry `samplerate`,
    `channelConfig`, `codec` and `bitrate`;
  - a **video** track MUST carry `codec` and `bitrate`;
  - a **LOC/CMAF media** track MUST carry `isLive`, `codec` and `bitrate`;
  - an **eventtimeline** track MUST carry `eventType`.

  These conditionals are driven by hidden `_pkg`/`_role` fields that mirror
  `packaging`/`role` but fall back to a sentinel when absent, so the `if` guards
  always see a concrete value. The default marker (`*packaging | "__unset__"`)
  is placed on the real field so a present value always wins.
- **Referential integrity** (`#refCheck`) — `initRef` must equal one of the
  `initDataList[].id` values declared in the catalog, and each
  `contentProtectionRefIDs` entry must equal one of the `contentProtections[].refID`
  values. The schema collects the declared ids into a domain and constrains each
  reference field with `or(domain)`, so `cue vet` alone catches a dangling
  reference. (The Go layer only rewrites the resulting message to read clearly.)

Structs are kept **open**: the spec lets producers add custom, reverse-DNS
namespaced fields and requires parsers to ignore unknown fields, so the schema
never rejects extra keys. (Likely typos are caught as warnings — see below.)

### 2. Go semantic checks (`internal/validator/semantic.go`)

Rules that CUE expresses poorly, or where a hand-written message is clearer:

- **Mutually-exclusive fields**: `targetLatency` vs `buffers`.
- **Conditional absence**: `trackDuration` MUST NOT appear when `isLive` is
  true; `eventType` MUST NOT appear unless packaging is `eventtimeline`; a delta
  update MUST NOT carry `version` or `tracks`.
- **Uniqueness**: track `name` per namespace; `initDataList` `id`.
- **Unknown-field lint** (warnings): an unrecognized field whose lower-cased
  spelling matches a known field is flagged as a probable typo
  (`sampleRate` → "did you mean `samplerate`?"); other non-namespaced unknown
  fields get a softer warning; reverse-DNS custom fields are left alone.
- **SHOULD-level recommendations** (warnings): a video track without
  `width`/`height`; `generatedAt` present when nothing is live.

To produce a complete report, each track is also validated **individually**
against the `#Track` schema, so one catastrophic error (e.g. an invalid enum
value) cannot mask required-field errors on a sibling track. Duplicate findings
are de-duplicated.

## Version dispatch

The schema is chosen from the document's `version` string, which must match a
known schema exactly:

- `"draft-01"` → the draft-01 schema.
- Anything else (including the legacy `"1"`) → an **error** (unsupported
  version); validation stops because no schema is known.
- A **delta update** carries no `version` of its own (§5.3); it is validated
  against the default (`draft-01`), noted as an info finding.

## Adding a new version

1. Add `schemas/draft-NN/catalog.cue`.
2. Embed it in `schemas/schemas.go` and register it in `validator.New`
   (`e.schemas["draft-NN"] = ...`).
3. Add fixtures under `testdata/`.

## Strictness: definition over examples

The draft-01 documents have a few internal inconsistencies where the **examples**
disagreed with the normative **field definitions**. This validator deliberately
follows the *definitions* and is strict; we do not loosen the rules to admit the
(stale) example forms. Most of these were corrected upstream in
[moq-wg/msf#177](https://github.com/moq-wg/msf/pull/177).

- **Version** (§5.1.1) must use the `"draft-XX"` convention, so only
  `"draft-01"` is accepted. The earlier examples' `"version": "1"` is rejected.
- **Mime type** (§5.2.19) is `mimeType`. The lower-case `mimetype` some examples
  used is flagged as a probable typo (warning), not accepted.
- **Packaging** (§5.2.4) is required on every declared track, including tracks
  added by a delta `add` operation (an early example omitted it). Cloned tracks
  (`clone`) inherit from their parent, so `packaging` is optional there.

Other deliberate choices:

- `initDataList` SHOULD appear after `tracks` (§5.1.7). Ordering is not yet
  checked (a parsed JSON object does not preserve key order); only presence and
  id-uniqueness are.

## Layout

```
cmd/msf-catalog-validator/   CLI (run() is testable) + HTTP serve mode
internal/validator/          engine, report, CUE + semantic checks, tests
schemas/                     embedded CUE schemas (schemas.go + draft-01/)
examples/                    curated catalogs shown in the web UI (embedded)
testdata/valid/              compliant catalogs (must validate clean)
testdata/invalid/            non-compliant catalogs (each targets a rule)
```

## Commits, ChangeLog

This project aims to follow Semantic Versioning and
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
There is a manual [ChangeLog](CHANGELOG.md) that should be updated with
each change.

This project uses [pre-commit hooks][precmt]. Run `make pre-commit-install`
(which creates a Python virtual environment in `venv/` and installs
`pre-commit` and `codespell`) to enable them, or run them on demand with
`make check`.

## License

MIT, see [LICENSE](LICENSE).

## Support

Join our [community on Slack](http://slack.streamingtech.se) where you can post any questions regarding any of our open source projects. Eyevinn's consulting business can also offer you:

* Further development of this component
* Customization and integration of this component into your platform
* Support and maintenance agreement

Contact [sales@eyevinn.se](mailto:sales@eyevinn.se) if you are interested.

## About Eyevinn Technology

[Eyevinn Technology](https://www.eyevinntechnology.se) is an independent consultant firm specialized in video and streaming. Independent in a way that we are not commercially tied to any platform or technology vendor. As our way to innovate and push the industry forward we develop proof-of-concepts and tools. The things we learn and the code we write we share with the industry in [blogs](https://dev.to/video) and by open sourcing the code we have written.

Want to know more about Eyevinn and how it is to work here. Contact us at work@eyevinn.se!

[precmt]: https://pre-commit.com
