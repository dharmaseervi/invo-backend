package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Every request gets a deadline.
//
// Without one, a query that goes slow holds its database connection until it finishes,
// however long that takes, and the caller may have hung up ten minutes earlier. On a
// pool of a couple of dozen connections it takes surprisingly few of those to stop the
// shop billing anybody — the symptom is the whole app going unresponsive at once, for
// reasons that have nothing to do with what anybody was doing.
//
// This bounds the request's context, so anything built on it — every query that takes a
// context — is cancelled when the time runs out or when the client disconnects. It does
// not by itself stop a handler that ignores its context, which is why the queries were
// given one too.

// RequestDeadline bounds every request to `limit`.
//
// PDFs are the exception worth knowing about: rendering a long statement is slower than
// a query, so the deadline has to be generous enough for the work the server genuinely
// does rather than tuned to the fastest endpoint.
func RequestDeadline(limit time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), limit)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
