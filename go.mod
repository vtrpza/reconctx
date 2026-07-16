module github.com/vtrpza/reconctx

go 1.24.0

require (
	github.com/google/jsonschema-go v0.4.3 // Draft 2020-12 validation for emitted handoff contracts.
	golang.org/x/net v0.48.0 // UTS #46 non-transitional IDNA required by compatibility vectors.
	golang.org/x/sys v0.41.0 // openat2 and renameat2 provide Linux-architecture-safe rooted filesystem operations.
	golang.org/x/text v0.32.0 // NFC normalization required by url-canonicalization/v0.
)

require go.yaml.in/yaml/v3 v3.0.4
