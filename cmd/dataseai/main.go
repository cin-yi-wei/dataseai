package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	dataseai "github.com/conray/dataseai"
	"github.com/conray/dataseai/internal/api"
	"github.com/conray/dataseai/internal/config"
	"github.com/conray/dataseai/internal/crypto"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/mcp"
	mysqlpkg "github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	keyPath := filepath.Join(filepath.Dir(cfg.DBPath), "master.key")
	key, source, err := crypto.LoadOrGenerateKey(cfg.MasterKeyHex, keyPath)
	if err != nil {
		log.Fatalf("master key: %v", err)
	}
	if source == "generated" {
		log.Printf("⚠ generated new master key at %s — set MYSQLWEB_MASTER_KEY in env to persist", keyPath)
	}
	cipher, err := crypto.New(key)
	if err != nil {
		log.Fatalf("crypto init: %v", err)
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("store open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	s := &store.Store{DB: db}
	pool := mysqlpkg.NewPool(mysqlpkg.PoolConfig{IdleTimeout: 5 * time.Minute})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for now := range ticker.C {
			pool.Sweep(now)
		}
	}()

	sub, err := fs.Sub(dataseai.WebFS, "web/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	llmCfg := llm.Config{
		Default:         cfg.LLMDefault,
		AnthropicAPIKey: cfg.AnthropicAPIKey,
		OpenAIAPIKey:    cfg.OpenAIAPIKey,
	}

	// Optional MCP subprocess. MYSQLWEB_MCP_COMMAND is the shell-style command
	// to spawn (e.g. "npx -y @askdba/mcp-server-mysql" or the path to a Go
	// binary). Whitespace-tokenised; for anything more elaborate (env vars,
	// shell expansion) wrap it in a script. MYSQL_MCP_EXTENDED=1 is added to
	// the child env automatically so askdba exposes add_connection.
	var mcpClient *mcp.Client
	if cmd := strings.TrimSpace(os.Getenv("MYSQLWEB_MCP_COMMAND")); cmd != "" {
		parts := strings.Fields(cmd)
		childEnv := append(os.Environ(), "MYSQL_MCP_EXTENDED=1")
		c, err := mcp.Spawn(context.Background(), parts[0], parts[1:], childEnv)
		if err != nil {
			log.Printf("⚠ MCP spawn failed (%s): %v — chat will use direct-tools fallback", cmd, err)
		} else {
			mcpClient = c
			log.Printf("MCP subprocess running: %s", cmd)
		}
	}

	r := api.NewRouter(api.Deps{
		Version:       version,
		Store:         s,
		Cipher:        cipher,
		Pool:          pool,
		Registration:  cfg.Registration,
		QueryTimeoutS: cfg.QueryTimeoutS,
		HistoryMax:    cfg.HistoryMax,
		WebFS:         sub,
		LLMConfig:     llmCfg,
		MCP:           mcpClient,
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("dataseai listening on %s (version=%s, key=%s)", addr, version, source)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
