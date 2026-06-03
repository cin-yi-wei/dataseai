package api

import (
	"encoding/json"
	"net/http"
	"time"
)

var startedAt = time.Now()

func handleHealth(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":       true,
			"version":  version,
			"uptime_s": int(time.Since(startedAt).Seconds()),
		})
	}
}
