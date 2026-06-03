package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"

	mysqlweb "github.com/conray/mysqlweb"
	"github.com/conray/mysqlweb/internal/api"
	"github.com/conray/mysqlweb/internal/config"
	"github.com/conray/mysqlweb/internal/crypto"
	mysqlpkg "github.com/conray/mysqlweb/internal/mysql"
	"github.com/conray/mysqlweb/internal/store"
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

	sub, err := fs.Sub(mysqlweb.WebFS, "web/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
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
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("mysqlweb listening on %s (version=%s, key=%s)", addr, version, source)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
