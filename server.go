package dataseai

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/api"
	"github.com/conray/dataseai/internal/crypto"
	dbpkg "github.com/conray/dataseai/internal/db"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/store"
)

// ServerConfig holds configuration for a DataseAI server instance.
type ServerConfig struct {
	DBPath       string
	KeyPath      string
	MasterKeyHex string // overrides KeyPath when set

	Version      string
	Registration string // "open" | "closed"; defaults to "open"

	QueryTimeoutS int
	HistoryMax    int

	// ForgotPassword enables the unauthenticated self-serve reset. The desktop
	// GUI sets this true; the public server leaves it off.
	ForgotPassword bool

	LLMDefault      string
	AnthropicAPIKey string
	OpenAIAPIKey    string
	GeminiAPIKey    string

	// WebFS serves the SPA frontend. Set to nil to skip static file serving
	// (e.g. when the caller handles static files separately).
	WebFS fs.FS
}

// Server wraps an http.Handler with lifecycle management.
type Server struct {
	handler    http.Handler
	cancelPool context.CancelFunc
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler { return s.handler }

// Close releases background resources (connection pool sweep goroutine).
func (s *Server) Close() { s.cancelPool() }

// NewServer initialises the data store, connection pool, crypto, and HTTP
// router. Returns a Server whose Handler() can be passed to http.Serve.
func NewServer(cfg ServerConfig) (*Server, error) {
	key, _, err := crypto.LoadOrGenerateKey(cfg.MasterKeyHex, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("master key: %w", err)
	}
	cipher, err := crypto.New(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: %w", err)
	}

	sqlDB, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("store open: %w", err)
	}
	if err := store.Migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	reg := cfg.Registration
	if reg == "" {
		reg = "open"
	}

	st := &store.Store{DB: sqlDB}
	pool := dbpkg.NewPool(dbpkg.PoolConfig{IdleTimeout: 5 * time.Minute})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				pool.Sweep(now)
			}
		}
	}()

	h := api.NewRouter(api.Deps{
		Version:       cfg.Version,
		Store:         st,
		Cipher:        cipher,
		Pool:          pool,
		Registration:          reg,
		QueryTimeoutS:         cfg.QueryTimeoutS,
		HistoryMax:            cfg.HistoryMax,
		ForgotPasswordEnabled: cfg.ForgotPassword,
		WebFS:                 cfg.WebFS,
		LLMConfig: llm.Config{
			Default:         cfg.LLMDefault,
			AnthropicAPIKey: cfg.AnthropicAPIKey,
			OpenAIAPIKey:    cfg.OpenAIAPIKey,
			GeminiAPIKey:    cfg.GeminiAPIKey,
		},
	})

	return &Server{handler: h, cancelPool: cancel}, nil
}
