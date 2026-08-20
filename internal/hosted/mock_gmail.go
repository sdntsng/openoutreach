package hosted

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andersmyrmel/cold-cli/internal"
)

// MockGmail implements internal.GWSClient for local/dev/tests.
type MockGmail struct {
	mu sync.Mutex

	Sent           []MockSent
	Inbox          []internal.GWSMessage
	SendError      error
	ListError      error
	GetError       error
	ThreadError    error
	RevokedAccounts map[string]bool

	nextMsgID int
}

type MockSent struct {
	Account  string
	To       string
	RawMsg   string
	ThreadID string
	MsgID    string
}

func NewMockGmail() *MockGmail {
	return &MockGmail{
		RevokedAccounts: map[string]bool{},
		nextMsgID:       1000,
	}
}

func (m *MockGmail) SendEmail(account, to, rawMsg, threadID string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.RevokedAccounts[account] {
		return "", "", fmt.Errorf("oauth revoked for %s", account)
	}
	if m.SendError != nil {
		return "", "", m.SendError
	}
	m.nextMsgID++
	msgID := fmt.Sprintf("mock-msg-%d", m.nextMsgID)
	if threadID == "" {
		threadID = fmt.Sprintf("mock-thread-%d", m.nextMsgID)
	}
	m.Sent = append(m.Sent, MockSent{
		Account: account, To: to, RawMsg: rawMsg, ThreadID: threadID, MsgID: msgID,
	})
	return msgID, threadID, nil
}

func (m *MockGmail) ListMessages(account, query string, includeSpamTrash ...bool) ([]internal.GWSMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ListError != nil {
		return nil, m.ListError
	}
	out := make([]internal.GWSMessage, 0, len(m.Inbox))
	for _, msg := range m.Inbox {
		out = append(out, msg)
	}
	return out, nil
}

func (m *MockGmail) GetMessage(account, msgID string) (*internal.GWSMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetError != nil {
		return nil, m.GetError
	}
	for i := range m.Inbox {
		if m.Inbox[i].ID == msgID {
			msg := m.Inbox[i]
			return &msg, nil
		}
	}
	for _, sent := range m.Sent {
		if sent.MsgID == msgID {
			return &internal.GWSMessage{
				ID: sent.MsgID, ThreadID: sent.ThreadID, From: account, To: sent.To,
				Headers: map[string]string{"Message-ID": "<" + sent.MsgID + "@mock.local>"},
				Date:    time.Now().UTC(),
			}, nil
		}
	}
	return nil, fmt.Errorf("message %s not found", msgID)
}

func (m *MockGmail) GetThreadMessages(account, threadID string) ([]internal.GWSMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ThreadError != nil {
		return nil, m.ThreadError
	}
	var out []internal.GWSMessage
	for _, msg := range m.Inbox {
		if msg.ThreadID == threadID {
			out = append(out, msg)
		}
	}
	for _, sent := range m.Sent {
		if sent.ThreadID == threadID {
			out = append(out, internal.GWSMessage{
				ID: sent.MsgID, ThreadID: threadID, From: account, To: sent.To,
				Date: time.Now().UTC(),
			})
		}
	}
	return out, nil
}

// InjectReply adds an inbound reply that matches a sent message via In-Reply-To.
func (m *MockGmail) InjectReply(from, to, subject, inReplyTo, threadID, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextMsgID++
	headers := map[string]string{
		"From": from, "To": to, "Subject": subject, "In-Reply-To": inReplyTo,
	}
	m.Inbox = append(m.Inbox, internal.GWSMessage{
		ID:        fmt.Sprintf("mock-reply-%d", m.nextMsgID),
		ThreadID:  threadID,
		Snippet:   body,
		TextBody:  body,
		Headers:   headers,
		From:      from,
		To:        to,
		Subject:   subject,
		InReplyTo: inReplyTo,
		Date:      time.Now().UTC(),
		LabelIDs:  []string{"INBOX"},
	})
}

func (m *MockGmail) InjectBounce(bouncedEmail, originalMessageID, threadID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextMsgID++
	snippet := fmt.Sprintf("Delivery Status Notification\nX-Failed-Recipients: %s\n", bouncedEmail)
	m.Inbox = append(m.Inbox, internal.GWSMessage{
		ID:       fmt.Sprintf("mock-bounce-%d", m.nextMsgID),
		ThreadID: threadID,
		Snippet:  snippet,
		TextBody: snippet,
		Headers: map[string]string{
			"From":        "mailer-daemon@google.com",
			"Subject":     "Delivery Status Notification (Failure)",
			"In-Reply-To": originalMessageID,
		},
		From:     "mailer-daemon@google.com",
		Subject:  "Delivery Status Notification (Failure)",
		InReplyTo: originalMessageID,
		Date:     time.Now().UTC(),
		LabelIDs: []string{"INBOX"},
	})
}

func (m *MockGmail) LastSentThreadID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Sent) == 0 {
		return ""
	}
	return m.Sent[len(m.Sent)-1].ThreadID
}

func (m *MockGmail) ExtractRFCMessageID(raw string) string {
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "message-id:") {
			return strings.TrimSpace(line[len("Message-ID:"):])
		}
	}
	// raw may be base64; return empty and let tests use mock IDs
	return ""
}
