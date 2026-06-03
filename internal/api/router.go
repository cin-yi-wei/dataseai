package api

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/crypto"
	"github.com/conray/mysqlweb/internal/mysql"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version       string
	Store         *store.Store
	Cipher        *crypto.Cipher
	Pool          *mysql.Pool
	QueryRegistry *mysql.Registry
	Registration  string
	QueryTimeoutS int
	HistoryMax    int
	WebFS         fs.FS // sub-FS rooted at the SPA's dist; nil → no SPA serving (test mode)
}

func NewRouter(d Deps) http.Handler {
	if d.QueryTimeoutS == 0 {
		d.QueryTimeoutS = 5
	}
	if d.HistoryMax == 0 {
		d.HistoryMax = 1000
	}
	if d.QueryRegistry == nil {
		d.QueryRegistry = mysql.NewRegistry()
	}
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))
	r.Get("/ws/query", handleWSQuery(d))

	loginLimiter := NewRateLimiter(5, 5.0/60.0)    // burst 5, refill 5/min
	registerLimiter := NewRateLimiter(3, 3.0/60.0) // burst 3, refill 3/min
	r.With(registerLimiter).Post("/api/auth/register", handleRegister(d))
	r.With(loginLimiter).Post("/api/auth/login", handleLogin(d))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(d.Store))
		r.Get("/api/auth/me", handleMe(d))
		r.Post("/api/auth/logout", handleLogout(d))
		passwordLimiter := NewRateLimiter(3, 3.0/60.0) // burst 3, refill 3/min
		r.With(passwordLimiter).Put("/api/auth/password", handlePasswordChange(d))
		r.Get("/api/auth/sessions", handleListSessions(d))
		r.Delete("/api/auth/sessions/{id}", handleRevokeSession(d))
		r.Post("/api/connections", handleCreateConnection(d))
		r.Get("/api/connections", handleListConnections(d))
		r.Get("/api/connections/{id}", handleGetConnection(d))
		r.Put("/api/connections/{id}", handleUpdateConnection(d))
		r.Delete("/api/connections/{id}", handleDeleteConnection(d))
		r.Post("/api/connections/{id}/test", handleTestConnection(d))
		r.Get("/api/db/{connId}/databases", handleListDatabases(d))
		r.Get("/api/db/{connId}/databases/{db}/tables", handleListTables(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/data", handleTableData(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/structure", handleStructure(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/indexes", handleIndexes(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/fks", handleFKs(d))
		r.Patch("/api/db/{connId}/databases/{db}/tables/{table}/rows", handlePatchRow(d))
		r.Post("/api/db/{connId}/databases/{db}/tables/{table}/rows", handleInsertRow(d))
		r.Delete("/api/db/{connId}/databases/{db}/tables/{table}/rows", handleDeleteRow(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/export", handleExport(d))
		r.Post("/api/db/{connId}/databases/{db}/tables/{table}/import", handleImport(d))
		r.Post("/api/query", handleQuery(d))
		r.Get("/api/history", handleListHistory(d))
		r.Delete("/api/history/{id}", handleDeleteHistoryEntry(d))
		r.Delete("/api/history", handleClearHistory(d))
		r.Get("/api/queries/active", handleActiveQueries(d))
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
