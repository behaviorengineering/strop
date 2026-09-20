package regenerate

// ShouldSkipMaxVersions returns true when a run should be skipped due to max-versions enforcement.
// Pattern: if not forcing and nextVersion would exceed maxVersions, do not create a new version.
func ShouldSkipMaxVersions(nextVersion, maxVersions int, opts RegenerateOptions) bool {
	if maxVersions < 1 {
		return false
	}
	return !opts.Force && nextVersion > maxVersions
}

// EffectiveMaxVersions returns the effective loop maxVersions for a regenerate run.
//
// When forcing over cap, we allow one "full loop budget" starting at nextVersion:
// effectiveMaxVersions = nextVersion + maxVersions - 1
// so the loop can run up to maxVersions iterations (e.g. v6..v10 when cap=5 and next=6).
func EffectiveMaxVersions(nextVersion, maxVersions int, opts RegenerateOptions) int {
	if maxVersions < 1 {
		maxVersions = 1
	}
	if opts.Force && nextVersion > maxVersions {
		return nextVersion + maxVersions - 1
	}
	return maxVersions
}
