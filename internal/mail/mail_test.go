package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendSend_PostsExpectedRequest(t *testing.T) {
	var gotAuth, gotCT string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	}))
	defer srv.Close()

	s := NewResend("re_test", "no-reply@conray.top")
	s.Endpoint = srv.URL
	if err := s.Send(context.Background(), "user@example.com", "code", "your code is 123456"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotAuth != "Bearer re_test" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if body["from"] != "no-reply@conray.top" || body["subject"] != "code" {
		t.Errorf("payload = %+v", body)
	}
	if to, _ := body["to"].([]any); len(to) != 1 || to[0] != "user@example.com" {
		t.Errorf("to = %+v", body["to"])
	}
}

func TestResendSend_ErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"domain not verified"}`))
	}))
	defer srv.Close()
	s := NewResend("re_test", "no-reply@conray.top")
	s.Endpoint = srv.URL
	if err := s.Send(context.Background(), "u@example.com", "s", "b"); err == nil {
		t.Fatal("expected error on 422")
	}
}

func TestResendSend_UnconfiguredErrors(t *testing.T) {
	s := NewResend("", "")
	if err := s.Send(context.Background(), "u@example.com", "s", "b"); err == nil {
		t.Fatal("expected error when unconfigured")
	}
}

func TestMockSender_Records(t *testing.T) {
	m := &MockSender{}
	_ = m.Send(context.Background(), "a@b.com", "sub", "body")
	if len(m.Sent) != 1 || m.Sent[0].To != "a@b.com" || m.Sent[0].Subject != "sub" {
		t.Fatalf("recorded = %+v", m.Sent)
	}
}
