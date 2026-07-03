package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRustdeskLoadKeyFileTrimsWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519.pub")
	if err := os.WriteFile(path, []byte("  test-key\n\t"), 0644); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	rd := Rustdesk{KeyFile: path}
	rd.LoadKeyFile()

	if rd.Key != "test-key" {
		t.Fatalf("loaded key = %q, want trimmed key", rd.Key)
	}
}
