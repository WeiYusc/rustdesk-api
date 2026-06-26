package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestCurrentUserAndUserInfoRoutesReturnSameAuthenticatedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)

	isAdmin := true
	user := &model.User{
		Username: "current-user",
		Email:    "current-user@example.test",
		Status:   model.COMMON_STATUS_ENABLE,
		IsAdmin:  &isAdmin,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "current-user-token", ExpiredAt: serviceTokenExpiry()}).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	router := gin.New()
	controller := &User{}
	router.GET("/api/user/info", middleware.RustAuth(), controller.Info)
	router.POST("/api/currentUser", middleware.RustAuth(), controller.Info)

	unauthInfo := getCompatibilityJSON(router, "/api/user/info", "")
	if unauthInfo.Code != http.StatusUnauthorized {
		t.Fatalf("unauth user/info status = %d, want %d; body=%q", unauthInfo.Code, http.StatusUnauthorized, unauthInfo.Body.String())
	}
	unauthCurrent := postCompatibilityJSON(router, "/api/currentUser", `{}`, "")
	if unauthCurrent.Code != http.StatusUnauthorized {
		t.Fatalf("unauth currentUser status = %d, want %d; body=%q", unauthCurrent.Code, http.StatusUnauthorized, unauthCurrent.Body.String())
	}

	userInfo := getCompatibilityJSON(router, "/api/user/info", "current-user-token")
	if userInfo.Code != http.StatusOK {
		t.Fatalf("user/info status = %d, want %d; body=%q", userInfo.Code, http.StatusOK, userInfo.Body.String())
	}
	currentUser := postCompatibilityJSON(router, "/api/currentUser", `{}`, "current-user-token")
	if currentUser.Code != http.StatusOK {
		t.Fatalf("currentUser status = %d, want %d; body=%q", currentUser.Code, http.StatusOK, currentUser.Body.String())
	}
	if userInfo.Body.String() != currentUser.Body.String() {
		t.Fatalf("payload mismatch: user/info=%q currentUser=%q", userInfo.Body.String(), currentUser.Body.String())
	}

	var payload struct {
		Name    string         `json:"name"`
		Email   string         `json:"email"`
		Note    string         `json:"note"`
		IsAdmin *bool          `json:"is_admin"`
		Status  int            `json:"status"`
		Info    map[string]any `json:"info"`
	}
	if err := json.Unmarshal(userInfo.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal user payload: %v; body=%q", err, userInfo.Body.String())
	}
	if payload.Name != user.Username || payload.Email != user.Email || payload.Status != int(model.COMMON_STATUS_ENABLE) {
		t.Fatalf("payload identity/status = %#v", payload)
	}
	if payload.IsAdmin == nil || !*payload.IsAdmin {
		t.Fatalf("payload is_admin = %#v, want true pointer", payload.IsAdmin)
	}
	if payload.Note != "" {
		t.Fatalf("payload note = %q, want empty current field", payload.Note)
	}
	if payload.Info == nil || len(payload.Info) != 0 {
		t.Fatalf("payload info = %#v, want empty object", payload.Info)
	}
}
