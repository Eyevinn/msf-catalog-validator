# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
