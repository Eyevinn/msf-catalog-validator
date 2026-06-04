// Package draft01 holds the CUE schema for MSF / CMSF catalogs that declare
// a "version" of "draft-01" (draft-ietf-moq-msf-01 and draft-ietf-moq-cmsf-01).
//
// CMSF is a superset of MSF: it adds the "cmaf" packaging value, the
// maxGrpSapStartingType / maxObjSapStartingType track fields, a root-level
// contentProtections array and the track-level contentProtectionRefIDs field.
// A single schema therefore covers both formats; whether a catalog is "MSF"
// or "CMSF" follows from the packaging values and DRM fields it actually uses.
//
// The schema also accepts the "locmaf" packaging value and the locmafVersion
// track field from [LOCMAF].
//
// References:
//   [MSF]    draft-ietf-moq-msf-01    - MOQT Streaming Format
//   [CMSF]   draft-ietf-moq-cmsf-01   - CMAF-based MOQT Streaming Format
//   [LOCMAF] draft-einarsson-moq-locmaf - Low Overhead CMAF for Media over QUIC
//
// Design notes
//   - Structs are *open* (`...`): the spec allows producers to add custom,
//     namespaced fields (e.g. "com.example-tier") and requires parsers to
//     ignore unknown fields. Typo detection for near-miss field names is done
//     in Go, not here, so that custom fields are never rejected.
//   - Conditional requirements (an audio track MUST carry samplerate, a media
//     track MUST carry codec/bitrate/isLive, ...) are driven by the hidden
//     `_pkg` and `_role` fields. These mirror `packaging`/`role` but fall back
//     to a sentinel when the source field is absent, so the `if` guards always
//     see a concrete value instead of failing as "non-concrete".
//   - Reference integrity is enforced here too: initRef must resolve to an
//     initDataList id and each contentProtectionRefIDs entry to a
//     contentProtections refID (see #refCheck).
//   - A few checks that CUE expresses poorly (mutually-exclusive fields, unique
//     track names, duplicate ids) are implemented as Go semantic checks instead.
package draft01

import "list"

// #Catalog is an independent (complete) catalog object. Delta updates are
// validated against #DeltaCatalog; the caller chooses based on the presence of
// the deltaUpdate field.
#Catalog: {
	// Section 5.1.1 - MSF version. Required.
	version!: string

	// Section 5.1.2 - wallclock generation time, milliseconds since the Unix
	// epoch. SHOULD NOT be present when the content is not live.
	generatedAt?: int & >=0

	// Section 5.1.3 - a catalog-level "broadcast complete" flag. It MUST NOT be
	// present when false, so the only legal value is true.
	isComplete?: true

	// Section 5.1.4 - the track array. Required for an independent catalog.
	// Each track is additionally constrained by #refCheck so that its
	// references resolve to entries that actually exist in this catalog.
	tracks!: [...(#Track & #refCheck)]

	// Section 5.1.5 - publish tracks (logs, metrics, QoE). Same shape as tracks.
	publishTracks?: [...(#Track & #refCheck)]

	// Section 5.1.7 - initialization data list. Per the spec it SHOULD appear
	// after tracks; ordering is checked in Go. Defaults to an empty list so the
	// reference checks below always have a concrete set to test against.
	initDataList: [...#InitData] | *[]

	// CMSF Section 4.1.1 - root-level DRM descriptions.
	contentProtections: [...#ContentProtection] | *[]

	... // allow custom root-level fields

	// The ids/refIDs declared in this catalog, used for reference resolution.
	let _initIDs = [for d in initDataList {d.id}]
	let _cpIDs = [for c in contentProtections {c.refID}]

	// Reference domains. A sentinel that cannot appear in real data keeps each
	// domain non-empty when nothing is declared, so an unresolved reference
	// still fails *at the reference field* instead of as a bare empty-set error.
	let _unresolved = "<unresolved-reference>"
	let _initDomain = list.Concat([_initIDs, [if len(_initIDs) == 0 {_unresolved}]])
	let _cpDomain = list.Concat([_cpIDs, [if len(_cpIDs) == 0 {_unresolved}]])

	// #refCheck constrains a track's reference fields against the ids declared
	// above. It is unified onto every track:
	//   - initRef (Section 5.2.13) must equal one of the initDataList ids.
	//   - each contentProtectionRefIDs entry (CMSF Section 4.1.2) must equal one
	//     of the contentProtections refIDs.
	#refCheck: {
		initRef?: or(_initDomain)
		contentProtectionRefIDs?: [...or(_cpDomain)]
		...
	}
}

// #DeltaCatalog is a delta (partial) update. Per Section 5.3 it MUST carry a
// deltaUpdate array with at least one operation and MUST NOT carry tracks or
// version.
// A delta update MUST carry a deltaUpdate array with at least one operation
// and MUST NOT carry tracks or version (Section 5.3). The "MUST NOT" half is a
// Go semantic check so the report can explain it clearly.
#DeltaCatalog: {
	generatedAt?: int & >=0
	deltaUpdate!: [#DeltaOp, ...#DeltaOp]
	...
}

#DeltaOp: {
	op!: "add" | "remove" | "clone"

	if op == "add" {
		tracks!: [...#Track]
	}
	if op == "remove" {
		// A remove entry carries only name (and optionally namespace).
		tracks!: [...#RemoveTrack]
	}
	if op == "clone" {
		tracks!: [...#CloneTrack]
	}
	...
}

#RemoveTrack: close({
	name!:      string
	namespace?: string
})

// A cloned track inherits everything from its parent, so the only hard
// requirements are parentName and the new name.
#CloneTrack: #TrackBase & {
	parentName!:      string
	parentNamespace?: string
	name!:            string
}

// -----------------------------------------------------------------------------
// Tracks
// -----------------------------------------------------------------------------

// Section 5.2.4 - allowed packaging values. CMSF adds "cmaf"; "locmaf" is the
// Low Overhead CMAF packaging defined in [LOCMAF] (draft-einarsson-moq-locmaf,
// "Low Overhead CMAF for Media over QUIC").
#Packaging: "loc" | "cmaf" | "locmaf" | "mediatimeline" | "eventtimeline" | "moqlog" | "moqmetrics"

// #Track is a declared track in an independent catalog or an "add" delta op.
// packaging is required here (Section 5.2.4).
#Track: #TrackBase & {
	packaging!: #Packaging
}

// #TrackBase carries every track field and all conditional rules, but leaves
// packaging optional so it can be reused for cloned tracks (which inherit it).
#TrackBase: {
	// Section 5.2.3 - track name. Required and, per namespace, unique
	// (uniqueness is enforced in Go).
	name!: string

	// Section 5.2.2 - namespace. Optional; inherits the catalog namespace.
	namespace?: string

	// Section 5.2.4 - packaging. Optional at this level; #Track makes it
	// required for declared tracks.
	packaging?: #Packaging

	// LOCMAF wire-format version, a track-level field defined by [LOCMAF]
	// (draft-einarsson-moq-locmaf). Required for "locmaf" packaging and, for
	// now, fixed to "0.2" (the only version this validator knows); see the
	// conditional rule below.
	locmafVersion?: string

	// Section 5.2.6 - track role. Optional; known roles are documented below
	// but custom roles are permitted, so the type stays an open string.
	//   audiodescription | video | audio | mediatimeline | eventtimeline |
	//   caption | subtitle | signlanguage | log | metrics | <custom>
	role?: string

	// Section 5.2.5 - event timeline type. Constrained below: required for
	// eventtimeline packaging and forbidden otherwise.
	eventType?: string

	// Section 5.2.7 - whether new objects will be added to this track.
	isLive?: bool

	// Section 5.2.8 - target latency in milliseconds. Mutually exclusive with
	// buffers (checked in Go).
	targetLatency?: int & >=0

	// Section 5.2.9 - target buffer object.
	buffers?: #Buffers

	// Section 5.2.10 - human-readable label.
	label?: string

	// Section 5.2.11 / 5.2.12 - render and alternate group identifiers.
	renderGroup?: int & >=0
	altGroup?:    int & >=0

	// Section 5.2.13 - reference into initDataList (resolved by #refCheck).
	initRef?: string

	// Section 5.2.14 - track names this track depends on.
	depends?: [...string]

	// Section 5.2.16 / 5.2.17 - SVC layer identifiers.
	temporalId?: int & >=0
	spatialId?:  int & >=0

	// Section 5.2.18 - codec string. Required for media tracks (below).
	codec?: string

	// Section 5.2.19 - mime type. The field table spells it "mimeType"; the
	// lower-case "mimetype" used by some early examples is not accepted (it is
	// flagged as a probable typo by the Go field lint).
	mimeType?: string

	// Section 5.2.20 - frame rate (may be fractional, e.g. 29.97).
	framerate?: number & >0

	// Section 5.2.21 - timescale.
	timescale?: int & >0

	// Section 5.2.22 / 5.2.23 - bitrates in bits per second.
	bitrate?:    int & >0
	avgBitrate?: int & >0

	// Section 5.2.24 / 5.2.25 - durations in milliseconds.
	maxGopDuration?:   int & >=0
	maxGroupDuration?: int & >=0

	// Section 5.2.26 / 5.2.27 - encoded dimensions.
	width?:  int & >0
	height?: int & >0

	// Section 5.2.28 / 5.2.29 - audio properties. Required for audio (below).
	samplerate?:    int & >0
	channelConfig?: string

	// Section 5.2.30 / 5.2.31 - intended display dimensions.
	displayWidth?:  int & >0
	displayHeight?: int & >0

	// Section 5.2.32 - BCP-47 language tag.
	lang?: string

	// Section 5.2.35 - track duration in milliseconds (VOD only).
	trackDuration?: int & >=0

	// Section 5.2.36 / 5.2.37 - publish-track connection details.
	connectionURI?: string
	token?:         string

	// Section 5.2.38-5.2.41 - MoQ Secure Objects encryption.
	encryptionScheme?: string
	cipherSuite?:      string
	keyId?:            string
	trackBaseKey?:     string

	// Section 5.2.42 - authorization requirement (scheme -> config object).
	authInfo?: {...}

	// Section 5.2.44 - accessibility descriptors.
	accessibility?: [...#Accessibility]

	// CMSF Section 3.5.2 - max SAP starting types (0..3).
	maxGrpSapStartingType?: int & >=0 & <=3
	maxObjSapStartingType?: int & >=0 & <=3

	// CMSF Section 4.1.2 - references into contentProtections (resolved by #refCheck).
	contentProtectionRefIDs?: [...string]

	... // allow custom, namespaced track fields

	// --- hidden discriminators -------------------------------------------------
	// Mirror packaging/role but fall back to a sentinel when the source field
	// is absent, so the `if` guards below always evaluate against a concrete
	// value. The default marker (*) is on the real field: when it carries a
	// concrete value that value wins; only when it is absent does the sentinel
	// apply.
	_pkg:  *packaging | "__unset__"
	_role: *role | "__unset__"

	// --- conditional requirements ----------------------------------------------

	// LOC/CMAF/LOCMAF media tracks MUST carry isLive and codec. bitrate is
	// required only for audio and video tracks (Section 5.2.22), which is
	// enforced by the role-based rules below, so it is not required here (a
	// subtitle/caption track legitimately has no bitrate).
	// (Sections 5.2.7, 5.2.18.)
	if _pkg == "loc" || _pkg == "cmaf" || _pkg == "locmaf" {
		isLive!: bool
		codec!:  string
	}

	// LOCMAF tracks MUST carry locmafVersion; only "0.2" is supported for now.
	// ([LOCMAF] draft-einarsson-moq-locmaf.)
	if _pkg == "locmaf" {
		locmafVersion!: "0.2"
	}

	// Audio tracks MUST carry codec, samplerate, channelConfig and bitrate.
	// (Sections 5.2.18, 5.2.28, 5.2.29, 5.2.22.)
	if _role == "audio" {
		codec!:         string
		samplerate!:    int & >0
		channelConfig!: string
		bitrate!:       int & >0
	}

	// Video tracks MUST carry codec and bitrate. (Sections 5.2.18, 5.2.22.)
	if _role == "video" {
		codec!:   string
		bitrate!: int & >0
	}

	// eventType is required for eventtimeline packaging (Section 5.2.5). The
	// converse rule ("eventType MUST NOT appear on other packagings") is
	// enforced as a Go semantic check so the report can carry a clear message.
	if _pkg == "eventtimeline" {
		eventType!: string
	}
}

#Buffers: {
	target?: int & >=0
	min?:    int & >=0
	max?:    int & >=0
	... // unknown keys MUST be ignored
}

#Accessibility: {
	scheme!: string
	value!:  string
	...
}

// -----------------------------------------------------------------------------
// Initialization data
// -----------------------------------------------------------------------------

#InitData: {
	// Section 5.1.7 - unique reference id, type and payload.
	id!:   string
	type!: "inline"
	data!: string
	...
}

// -----------------------------------------------------------------------------
// Content protection (CMSF Section 4.1)
// -----------------------------------------------------------------------------

#UUID: =~"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"

#ContentProtection: {
	refID!: string
	defaultKID!: [...#UUID]
	scheme!:    "cenc" | "cbcs"
	drmSystem!: #DRMSystem
	...
}

#DRMSystem: {
	systemID!:   #UUID
	laURL?:      #URLObject
	certURL?:    #URLObject
	authURL?:    #URLObject
	pssh?:       string
	robustness?: string
	...
}

#URLObject: {
	url!:  string
	type?: string
	...
}
