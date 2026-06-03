package api

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version      string
	Store        *store.Store
	Registration string
	WebFS        fs.FS // sub-FS rooted at the SPA's dist; nil → no SPA serving (test mode)
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))

	loginLimiter := NewRateLimiter(5, 1)
	registerLimiter := NewRateLimiter(3, 1)
	r.With(registerLimiter).Post("/api/auth/register", handleRegister(d))
	r.With(loginLimiter).Post("/api/auth/login", handleLogin(d))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(d.Store))
		r.Get("/api/auth/me", handleMe(d))
		r.Post("/api/auth/logout", handleLogout(d))
		r.Put("/api/auth/password", handlePasswordChange(d))
		r.Get("/api/auth/sessions", handleListSessions(d))
		r.Delete("/api/auth/sessions/{id}", handleRevokeSession(d))
	})

	if d.WebFS != nil {
		fileServer := http.FileServer(http.FS(d.WebFS))
		r.Handle("/assets/*", fileServer)
		r.Get("/*", spaHandler(d.WebFS))
	}
	return r
}

func spaHandler(webFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := webFS.Open("index.html")
		if err != nil {
			http.Error(w, "spa not built", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "spa read failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(data))
	}
}
