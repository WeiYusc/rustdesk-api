package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGetBuildInfoReadsFullS6MetadataFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "build.json")
	raw := `{
		"server_commit":"server123",
		"api_commit":"api456",
		"web_commit":"web789",
		"image":"ghcr.io/example/full-s6:test",
		"built_at":"2026-07-04T03:10:02Z"
	}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write build info: %v", err)
	}

	info := (&AppService{}).GetBuildInfoFromPath(path)

	if info.ServerCommit != "server123" {
		t.Fatalf("ServerCommit = %q", info.ServerCommit)
	}
	if info.APICommit != "api456" {
		t.Fatalf("APICommit = %q", info.APICommit)
	}
	if info.WebCommit != "web789" {
		t.Fatalf("WebCommit = %q", info.WebCommit)
	}
	if info.Image != "ghcr.io/example/full-s6:test" {
		t.Fatalf("Image = %q", info.Image)
	}
	if info.BuiltAt != "2026-07-04T03:10:02Z" {
		t.Fatalf("BuiltAt = %q", info.BuiltAt)
	}
	if info.Source != path {
		t.Fatalf("Source = %q, want %q", info.Source, path)
	}
}

func TestGetBuildInfoReturnsEmptyWhenMetadataMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	info := (&AppService{}).GetBuildInfoFromPath(path)

	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal build info: %v", err)
	}
	if string(encoded) != `{"version":"","server_commit":"","api_commit":"","web_commit":"","image":"","built_at":"","source":""}` {
		t.Fatalf("unexpected build info json: %s", encoded)
	}
}
