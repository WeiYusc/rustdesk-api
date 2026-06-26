package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWebClientSharedPeerRejectsInvalidShareToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing share token",
			body: `{}`,
		},
		{
			name: "empty share token",
			body: `{"share_token":""}`,
		},
		{
			name: "non-string share token",
			body: `{"share_token":123}`,
		},
		{
			name: "malformed json",
			body: `{"share_token":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			controller := &WebClient{}
			router.POST("/api/shared-peer", controller.SharedPeer)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/shared-peer", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"code":101`) {
				t.Fatalf("response body = %q, want code 101", recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "share_token is required") {
				t.Fatalf("response body = %q, want share_token validation error", recorder.Body.String())
			}
		})
	}
}
