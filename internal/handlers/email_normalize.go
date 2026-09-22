package handlers

import (
	"regexp"
	"strings"
)

// normalizeEmail is applied to every email a sign-in route receives, before it is
// validated, stored or looked up.
//
// Register only accepted lower case, so "Ravi@Example.com" typed on a phone that
// capitalises the first letter was refused as "invalid email format", and the same
// address typed two ways could never match. Lookups compare lower(email) as well, so an
// account stored with capitals before that rule existed still signs in.
func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// The same pattern Register has always used, minus its four-letter cap on the ending,
// which refused addresses such as name@shop.online.
var emailPattern = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)

// validEmail checks an address after normalizeEmail. The format used to be checked by
// gin's binding, which runs before normalising — so a trailing space, which phone
// keyboards add after autocompleting an address, was refused as "invalid input".
func validEmail(s string) bool { return emailPattern.MatchString(s) }
