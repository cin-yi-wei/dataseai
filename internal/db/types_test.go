package db

import "testing"

func TestEngineString(t *testing.T) {
	if EngineMySQL.String() != "mysql" {
		t.Fatalf("EngineMySQL string = %q, want %q", EngineMySQL.String(), "mysql")
	}
}

func TestParseEngineKnown(t *testing.T) {
	e, err := ParseEngine("mysql")
	if err != nil {
		t.Fatalf("ParseEngine(mysql): %v", err)
	}
	if e != EngineMySQL {
		t.Fatalf("got %v, want EngineMySQL", e)
	}
}

func TestParseEngineUnknown(t *testing.T) {
	if _, err := ParseEngine("nonexistent_engine"); err == nil {
		t.Fatal("expected error for unknown engine")
	}
}

func TestParseEnginePostgres(t *testing.T) {
	e, err := ParseEngine("postgres")
	if err != nil {
		t.Fatal(err)
	}
	if e != EnginePostgres {
		t.Fatalf("got %v, want EnginePostgres", e)
	}
}

func TestSSHConfigIsZero(t *testing.T) {
	if !(SSHConfig{}).IsZero() {
		t.Fatal("empty SSHConfig should be zero")
	}
	cfg := SSHConfig{Host: "h", User: "u"}
	if cfg.IsZero() {
		t.Fatal("populated SSHConfig should not be zero")
	}
}
