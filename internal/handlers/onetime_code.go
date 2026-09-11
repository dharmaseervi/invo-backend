package handlers

import (
	"database/sql"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// maxCodeAttempts burns a one-time code after this many wrong guesses. A six-digit
// code is only a million possibilities, so without a cap it is guessable over the
// network in minutes.
const maxCodeAttempts = 5

// errInvalidCode is returned for wrong, expired, already-used and exhausted codes
// alike. Distinguishing them tells an attacker which addresses have a code
// outstanding and whether they are close to guessing it.
var errInvalidCode = errors.New("invalid or expired code")

// hashOneTimeCode hashes a code for storage. bcrypt rather than SHA-256 on purpose:
// six digits is a small enough space that a fast hash is brute-forced instantly if
// the table ever leaks, and these are verified rarely enough that the cost is free.
func hashOneTimeCode(code string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	return string(hash), err
}

// consumeOneTimeCode verifies a code against the newest unused, unexpired row for this
// address and marks it used. Wrong guesses increment the row's attempt counter.
//
// table is matched against a fixed set rather than interpolated, so it can never carry
// caller-controlled text into the query.
func consumeOneTimeCode(db *sql.DB, table, email, code string) error {
	var selectQuery, attemptQuery, consumeQuery string

	switch table {
	case "otp_codes":
		selectQuery = `SELECT id, code, attempts FROM otp_codes
			WHERE email = $1 AND used = FALSE AND expires_at > NOW()
			ORDER BY created_at DESC LIMIT 1`
		attemptQuery = `UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1`
		consumeQuery = `UPDATE otp_codes SET used = TRUE WHERE id = $1 AND used = FALSE`
	case "password_reset_tokens":
		selectQuery = `SELECT id, code, attempts FROM password_reset_tokens
			WHERE email = $1 AND used = FALSE AND expires_at > NOW()
			ORDER BY created_at DESC LIMIT 1`
		attemptQuery = `UPDATE password_reset_tokens SET attempts = attempts + 1 WHERE id = $1`
		consumeQuery = `UPDATE password_reset_tokens SET used = TRUE WHERE id = $1 AND used = FALSE`
	default:
		return errors.New("unknown one-time code table")
	}

	var id, attempts int
	var storedHash string
	if err := db.QueryRow(selectQuery, email).Scan(&id, &storedHash, &attempts); err != nil {
		return errInvalidCode
	}

	if attempts >= maxCodeAttempts {
		_, _ = db.Exec(consumeQuery, id)
		return errInvalidCode
	}

	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(code)) != nil {
		_, _ = db.Exec(attemptQuery, id)
		return errInvalidCode
	}

	// Guarded by used = FALSE so two requests racing on the same valid code cannot both
	// succeed — exactly one UPDATE reports a row.
	result, err := db.Exec(consumeQuery, id)
	if err != nil {
		return errInvalidCode
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errInvalidCode
	}

	return nil
}
