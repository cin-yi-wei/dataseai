package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("MYSQLWEB_PORT", "")
	t.Setenv("MYSQLWEB_DB_PATH", "")
	t.Setenv("MYSQLWEB_MASTER_KEY", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c.Port != 53306 {
		t.Errorf("Port = %d, want 53306", c.Port)
	}
	if c.DBPath != "/data/dataseai.db" {
		t.Errorf("DBPath = %q, want /data/dataseai.db", c.DBPath)
	}
	if c.Registration != "open" {
		t.Errorf("Registration = %q, want open", c.Registration)
	}
	if c.HistoryMax != 1000 {
		t.Errorf("HistoryMax = %d, want 1000", c.HistoryMax)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	t.Setenv("MYSQLWEB_PORT", "9999")
	t.Setenv("MYSQLWEB_DB_PATH", "/tmp/x.db")
	t.Setenv("MYSQLWEB_REGISTRATION", "closed")
	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c.Port != 9999 {
		t.Errorf("Port = %d", c.Port)
	}
	if c.DBPath != "/tmp/x.db" {
		t.Errorf("DBPath = %q", c.DBPath)
	}
	if c.Registration != "closed" {
		t.Errorf("Registration = %q", c.Registration)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("MYSQLWEB_PORT", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected error, got nil")
	}
}
