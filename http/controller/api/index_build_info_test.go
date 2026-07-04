package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

func TestBuildInfoReturnsMetadataWhenAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	buildPath := filepath.Join(tmp, "build.json")
	if err := os.WriteFile(buildPath, []byte(`{"api_commit":"api456","image":"image:test"}`), 0o644); err != nil {
		t.Fatalf("write build metadata: %v", err)
	}
	t.Setenv("RUSTDESK_API_BUILD_INFO_FILE", buildPath)
	previousService := service.AllService
	t.Cleanup(func() { service.AllService = previousService })
	service.AllService = &service.Service{AppService: &service.AppService{}}

	router := gin.New()
	router.GET("/api/build-info", (&Index{}).BuildInfo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/build-info", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Version   string `json:"version"`
			APICommit string `json:"api_commit"`
			Image     string `json:"image"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, w.Body.String())
	}
	if payload.Code != 0 {
		t.Fatalf("code = %d", payload.Code)
	}
	if payload.Data.APICommit != "api456" {
		t.Fatalf("api commit = %q", payload.Data.APICommit)
	}
	if payload.Data.Image != "image:test" {
		t.Fatalf("image = %q", payload.Data.Image)
	}
}
