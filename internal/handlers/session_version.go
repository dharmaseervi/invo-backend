package handlers

import (
	"context"
	"database/sql"
)

// sessionVersion is the counter a user's live tokens must carry.
//
// It replaces a timestamp comparison that could not be made exact: a JWT's "issued at"
// is whole seconds, so a token minted in the same second as a logout had to be honoured
// — otherwise a fresh sign-in could reject itself — which left a one-second window in
// which a stolen token outlived the logout that was meant to kill it. Logout and
// password reset raise this number instead, and the middleware refuses anything
// carrying an older one.
//
// 0 means "unknown", which the middleware treats as no version check rather than as a
// mismatch, so a token issued before this existed keeps working until it expires.
func sessionVersion(ctx context.Context, db *sql.DB, userID int64) int {
	var v int
	if err := db.QueryRowContext(ctx,
		`SELECT session_version FROM users WHERE id = $1`, userID).Scan(&v); err != nil {
		return 0
	}
	return v
}
