package api

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// NewRateLimiter returns middleware that allows `burst` requests up front and
// refills at `perSec` tokens per second, tracked per source IP.
func NewRateLimiter(burst int, perSec int) func(http.Handler) http.Handler {
	var (
		mu  sync.Mutex
		ips = map[string]*rate.Limiter{}
	)
	get := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := ips[ip]
		if !ok {
			l = rate.NewLimiter(rate.Limit(perSec), burst)
			ips[ip] = l
		}
		return l
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if !get(host).Allow() {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
