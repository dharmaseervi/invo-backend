package services

import "testing"

// The three ways this deployment can be misconfigured, each of which previously
// surfaced only as a generic "Failed to send verification email" on every signup.
func TestEmailConfigErrorNamesTheMissingSetting(t *testing.T) {
	cases := []struct {
		name, key, from string
		wantErr         bool
	}{
		{"no api key", "", "noreply@example.com", true},
		{"no from address", "re_test", "", true},
		{"from is not an address", "re_test", "invobilling.com", true},
		{"configured", "re_test", "noreply@example.com", false},
	}
	for _, c := range cases {
		err := NewEmailService(c.key, c.from, "Invo").configError()
		if (err != nil) != c.wantErr {
			t.Errorf("%s: got err=%v, want error=%v", c.name, err, c.wantErr)
		}
	}
}

// Whitespace pasted into a hosting dashboard's env field is a real failure mode.
func TestEmailConfigTrimsWhitespace(t *testing.T) {
	s := NewEmailService("  re_test  ", "  noreply@example.com  ", " Invo ")
	if err := s.configError(); err != nil {
		t.Fatalf("padded values should be accepted: %v", err)
	}
	if s.fromEmail != "noreply@example.com" {
		t.Errorf("fromEmail = %q, want trimmed", s.fromEmail)
	}
}
