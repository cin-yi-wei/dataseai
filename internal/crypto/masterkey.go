package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrGenerateKey returns a 32-byte AES key. Sources in priority order:
//  1. envHex non-empty → decode hex and return
//  2. file at path exists → read hex and return
//  3. generate fresh 32 bytes, write to path (0600), return
//
// Source string is one of: "env", "file", "generated".
func LoadOrGenerateKey(envHex, path string) ([]byte, string, error) {
	if envHex != "" {
		k, err := hex.DecodeString(envHex)
		if err != nil {
			return nil, "", fmt.Errorf("decode env hex: %w", err)
		}
		if len(k) != 32 {
			return nil, "", ErrKeyLength
		}
		return k, "env", nil
	}
	if data, err := os.ReadFile(path); err == nil {
		k, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, "", fmt.Errorf("decode file hex: %w", err)
		}
		if len(k) != 32 {
			return nil, "", ErrKeyLength
		}
		return k, "file", nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, "", fmt.Errorf("read master key file: %w", err)
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(k)), 0600); err != nil {
		return nil, "", err
	}
	return k, "generated", nil
}
