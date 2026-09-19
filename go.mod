module github.com/slashdevops/machineid

go 1.27.1

// v0.20.0 was a mistyped tag for v0.2.0 (same commit). It is already in the
// Go module proxy and checksum database, where it cannot be removed, and it
// sorts above every real 0.x release, so `@latest` would resolve to it forever.
// v0.20.1 exists only to carry this directive; both are retracted.
retract (
	v0.20.1 // retraction-only release, contains no changes
	v0.20.0 // mistyped tag, use v0.2.0
)
