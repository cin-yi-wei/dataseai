package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimit_BlocksAfterBurst(t *testing.T) {
	mw := NewRateLimiter(3, 1)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	hit := func() int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if hit() != 200 || hit() != 200 || hit() != 200 {
		t.Fatal("first 3 should succeed")
	}
	if hit() != http.StatusTooManyRequests {
		t.Fatal("4th should be 429")
	}
}

func TestRateLimit_PerIP(t *testing.T) {
	mw := NewRateLimiter(1, 1)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	hit := func(addr string) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = addr + ":1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if hit("10.0.0.1") != 200 || hit("10.0.0.2") != 200 {
		t.Fatal("first hit per IP should pass")
	}
	if hit("10.0.0.1") != 429 {
		t.Fatal("10.0.0.1 second should be 429")
	}
	if hit("10.0.0.2") != 429 {
		t.Fatal("10.0.0.2 second should be 429")
	}
}
