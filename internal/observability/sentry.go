// Package observability wires optional error reporting.
//
// Everything here is inert unless SENTRY_DSN is set, so the code ships without
// requiring an account, a key, or any third-party service to be reachable.
package observability

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

// Init starts error reporting if SENTRY_DSN is configured, and reports whether it did.
// A missing DSN is normal, not an error — it simply means reporting is off.
func Init(environment, release string) bool {
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if dsn == "" {
		return false
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: environment,
		Release:     release,

		// Off by default. This is a billing API: request bodies carry invoice
		// amounts, client names and addresses, and none of that belongs in a
		// third-party error tracker.
		SendDefaultPII: false,

		// Errors only. Performance tracing on every request would burn the free
		// tier's quota in days and tells us nothing we cannot measure directly.
		EnableTracing: false,

		BeforeSend: scrubSensitive,
	})
	if err != nil {
		// Reporting failing to start must never stop the server from serving.
		log.Println("sentry: disabled, failed to initialise:", err)
		return false
	}

	log.Println("sentry: error reporting enabled")
	return true
}

// Middleware reports panics and leaves the request to continue as before. Repanic is
// true so Gin's own Recovery still runs and the client still receives a 500 — error
// reporting must not change what the caller sees.
func Middleware() gin.HandlerFunc {
	return sentrygin.New(sentrygin.Options{Repanic: true})
}

// Flush gives queued events a moment to send during shutdown, since the process is
// about to exit and anything unsent is lost.
func Flush() {
	sentry.Flush(2 * time.Second)
}

// scrubSensitive strips credentials before an event leaves the process.
//
// The Gin integration attaches request headers, which include Authorization — sending
// those would hand a working bearer token to a third party on every reported error.
// Cookies and query strings go the same way, since a query string can carry a
// password reset code.
func scrubSensitive(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event.Request != nil {
		if event.Request.Headers != nil {
			for name := range event.Request.Headers {
				switch strings.ToLower(name) {
				case "authorization", "cookie", "set-cookie", "x-api-key":
					event.Request.Headers[name] = "[redacted]"
				}
			}
		}
		event.Request.Cookies = ""
		event.Request.QueryString = ""
		event.Request.Data = ""
	}
	return event
}

// ReportServerErrors reports handled 5xx responses, not just panics.
//
// The panic middleware above only fires when a handler crashes. Almost every real
// failure in this API is caught and turned into a 500 — a query that failed, a
// transaction that could not commit — and those are precisely the ones worth an
// alert. Without this they only ever reached stdout, where nobody was looking.
//
// The message is built from the route pattern rather than the URL, so every failure
// of the same endpoint groups into one Sentry issue instead of one per invoice id.
func ReportServerErrors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		status := c.Writer.Status()
		if status < 500 {
			return
		}

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		hub := sentrygin.GetHubFromContext(c)
		if hub == nil {
			hub = sentry.CurrentHub()
		}

		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelError)
			scope.SetTag("route", route)
			scope.SetTag("method", c.Request.Method)
			scope.SetTag("status", strconv.Itoa(status))
			// The user id is an integer we issued, not personal data, and it is the
			// difference between "something is broken" and "this tenant is broken".
			if userID := c.GetInt("user_id"); userID != 0 {
				scope.SetUser(sentry.User{ID: strconv.Itoa(userID)})
			}

			// Whatever the handler recorded with c.Error carries the underlying
			// cause; the status line alone rarely says why.
			if len(c.Errors) > 0 {
				scope.SetContext("handler", sentry.Context{"errors": c.Errors.String()})
			}

			hub.CaptureMessage(c.Request.Method + " " + route + " responded " + strconv.Itoa(status))
		})
	}
}
