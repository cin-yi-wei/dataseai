package api

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/agent"
	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/crypto"
	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version       string
	Store         *store.Store
	Cipher        *crypto.Cipher
	Pool          *db.Pool
	Dialect       db.Dialect
	QueryRegistry *db.KillRegistry
	AgentRegistry *agent.Registry
	Registration  string
	QueryTimeoutS int
	HistoryMax    int
	WebFS         fs.FS // sub-FS rooted at the SPA's dist; nil → no SPA serving (test mode)
	LLMConfig     llm.Config
}

func NewRouter(d Deps) http.Handler {
	if d.QueryTimeoutS == 0 {
		d.QueryTimeoutS = 5
	}
	if d.HistoryMax == 0 {
		d.HistoryMax = 1000
	}
	if d.Dialect == nil {
		d.Dialect = mysqldialect.MySQL{}
	}
	if d.QueryRegistry == nil {
		d.QueryRegistry = db.NewKillRegistry()
	}
	if d.AgentRegistry == nil {
		d.AgentRegistry = agent.NewRegistry()
	}
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))
	r.Get("/agent", agent.Handler(d.AgentRegistry, d.Store))
	r.Get("/ws/query", handleWSQuery(d))
	r.HandleFunc("/ws/chat", handleWSChat(d))

	loginLimiter := NewRateLimiter(5, 5.0/60.0)    // burst 5, refill 5/min
	registerLimiter := NewRateLimiter(3, 3.0/60.0) // burst 3, refill 3/min
	r.With(registerLimiter).Post("/api/auth/register", handleRegister(d))
	r.With(loginLimiter).Post("/api/auth/login", handleLogin(d))
	forgotLimiter := NewRateLimiter(3, 3.0/60.0) // burst 3, refill 3/min
	r.With(forgotLimiter).Post("/api/auth/forgot-password", handleForgotPassword(d))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(d.Store))
		r.Get("/api/auth/me", handleMe(d))
		r.Post("/api/auth/logout", handleLogout(d))
		r.Post("/api/auth/agents", handleCreateAgent(d))
		r.Get("/api/auth/agents", handleListAgents(d))
		r.Delete("/api/auth/agents/{id}", handleDeleteAgent(d))
		passwordLimiter := NewRateLimiter(3, 3.0/60.0) // burst 3, refill 3/min
		r.With(passwordLimiter).Put("/api/auth/password", handlePasswordChange(d))
		r.Get("/api/auth/sessions", handleListSessions(d))
		r.Delete("/api/auth/sessions/{id}", handleRevokeSession(d))
		r.Post("/api/connections", handleCreateConnection(d))
		r.Get("/api/connections", handleListConnections(d))
		r.Post("/api/connections/test-draft", handleTestConnectionDraft(d))
		r.Get("/api/connections/{id}", handleGetConnection(d))
		r.Put("/api/connections/{id}", handleUpdateConnection(d))
		r.Delete("/api/connections/{id}", handleDeleteConnection(d))
		r.Post("/api/connections/{id}/test", handleTestConnection(d))
		r.Get("/api/db/{connId}/databases", handleListDatabases(d))
		r.Get("/api/db/{connId}/databases/{db}/tables", handleListTables(d))
		r.Get("/api/db/{connId}/databases/{db}/schema", handleDBSchema(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/data", handleTableData(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/structure", handleStructure(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/indexes", handleIndexes(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/fks", handleFKs(d))
		r.Patch("/api/db/{connId}/databases/{db}/tables/{table}/rows", handlePatchRow(d))
		r.Post("/api/db/{connId}/databases/{db}/tables/{table}/rows", handleInsertRow(d))
		r.Delete("/api/db/{connId}/databases/{db}/tables/{table}/rows", handleDeleteRow(d))
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/export", handleExport(d))
		r.Post("/api/db/{connId}/databases/{db}/tables/{table}/import", handleImport(d))
		r.Post("/api/db/{connId}/databases/{db}/tables/{table}/truncate", handleTruncateTable(d))
		r.Delete("/api/db/{connId}/databases/{db}/tables/{table}", handleDropTable(d))
		r.Post("/api/query", handleQuery(d))
		r.Get("/api/history", handleListHistory(d))
		r.Delete("/api/history/{id}", handleDeleteHistoryEntry(d))
		r.Delete("/api/history", handleClearHistory(d))
		r.Get("/api/queries/active", handleActiveQueries(d))
		r.Get("/api/auth/api-keys", handleGetAPIKeys(d))
		r.Put("/api/auth/api-keys", handlePutAPIKey(d))
		r.Post("/api/auth/claudecode/start", handleClaudeCodeStart(d))
		r.Post("/api/auth/claudecode/exchange", handleClaudeCodeExchange(d))
		r.Post("/api/auth/claudecode/disconnect", handleClaudeCodeDisconnect(d))
		r.Post("/api/auth/codex/start", handleCodexStart(d))
		r.Post("/api/auth/codex/exchange", handleCodexExchange(d))
		r.Post("/api/auth/codex/disconnect", handleCodexDisconnect(d))
		r.Get("/api/auth/ai-writes", handleGetAIWrites(d))
		r.Put("/api/auth/ai-writes", handlePutAIWrites(d))
		r.Get("/api/auth/ai-policy", handleListAIPolicy(d))
		r.Put("/api/auth/ai-policy", handlePutAIPolicy(d))
		r.Put("/api/auth/ai-policy/batch", handleBatchAIPolicy(d))
		r.Delete("/api/auth/ai-policy", handleDeleteAIPolicy(d))
		r.Get("/api/auth/ai-audit", handleListAIAudit(d))

		// Admin routes (require admin role)
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAdmin())
			r.Get("/api/admin/stats", handleAdminStats(d))
			r.Get("/api/admin/users", handleAdminListUsers(d))
			r.Delete("/api/admin/users/{id}", handleAdminDeleteUser(d))
			r.Patch("/api/admin/users/{id}/admin", handleAdminSetAdmin(d))
			r.Get("/api/admin/connections", handleAdminListConnections(d))
		})
	})

	if d.WebFS != nil {
		fileServer := http.FileServer(http.FS(d.WebFS))
		r.Handle("/assets/*", fileServer)
		r.Handle("/logos/*", fileServer)
		r.Handle("/favicon.ico", fileServer)
		r.Handle("/favicon.svg", fileServer)
		r.Handle("/logo.svg", fileServer)
		r.Handle("/policy-batch-design.html", fileServer)
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
