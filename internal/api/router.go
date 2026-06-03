package api

import (
	"net/http"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version      string
	Store        *store.Store
	Registration string
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))
	r.Post("/api/auth/register", handleRegister(d))
	r.Post("/api/auth/login", handleLogin(d))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(d.Store))
		r.Get("/api/auth/me", handleMe(d))
		r.Post("/api/auth/logout", handleLogout(d))
		r.Put("/api/auth/password", handlePasswordChange(d))
	})
	return r
}
