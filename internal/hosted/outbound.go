package hosted

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxOutboundAttempts = 8

type outboundDelivery struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	CampaignID  int64  `json:"campaign_id"`
	LeadID      int64  `json:"lead_id"`
	Email       string `json:"email,omitempty"`
	Status      string `json:"status"`
	HTTPStatus  int    `json:"http_status"`
	Error       string `json:"error_message,omitempty"`
	Attempts    int    `json:"attempts"`
	LastAttempt string `json:"last_attempt_at,omitempty"`
}

func (s *Server) outboundHook(workspaceID string) (hookURL, hmacSecret string) {
	key := s.encKey()
	if key == nil {
		return "", ""
	}
	var enc, metadata string
	err := queryRow(s.Store.DB, `
		SELECT encrypted_secret, COALESCE(metadata, '')
		FROM integration_credentials
		WHERE workspace_id = ? AND provider = 'outbound' AND status = 'active'
		ORDER BY id DESC LIMIT 1`, workspaceID).Scan(&enc, &metadata)
	if err != nil {
		return "", ""
	}
	plain, err := Decrypt(key, enc)
	if err != nil || len(plain) == 0 {
		return "", jsonStringField(metadata, "url")
	}
	hookURL = strings.TrimSpace(string(plain))
	if !strings.HasPrefix(hookURL, "http://") && !strings.HasPrefix(hookURL, "https://") {
		hookURL = jsonStringField(metadata, "url")
	}
	if !strings.HasPrefix(hookURL, "http://") && !strings.HasPrefix(hookURL, "https://") {
		return "", jsonStringField(metadata, "hmac_secret")
	}
	return hookURL, jsonStringField(metadata, "hmac_secret")
}

func (s *Server) dispatchOutboundEvents(workspaceID string) {
	hookURL, hmacSecret := s.outboundHook(workspaceID)
	s.deliverPendingEvents(workspaceID, hookURL, hmacSecret)
	s.deliverPendingHandoffs(workspaceID, hookURL, hmacSecret)
}

func (s *Server) deliverPendingEvents(workspaceID, hookURL, hmacSecret string) {
	if hookURL == "" {
		return
	}
	rows, err := query(s.Store.DB, `
		SELECT e.id, e.type, e.campaign_id, e.lead_id, e.timestamp, COALESCE(l.email, '')
		FROM events e
		JOIN campaigns c ON c.id = e.campaign_id
		LEFT JOIN leads l ON l.id = e.lead_id
		WHERE c.workspace_id = ? AND e.type IN ('sent', 'reply', 'bounce')
		AND NOT EXISTS (
			SELECT 1 FROM outbound_deliveries d
			WHERE d.workspace_id = ? AND d.event_id = e.id AND d.kind = e.type
			AND (d.status = 'success' OR d.attempts >= ?)
		)
		ORDER BY e.id ASC
		LIMIT 40`, workspaceID, workspaceID, maxOutboundAttempts)
	if err != nil {
		return
	}
	defer rows.Close()
	type ev struct {
		ID         int64
		Type       string
		CampaignID int64
		LeadID     int64
		Timestamp  time.Time
		Email      string
	}
	var events []ev
	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.ID, &e.Type, &e.CampaignID, &e.LeadID, &e.Timestamp, &e.Email); err != nil {
			return
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return
	}
	rows.Close()
	for _, e := range events {
		payload, _ := json.Marshal(map[string]any{
			"type": e.Type, "campaign_id": e.CampaignID, "lead_id": e.LeadID,
			"email": e.Email, "timestamp": e.Timestamp.UTC().Format(time.RFC3339),
		})
		s.recordDelivery(workspaceID, e.ID, e.Type, e.CampaignID, e.LeadID, payload, hookURL, hmacSecret)
	}
}

func (s *Server) deliverPendingHandoffs(workspaceID, hookURL, hmacSecret string) {
	rows, err := query(s.Store.DB, `
		SELECT id, campaign_id, lead_id, payload, attempts
		FROM outbound_deliveries
		WHERE workspace_id = ? AND kind = 'interested'
		AND (status = 'pending' OR (status = 'failed' AND attempts < ?))
		ORDER BY id ASC LIMIT 20`, workspaceID, maxOutboundAttempts)
	if err != nil {
		return
	}
	defer rows.Close()
	type row struct {
		ID, CampaignID, LeadID int64
		Payload                string
		Attempts               int
	}
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.ID, &r.CampaignID, &r.LeadID, &r.Payload, &r.Attempts) == nil {
			list = append(list, r)
		}
	}
	rows.Close()
	for _, r := range list {
		payload := []byte(r.Payload)
		if hookURL == "" {
			s.finishDelivery(r.ID, "failed", 0, "No outbound webhook connected", r.Attempts+1, r.CampaignID, r.LeadID)
			continue
		}
		status, code, errMsg := postOutbound(hookURL, hmacSecret, payload)
		s.finishDelivery(r.ID, status, code, errMsg, r.Attempts+1, r.CampaignID, r.LeadID)
	}
}

func (s *Server) recordDelivery(workspaceID string, eventID int64, kind string, campaignID, leadID int64, payload []byte, hookURL, hmacSecret string) {
	status, code, errMsg := postOutbound(hookURL, hmacSecret, payload)
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = exec(s.Store.DB, `
		INSERT INTO outbound_deliveries (
			workspace_id, event_id, kind, campaign_id, lead_id, payload, status, http_status, error_message, attempts, last_attempt_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(workspace_id, event_id, kind, campaign_id, lead_id) DO UPDATE SET
			payload = excluded.payload,
			status = excluded.status,
			http_status = excluded.http_status,
			error_message = excluded.error_message,
			attempts = outbound_deliveries.attempts + 1,
			last_attempt_at = excluded.last_attempt_at`,
		workspaceID, eventID, kind, campaignID, leadID, string(payload), status, code, errMsg, now)
}

func (s *Server) finishDelivery(id int64, status string, code int, errMsg string, attempts int, campaignID, leadID int64) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = exec(s.Store.DB, `
		UPDATE outbound_deliveries
		SET status = ?, http_status = ?, error_message = ?, attempts = ?, last_attempt_at = ?
		WHERE id = ?`, status, code, errMsg, attempts, now, id)
	if campaignID == 0 || leadID == 0 {
		return
	}
	handoff := "failed"
	needs := 1
	if status == "success" {
		handoff = "success"
		needs = 0
	}
	_, _ = exec(s.Store.DB, `
		INSERT INTO conversation_state (campaign_id, lead_id, needs_action, handoff_status, last_delivery_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(campaign_id, lead_id) DO UPDATE SET
			handoff_status = excluded.handoff_status,
			last_delivery_id = excluded.last_delivery_id,
			needs_action = excluded.needs_action,
			updated_at = excluded.updated_at`,
		campaignID, leadID, needs, handoff, id, now)
}

func postOutbound(hookURL, hmacSecret string, payload []byte) (status string, httpStatus int, errMsg string) {
	req, err := http.NewRequest(http.MethodPost, hookURL, bytes.NewReader(payload))
	if err != nil {
		return "failed", 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OpenOutreach-outbound")
	if hmacSecret != "" {
		mac := hmac.New(sha256.New, []byte(hmacSecret))
		mac.Write(payload)
		req.Header.Set("X-OpenOutreach-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := outboundHTTPClient.Do(req)
	if err != nil {
		return "failed", 0, err.Error()
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "success", resp.StatusCode, ""
	}
	return "failed", resp.StatusCode, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func (s *Server) queueInterestedHandoff(ws string, campaignID, leadID int64, owner string) (int64, error) {
	var email, company, campaignName, subject, snippet, classification string
	_ = queryRow(s.Store.DB, `
		SELECT COALESCE(l.email,''), COALESCE(l.company,''), c.name
		FROM leads l, campaigns c
		WHERE l.id = ? AND c.id = ? AND c.workspace_id = ?`, leadID, campaignID, ws).Scan(&email, &company, &campaignName)
	_ = queryRow(s.Store.DB, `
		SELECT COALESCE(em.subject,''), COALESCE(em.snippet,'')
		FROM email_messages em
		WHERE em.campaign_id = ? AND em.lead_id = ?
		ORDER BY em.id DESC LIMIT 1`, campaignID, leadID).Scan(&subject, &snippet)
	_ = queryRow(s.Store.DB, `
		SELECT COALESCE(classification,'') FROM reply_classifications
		WHERE campaign_id = ? AND lead_id = ? ORDER BY id DESC LIMIT 1`, campaignID, leadID).Scan(&classification)
	payload, _ := json.Marshal(map[string]any{
		"type":           "interested",
		"campaign_id":    campaignID,
		"lead_id":        leadID,
		"email":          email,
		"company":        company,
		"campaign":       campaignName,
		"subject":        subject,
		"latest_message": snippet,
		"classification": classification,
		"owner_email":    owner,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
	now := time.Now().UTC().Format(time.RFC3339)
	var id int64
	err := queryRow(s.Store.DB, `
		INSERT INTO outbound_deliveries (
			workspace_id, event_id, kind, campaign_id, lead_id, payload, status, attempts, last_attempt_at
		) VALUES (?, 0, 'interested', ?, ?, ?, 'pending', 0, ?)
		ON CONFLICT(workspace_id, event_id, kind, campaign_id, lead_id) DO UPDATE SET
			payload = excluded.payload,
			status = 'pending',
			error_message = '',
			http_status = 0
		RETURNING id`, ws, campaignID, leadID, string(payload), now).Scan(&id)
	if err != nil {
		// SQLite may not return id on conflict; look up.
		_ = queryRow(s.Store.DB, `
			SELECT id FROM outbound_deliveries
			WHERE workspace_id = ? AND event_id = 0 AND kind = 'interested' AND campaign_id = ? AND lead_id = ?`,
			ws, campaignID, leadID).Scan(&id)
		if id == 0 {
			return 0, err
		}
	}
	_, _ = exec(s.Store.DB, `
		INSERT INTO conversation_state (campaign_id, lead_id, owner_email, needs_action, handoff_status, last_delivery_id, updated_at)
		VALUES (?, ?, ?, 1, 'queued', ?, ?)
		ON CONFLICT(campaign_id, lead_id) DO UPDATE SET
			owner_email = CASE WHEN excluded.owner_email = '' THEN conversation_state.owner_email ELSE excluded.owner_email END,
			needs_action = 1,
			handoff_status = 'queued',
			last_delivery_id = excluded.last_delivery_id,
			updated_at = excluded.updated_at`, campaignID, leadID, owner, id, now)
	if owner != "" {
		_, _ = exec(s.Store.DB, `UPDATE conversation_state SET owner_email = ? WHERE campaign_id = ? AND lead_id = ?`, owner, campaignID, leadID)
	}
	return id, nil
}

func (s *Server) handleThreadHandoff(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	cid, _ := strconv.ParseInt(r.PathValue("campaignId"), 10, 64)
	lid, _ := strconv.ParseInt(r.PathValue("leadId"), 10, 64)
	var campaignWS string
	if err := queryRow(s.Store.DB, `SELECT workspace_id FROM campaigns WHERE id = ?`, cid).Scan(&campaignWS); err != nil || campaignWS != ws {
		writeErr(w, http.StatusNotFound, "not_found", "thread not found")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		OwnerEmail string `json:"owner_email"`
		Confirm    bool   `json:"confirm"`
	}
	_ = json.Unmarshal(body, &req)
	if !req.Confirm {
		writeErr(w, http.StatusBadRequest, "confirm_required", "Set confirm=true to route this conversation.")
		return
	}
	id, err := s.queueInterestedHandoff(ws, cid, lid, strings.TrimSpace(req.OwnerEmail))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "handoff_failed", err.Error())
		return
	}
	s.dispatchOutboundEvents(ws)
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"handoff": s.loadDelivery(id), "delivery_id": id,
		"conversation": s.conversationView(cid, lid),
		"next_actions": []string{"retry if failed", "reply in-thread"},
	}})
}

func (s *Server) handleRetryHandoff(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "invalid delivery id")
		return
	}
	var kind, st, ownerWS string
	var campaignID, leadID int64
	if err := queryRow(s.Store.DB, `
		SELECT workspace_id, kind, status, campaign_id, lead_id FROM outbound_deliveries WHERE id = ?`, id).
		Scan(&ownerWS, &kind, &st, &campaignID, &leadID); err != nil || ownerWS != ws {
		writeErr(w, http.StatusNotFound, "not_found", "delivery not found")
		return
	}
	if st == "success" {
		writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"id": id, "status": "success", "message": "already delivered"}})
		return
	}
	_, _ = exec(s.Store.DB, `UPDATE outbound_deliveries SET status = 'pending' WHERE id = ?`, id)
	s.dispatchOutboundEvents(ws)
	writeJSON(w, http.StatusOK, envelope{Data: s.loadDelivery(id)})
}

func (s *Server) handleListHandoffs(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	q := `
		SELECT d.id, d.kind, d.campaign_id, d.lead_id, COALESCE(l.email,''), d.status, d.http_status, d.error_message, d.attempts,
			COALESCE(CAST(d.last_attempt_at AS TEXT),'')
		FROM outbound_deliveries d
		LEFT JOIN leads l ON l.id = d.lead_id
		WHERE d.workspace_id = ? AND d.kind = 'interested'`
	args := []any{ws}
	if status != "" {
		q += ` AND d.status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY d.id DESC LIMIT 100`
	rows, err := query(s.Store.DB, q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()
	list := []outboundDelivery{}
	for rows.Next() {
		var d outboundDelivery
		if rows.Scan(&d.ID, &d.Kind, &d.CampaignID, &d.LeadID, &d.Email, &d.Status, &d.HTTPStatus, &d.Error, &d.Attempts, &d.LastAttempt) == nil {
			list = append(list, d)
		}
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{"handoffs": list}})
}

func (s *Server) handleThreadState(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceFromRequest(r)
	cid, _ := strconv.ParseInt(r.PathValue("campaignId"), 10, 64)
	lid, _ := strconv.ParseInt(r.PathValue("leadId"), 10, 64)
	var campaignWS string
	if err := queryRow(s.Store.DB, `SELECT workspace_id FROM campaigns WHERE id = ?`, cid).Scan(&campaignWS); err != nil || campaignWS != ws {
		writeErr(w, http.StatusNotFound, "not_found", "thread not found")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		OwnerEmail  string `json:"owner_email"`
		NeedsAction *bool  `json:"needs_action"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = exec(s.Store.DB, `
		INSERT INTO conversation_state (campaign_id, lead_id, owner_email, needs_action, updated_at)
		VALUES (?, ?, ?, 0, ?)
		ON CONFLICT(campaign_id, lead_id) DO NOTHING`,
		cid, lid, strings.TrimSpace(req.OwnerEmail), now)
	if req.OwnerEmail != "" {
		_, _ = exec(s.Store.DB, `UPDATE conversation_state SET owner_email = ?, updated_at = ? WHERE campaign_id = ? AND lead_id = ?`,
			strings.TrimSpace(req.OwnerEmail), now, cid, lid)
	}
	if req.NeedsAction != nil {
		needs := 0
		if *req.NeedsAction {
			needs = 1
		}
		_, _ = exec(s.Store.DB, `UPDATE conversation_state SET needs_action = ?, updated_at = ? WHERE campaign_id = ? AND lead_id = ?`,
			needs, now, cid, lid)
	}
	writeJSON(w, http.StatusOK, envelope{Data: map[string]any{
		"campaign_id": cid, "lead_id": lid, "owner_email": req.OwnerEmail,
	}})
}

func (s *Server) loadDelivery(id int64) outboundDelivery {
	var d outboundDelivery
	if id == 0 {
		return d
	}
	_ = queryRow(s.Store.DB, `
		SELECT id, kind, campaign_id, lead_id, status, http_status, error_message, attempts, COALESCE(CAST(last_attempt_at AS TEXT),'')
		FROM outbound_deliveries WHERE id = ?`, id).Scan(
		&d.ID, &d.Kind, &d.CampaignID, &d.LeadID, &d.Status, &d.HTTPStatus, &d.Error, &d.Attempts, &d.LastAttempt)
	return d
}

func isInterestClass(c string) bool {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "positive", "interested", "hot":
		return true
	}
	return false
}

type conversationView struct {
	OwnerEmail     string `json:"owner_email,omitempty"`
	NeedsAction    bool   `json:"needs_action"`
	HandoffStatus  string `json:"handoff_status,omitempty"`
	LastDeliveryID int64  `json:"last_delivery_id,omitempty"`
}

func (s *Server) conversationView(campaignID, leadID int64) conversationView {
	var v conversationView
	var needs int
	err := queryRow(s.Store.DB, `
		SELECT owner_email, needs_action, handoff_status, last_delivery_id
		FROM conversation_state WHERE campaign_id = ? AND lead_id = ?`, campaignID, leadID).
		Scan(&v.OwnerEmail, &needs, &v.HandoffStatus, &v.LastDeliveryID)
	if err != nil {
		return v
	}
	v.NeedsAction = needs != 0
	return v
}

func countFailedHandoffs(db *sql.DB, ws string) int {
	var n int
	_ = queryRow(db, `SELECT COUNT(*) FROM outbound_deliveries WHERE workspace_id = ? AND kind = 'interested' AND status = 'failed'`, ws).Scan(&n)
	return n
}

func countPendingShortlist(db *sql.DB, ws string) int {
	var n int
	_ = queryRow(db, `SELECT COUNT(*) FROM shortlist_candidates WHERE workspace_id = ? AND status = 'pending'`, ws).Scan(&n)
	return n
}

func countDraftsReady(db *sql.DB, ws string) int {
	var n int
	_ = queryRow(db, `SELECT COUNT(*) FROM campaigns WHERE workspace_id = ? AND status = 'draft'`, ws).Scan(&n)
	return n
}

func countNeedsReply(db *sql.DB, ws string) int {
	var n int
	_ = queryRow(db, `
		SELECT COUNT(*) FROM (
			SELECT em.campaign_id, em.lead_id FROM email_messages em
			JOIN campaigns c ON c.id = em.campaign_id
			WHERE c.workspace_id = ? AND em.direction = 'inbound'
			AND em.id IN (
				SELECT MAX(em2.id) FROM email_messages em2
				JOIN campaigns c2 ON c2.id = em2.campaign_id
				WHERE c2.workspace_id = ? AND em2.direction = 'inbound'
				GROUP BY em2.campaign_id, em2.lead_id
			)
			AND NOT EXISTS (
				SELECT 1 FROM email_messages o
				WHERE o.campaign_id = em.campaign_id AND o.lead_id = em.lead_id
				AND o.direction = 'outbound' AND o.occurred_at > em.occurred_at
			)
		)`, ws, ws).Scan(&n)
	return n
}
