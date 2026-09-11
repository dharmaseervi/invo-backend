package handlers

// maxPageSize caps any caller-supplied page size. Without a ceiling, ?limit=1000000 is
// accepted and the server builds the whole result set in memory — a denial-of-service
// handle that has nothing to do with how much data a tenant actually has.
const maxPageSize = 200

// clampPageSize applies the default for a missing or nonsensical value and the ceiling
// for an oversized one.
func clampPageSize(requested, fallback int) int {
	if requested <= 0 {
		return fallback
	}
	if requested > maxPageSize {
		return maxPageSize
	}
	return requested
}

// mustAtoi parses a query parameter, treating anything unparseable as zero so the
// caller's clamp applies its default.
func mustAtoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
		if n > 1<<30 {
			return 1 << 30
		}
	}
	return n
}
