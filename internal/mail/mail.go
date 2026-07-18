// Package mail sends transactional email (password-reset codes, etc.) via a
// provider's HTTPS API. HTTPS (443) is the only transport that works across
// all deployments — GCP prod (port 25 blocked), the local test server, and
// the desktop GUI on arbitrary customer networks.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Sender delivers a single plain-text email. Implementations must be safe for
// concurrent use.
type Sender interface {
	Send(ctx context.Context, to, subject, textBody string) error
}

// ResendSender sends through the Resend HTTPS API (https://resend.com).
type ResendSender struct {
	APIKey   string
	From     string
	Endpoint string // defaults to Resend's; overridable for tests
	Client   *http.Client
}

// NewResend builds a ResendSender with sane defaults. apiKey is the secret
// `re_...` token; from must be an address on a domain verified in Resend.
func NewResend(apiKey, from string) *ResendSender {
	return &ResendSender{
		APIKey:   apiKey,
		From:     from,
		Endpoint: "https://api.resend.com/emails",
		Client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *ResendSender) Send(ctx context.Context, to, subject, textBody string) error {
	if r.APIKey == "" || r.From == "" {
		return fmt.Errorf("mail: resend not configured (missing api key or from)")
	}
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = "https://api.resend.com/emails"
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	payload, _ := json.Marshal(map[string]any{
		"from":    r.From,
		"to":      []string{to},
		"subject": subject,
		"text":    textBody,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("mail: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mail: resend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// Message is one email captured by MockSender.
type Message struct {
	To, Subject, Body string
}

// MockSender records messages instead of sending them. Used by tests and as
// the no-op sender when email is disabled.
type MockSender struct {
	Sent []Message
	Err  error // when set, Send returns it
}

func (m *MockSender) Send(_ context.Context, to, subject, textBody string) error {
	if m.Err != nil {
		return m.Err
	}
	m.Sent = append(m.Sent, Message{To: to, Subject: subject, Body: textBody})
	return nil
}
