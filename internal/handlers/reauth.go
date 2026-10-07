package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	utils "invo-server/internal/util"

	"github.com/gin-gonic/gin"
)

// requireReauth reports whether a sensitive action must carry the account password.
//
// Off by default, because the iPhone app in the store cannot send one yet and its
// Delete account screen has to keep working. Turn it on (REQUIRE_REAUTH=true) once an
// app version that asks for the password is out everywhere.
func requireReauth() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("REQUIRE_REAUTH")), "true")
}

// confirmPassword checks the "password" field in the request body against the account,
// for actions where a stolen session alone should not be enough: deleting everything,
// or changing the bank details printed on invoices.
//
// A password that is sent must be correct. A missing one is allowed only while
// REQUIRE_REAUTH is off. It writes the response and returns an error when the caller
// must be refused, so handlers can `if err := h.confirmPassword(...); err != nil { return }`.
//
// The body is restored afterwards: handlers bind it after this runs.
func confirmAccountPassword(c *gin.Context, db *sql.DB, userID int) error {
	password, err := passwordFromBody(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return err
	}

	if password == "" {
		if requireReauth() {
			c.JSON(http.StatusForbidden, gin.H{
				"error":          "Enter your password to confirm this.",
				"needs_password": true,
			})
			return errReauthRequired
		}
		return nil
	}

	var hash string
	if err := db.QueryRowContext(c.Request.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return err
	}
	if !utils.CheckPasswordHash(password, hash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "That password is not right."})
		return errWrongPassword
	}
	return nil
}

type reauthError string

func (e reauthError) Error() string { return string(e) }

const (
	errReauthRequired = reauthError("password required")
	errWrongPassword  = reauthError("wrong password")
)

// passwordFromBody peeks at the JSON body for a password and puts the body back.
func passwordFromBody(c *gin.Context) (string, error) {
	if c.Request.Body == nil {
		return "", nil
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return "", err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		// Not JSON, or JSON this route will reject anyway — no password to check.
		return "", nil
	}
	return payload.Password, nil
}
