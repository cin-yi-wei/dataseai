package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"

	dataseai "github.com/conray/dataseai"
	"github.com/conray/dataseai/internal/api"
	"github.com/conray/dataseai/internal/config"
	"github.com/conray/dataseai/internal/crypto"
	dbpkg "github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	_ "github.com/conray/dataseai/internal/db/pg"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/mail"
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
	pool := dbpkg.NewPool(dbpkg.PoolConfig{IdleTimeout: 5 * time.Minute})
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
		GeminiAPIKey:    cfg.GeminiAPIKey,
	}

	var mailer mail.Sender
	if cfg.MailAPIKey != "" && cfg.MailFrom != "" {
		mailer = mail.NewResend(cfg.MailAPIKey, cfg.MailFrom)
	}

	r := api.NewRouter(api.Deps{
		Version:               version,
		Store:                 s,
		Cipher:                cipher,
		Pool:                  pool,
		Dialect:               mysqldialect.MySQL{},
		Registration:          cfg.Registration,
		QueryTimeoutS:         cfg.QueryTimeoutS,
		HistoryMax:            cfg.HistoryMax,
		ForgotPasswordEnabled: cfg.ForgotPassword,
		Mailer:                mailer,
		WebFS:                 sub,
		LLMConfig:             llmCfg,
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("dataseai listening on %s (version=%s, key=%s)", addr, version, source)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
