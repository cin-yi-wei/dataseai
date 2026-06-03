package api

import (
	"net/http"

	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version      string
	Store        *store.Store
	Registration string // "open" | "closed"
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))
	r.Post("/api/auth/register", handleRegister(d))
	r.Post("/api/auth/login", handleLogin(d))
	return r
}
