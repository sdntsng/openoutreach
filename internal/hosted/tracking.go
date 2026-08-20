package hosted

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type TrackingToken struct {
	Token           string
	Kind            string // open | click
	WorkspaceID     string
	CampaignID      int64
	LeadID          int64
	AccountID       int64
	ScheduledSendID *int64
	MessageID       string
	DestinationURL  string
}

func NewOpaqueToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func SignToken(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

func PutTrackingToken(db *sql.DB, t TrackingToken) error {
	_, err := exec(db, `
		INSERT INTO tracking_tokens (
			token, kind, workspace_id, campaign_id, lead_id, account_id,
			scheduled_send_id, message_id, destination_url
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Token, t.Kind, t.WorkspaceID, t.CampaignID, t.LeadID, t.AccountID,
		t.ScheduledSendID, t.MessageID, t.DestinationURL,
	)
	return err
}

func GetTrackingToken(db *sql.DB, token string) (*TrackingToken, error) {
	var t TrackingToken
	var sched sql.NullInt64
	err := queryRow(db, `
		SELECT token, kind, workspace_id, campaign_id, lead_id, account_id,
			scheduled_send_id, message_id, destination_url
		FROM tracking_tokens WHERE token = ?`, token).Scan(
		&t.Token, &t.Kind, &t.WorkspaceID, &t.CampaignID, &t.LeadID, &t.AccountID,
		&sched, &t.MessageID, &t.DestinationURL,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sched.Valid {
		v := sched.Int64
		t.ScheduledSendID = &v
	}
	return &t, nil
}

func RecordOpenEvent(db *sql.DB, t *TrackingToken, userAgent, country string) error {
	meta := fmt.Sprintf(`{"user_agent":%q,"country":%q}`, userAgent, country)
	_, err := exec(db, `
		INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, message_id, thread_id, metadata)
		VALUES (?, ?, ?, 'open', 0, ?, '', ?)`,
		t.CampaignID, t.LeadID, t.AccountID, t.MessageID, meta,
	)
	if err != nil {
		return err
	}
	var prior int
	_ = queryRow(db, `
		SELECT COUNT(*) FROM events
		WHERE type = 'open' AND campaign_id = ? AND lead_id = ? AND message_id = ?`,
		t.CampaignID, t.LeadID, t.MessageID).Scan(&prior)
	if prior == 1 {
		_, _ = exec(db, `
			INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, message_id, thread_id, metadata)
			VALUES (?, ?, ?, 'unique_open', 0, ?, '', ?)`,
			t.CampaignID, t.LeadID, t.AccountID, t.MessageID, meta,
		)
	}
	return nil
}

func RecordClickEvent(db *sql.DB, t *TrackingToken, userAgent, country string) error {
	meta := fmt.Sprintf(`{"user_agent":%q,"country":%q,"url":%q}`, userAgent, country, t.DestinationURL)
	_, err := exec(db, `
		INSERT INTO events (campaign_id, lead_id, account_id, type, step_number, message_id, thread_id, metadata)
		VALUES (?, ?, ?, 'click', 0, ?, '', ?)`,
		t.CampaignID, t.LeadID, t.AccountID, t.MessageID, meta,
	)
	return err
}

func OpenPixelURL(publicBase, token string) string {
	publicBase = strings.TrimRight(publicBase, "/")
	return fmt.Sprintf("%s/t/o/%s.gif", publicBase, token)
}

func ClickURL(publicBase, messageToken, linkToken string) string {
	publicBase = strings.TrimRight(publicBase, "/")
	return fmt.Sprintf("%s/t/c/%s/%s", publicBase, messageToken, linkToken)
}

func SetHostedKV(db *sql.DB, key, value string) error {
	_, err := exec(db, `
		INSERT INTO hosted_kv (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func GetHostedKV(db *sql.DB, key string) (string, error) {
	var v string
	err := queryRow(db, `SELECT value FROM hosted_kv WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func TouchLastTick(db *sql.DB) error {
	return SetHostedKV(db, "last_successful_tick", time.Now().UTC().Format(time.RFC3339))
}
