package services

import (
	"database/sql"
	"encoding/base64"
	"log"

	"invo-server/internal/config"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
)

// PushService sends APNs push notifications to a user's registered devices.
// When APNs isn't configured (no key/team/bundle ID set), Send* calls are a
// harmless no-op with a log line — this lets the rest of the app call them
// unconditionally without needing to check "is push configured?" everywhere.
type PushService struct {
	db       *sql.DB
	client   *apns2.Client
	bundleID string
	enabled  bool
}

func NewPushService(db *sql.DB, cfg *config.Config) *PushService {
	s := &PushService{db: db, bundleID: cfg.APNs.BundleID}

	if cfg.APNs.KeyID == "" || cfg.APNs.TeamID == "" || cfg.APNs.BundleID == "" || cfg.APNs.KeyBase64 == "" {
		log.Println("⚠️  APNs not configured — push notifications are disabled (set APNS_KEY_ID, APNS_TEAM_ID, APNS_BUNDLE_ID, APNS_KEY_BASE64 to enable)")
		return s
	}

	keyBytes, err := base64.StdEncoding.DecodeString(cfg.APNs.KeyBase64)
	if err != nil {
		log.Println("⚠️  APNS_KEY_BASE64 is not valid base64 — push notifications are disabled:", err)
		return s
	}

	authKey, err := token.AuthKeyFromBytes(keyBytes)
	if err != nil {
		log.Println("⚠️  Failed to parse APNs auth key — push notifications are disabled:", err)
		return s
	}

	tok := &token.Token{
		AuthKey: authKey,
		KeyID:   cfg.APNs.KeyID,
		TeamID:  cfg.APNs.TeamID,
	}

	client := apns2.NewTokenClient(tok)
	if cfg.APNs.Production {
		client = client.Production()
	} else {
		client = client.Development()
	}

	s.client = client
	s.enabled = true
	log.Println("✅ APNs configured (production =", cfg.APNs.Production, ")")
	return s
}

// RegisterToken saves (or refreshes) a device token for a user.
func (s *PushService) RegisterToken(userID int, deviceToken string) error {
	_, err := s.db.Exec(`
		INSERT INTO device_tokens (user_id, token, platform)
		VALUES ($1, $2, 'ios')
		ON CONFLICT (token) DO UPDATE SET user_id = $1
	`, userID, deviceToken)
	return err
}

// UnregisterToken removes a device token (called on logout).
func (s *PushService) UnregisterToken(deviceToken string) error {
	_, err := s.db.Exec(`DELETE FROM device_tokens WHERE token = $1`, deviceToken)
	return err
}

// SendToUser pushes a notification to every device registered for a user.
// Errors are logged, not returned — a failed push should never fail the
// business action (recording a payment, issuing an invoice) that triggered it.
func (s *PushService) SendToUser(userID int, title, body string) {
	if !s.enabled {
		return
	}

	rows, err := s.db.Query(`SELECT token FROM device_tokens WHERE user_id = $1`, userID)
	if err != nil {
		log.Println("push: failed to load device tokens:", err)
		return
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			tokens = append(tokens, t)
		}
	}

	for _, deviceToken := range tokens {
		notification := &apns2.Notification{
			DeviceToken: deviceToken,
			Topic:       s.bundleID,
			Payload:     payload.NewPayload().AlertTitle(title).AlertBody(body).Sound("default"),
		}

		res, err := s.client.Push(notification)
		if err != nil {
			log.Println("push: send failed:", err)
			continue
		}

		// BadDeviceToken / Unregistered → the app was deleted or the token rotated; stop
		// trying that token instead of failing forever on every future push.
		if res.Reason == "BadDeviceToken" || res.Reason == "Unregistered" {
			_, _ = s.db.Exec(`DELETE FROM device_tokens WHERE token = $1`, deviceToken)
		}
	}
}
