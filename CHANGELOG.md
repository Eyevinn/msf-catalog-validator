# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-07-07

### Changed

- `locmafVersion` on `locmaf` tracks must now be of the form `x.y` (e.g.
  `"0.3"`); values that don't match this form are rejected

## [0.2.0] - 2026-06-05

### Added

- `locmaf` packaging value and `locmafVersion` track field from LOCMAF
  (draft-einarsson-moq-locmaf). `locmafVersion` is required on `locmaf` tracks
  and must currently be `"0.2"`
- Web UI: one-click example catalogs (with a link to the full testdata on
  GitHub for more), a collapsible view of the draft-01 CUE schema, links to the
  MSF and CMSF specifications, and JSON/CUE syntax highlighting via Prism.js
- Web UI uses relative URLs so it can be hosted behind a reverse-proxy path
  prefix (e.g. /msf-catalog-validator); adds a systemd unit and a `build-linux`
  Makefile target for deployment

### Changed

- Stricter validation that follows the draft-01 field *definitions* rather than
  the (inconsistent) examples; see [moq-wg/msf#177](https://github.com/moq-wg/msf/pull/177):
  - `version` must use the `"draft-XX"` convention (only `"draft-01"`); the
    legacy `"1"` is rejected instead of being accepted as an alias
  - the mime-type field is `mimeType`; lower-case `mimetype` is flagged as a
    probable typo rather than accepted

### Fixed

- `bitrate` is no longer required for non-audio/video media tracks (e.g.
  subtitle/caption tracks in cmaf packaging); per MSF Section 5.2.22 it is
  required only for audio and video

## [0.1.0] - 2026-06-04

### Added

- Initial MSF/CMSF catalog validator built around a CUE schema
- `draft-01` schema covering MSF (draft-ietf-moq-msf-01) and CMSF
  (draft-ietf-moq-cmsf-01) catalogs, including conditional requirements
  (e.g. an audio track must declare `samplerate` and `channelConfig`)
- Version dispatch on the catalog `version` string; `"1"` accepted as an
  alias for `draft-01` with an informational note
- Referential integrity enforced in the CUE schema (`#refCheck`): `initRef`
  must resolve to an `initDataList` id and each `contentProtectionRefIDs` entry
  to a `contentProtections` refID
- Go semantic checks layered on top of CUE: `initRef`/`contentProtectionRefIDs`
  resolution, mutually-exclusive fields, `trackDuration`/`isLive` rule, unique
  track names, delta-update field rules, unknown-field typo detection, and
  SHOULD-level warnings
- CLI with text and JSON reports, file or stdin input, and a `-serve` HTTP
  upload UI
- `-version` flag and build-time version injection (via `internal.GetVersion`);
  the version and a link to the source repository are shown in the web UI. The
  schema-override flag is `-schema` (since `-version` now prints the version)

[Unreleased]: https://github.com/Eyevinn/msf-catalog-validator/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Eyevinn/msf-catalog-validator/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Eyevinn/msf-catalog-validator/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Eyevinn/msf-catalog-validator/releases/tag/v0.1.0
