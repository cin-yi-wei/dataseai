package crypto

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerateKey_FromHex(t *testing.T) {
	dir := t.TempDir()
	hexKey := hex.EncodeToString(make([]byte, 32))
	key, source, err := LoadOrGenerateKey(hexKey, filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("key len = %d", len(key))
	}
	if source != "env" {
		t.Fatalf("source = %q, want env", source)
	}
}

func TestLoadOrGenerateKey_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	hexKey := hex.EncodeToString(make([]byte, 32))
	if err := os.WriteFile(path, []byte(hexKey), 0600); err != nil {
		t.Fatal(err)
	}
	_, source, err := LoadOrGenerateKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if source != "file" {
		t.Fatalf("source = %q, want file", source)
	}
}

func TestLoadOrGenerateKey_GeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	key1, source, err := LoadOrGenerateKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if source != "generated" {
		t.Fatalf("source = %q, want generated", source)
	}
	key2, source, err := LoadOrGenerateKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if source != "file" {
		t.Fatalf("second source = %q, want file", source)
	}
	if string(key1) != string(key2) {
		t.Fatal("persisted key differs from generated")
	}
}

func TestLoadOrGenerateKey_BadHexFails(t *testing.T) {
	dir := t.TempDir()
	_, _, err := LoadOrGenerateKey("not-hex-at-all", filepath.Join(dir, "master.key"))
	if err == nil {
		t.Fatal("expected error for bad hex")
	}
}

func TestLoadOrGenerateKey_FromFile_TolerantOfTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	hexKey := hex.EncodeToString(make([]byte, 32))
	// simulate `echo $HEX > master.key` — trailing newline
	if err := os.WriteFile(path, []byte(hexKey+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, source, err := LoadOrGenerateKey("", path)
	if err != nil {
		t.Fatalf("trim should make this work: %v", err)
	}
	if source != "file" {
		t.Fatalf("source = %q, want file", source)
	}
}

func TestLoadOrGenerateKey_FileReadErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "master.key")
	// make the parent unreadable so os.ReadFile fails with permission-denied,
	// NOT not-exist
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("zz"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(path), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0700) })
	_, _, err := LoadOrGenerateKey("", path)
	if err == nil {
		t.Fatal("expected error when key file unreadable, got nil")
	}
}
