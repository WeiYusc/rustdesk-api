package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestLogoutRequiresAuthDeletesTokenAndUnbindsMatchingPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)

	isAdmin := false
	user := &model.User{
		Username: "logout-user",
		Status:   model.COMMON_STATUS_ENABLE,
		IsAdmin:  &isAdmin,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	otherUser := &model.User{
		Username: "other-user",
		Status:   model.COMMON_STATUS_ENABLE,
		IsAdmin:  &isAdmin,
	}
	if err := db.Create(otherUser).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	if err := db.Create(&model.UserToken{
		UserId:     user.Id,
		Token:      "logout-token",
		DeviceUuid: "logout-uuid",
		DeviceId:   "logout-peer",
		ExpiredAt:  time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("create logout token: %v", err)
	}
	if err := db.Create(&model.UserToken{
		UserId:     user.Id,
		Token:      "other-token",
		DeviceUuid: "other-uuid",
		DeviceId:   "other-peer",
		ExpiredAt:  time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("create other token: %v", err)
	}
	if err := db.Create(&model.Peer{Id: "logout-peer", Uuid: "logout-uuid", UserId: user.Id}).Error; err != nil {
		t.Fatalf("create logout peer: %v", err)
	}
	if err := db.Create(&model.Peer{Id: "other-peer", Uuid: "other-uuid", UserId: user.Id}).Error; err != nil {
		t.Fatalf("create other peer: %v", err)
	}
	if err := db.Create(&model.Peer{Id: "same-uuid-other-user", Uuid: "logout-uuid", UserId: otherUser.Id}).Error; err != nil {
		t.Fatalf("create other user peer: %v", err)
	}

	router := gin.New()
	router.POST("/api/logout", middleware.RustAuth(), (&Login{}).Logout)

	unauthenticated := postCompatibilityJSON(router, "/api/logout", `{}`, "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusUnauthorized, unauthenticated.Body.String())
	}

	logout := postCompatibilityJSON(router, "/api/logout", `{}`, "logout-token")
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d; body=%q", logout.Code, http.StatusOK, logout.Body.String())
	}
	if logout.Body.String() != "null" {
		t.Fatalf("logout body = %q, want null", logout.Body.String())
	}

	var matchingTokenCount int64
	if err := db.Model(&model.UserToken{}).Where("user_id = ? and token = ?", user.Id, "logout-token").Count(&matchingTokenCount).Error; err != nil {
		t.Fatalf("count matching token: %v", err)
	}
	if matchingTokenCount != 0 {
		t.Fatalf("matching token count = %d, want 0", matchingTokenCount)
	}

	var otherTokenCount int64
	if err := db.Model(&model.UserToken{}).Where("user_id = ? and token = ?", user.Id, "other-token").Count(&otherTokenCount).Error; err != nil {
		t.Fatalf("count other token: %v", err)
	}
	if otherTokenCount != 1 {
		t.Fatalf("other token count = %d, want 1", otherTokenCount)
	}

	var logoutPeer model.Peer
	if err := db.Where("id = ?", "logout-peer").First(&logoutPeer).Error; err != nil {
		t.Fatalf("find logout peer: %v", err)
	}
	if logoutPeer.UserId != 0 {
		t.Fatalf("logout peer user_id = %d, want 0", logoutPeer.UserId)
	}

	var otherPeer model.Peer
	if err := db.Where("id = ?", "other-peer").First(&otherPeer).Error; err != nil {
		t.Fatalf("find other peer: %v", err)
	}
	if otherPeer.UserId != user.Id {
		t.Fatalf("other peer user_id = %d, want %d", otherPeer.UserId, user.Id)
	}

	var otherUserPeer model.Peer
	if err := db.Where("id = ?", "same-uuid-other-user").First(&otherUserPeer).Error; err != nil {
		t.Fatalf("find other user peer: %v", err)
	}
	if otherUserPeer.UserId != otherUser.Id {
		t.Fatalf("other user peer user_id = %d, want %d", otherUserPeer.UserId, otherUser.Id)
	}
}

func TestLogoutSameUuidDeletesOnlyCurrentTokenAndUnbindsFirstMatchingPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)

	isAdmin := false
	user := &model.User{
		Username: "logout-same-uuid-user",
		Status:   model.COMMON_STATUS_ENABLE,
		IsAdmin:  &isAdmin,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserToken{
		UserId:     user.Id,
		Token:      "same-uuid-current-token",
		DeviceUuid: "same-logout-uuid",
		DeviceId:   "same-uuid-peer-a",
		ExpiredAt:  time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("create current token: %v", err)
	}
	if err := db.Create(&model.UserToken{
		UserId:     user.Id,
		Token:      "same-uuid-other-token",
		DeviceUuid: "same-logout-uuid",
		DeviceId:   "same-uuid-peer-b",
		ExpiredAt:  time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("create other same uuid token: %v", err)
	}
	firstPeer := &model.Peer{Id: "same-uuid-peer-a", Uuid: "same-logout-uuid", UserId: user.Id}
	if err := db.Create(firstPeer).Error; err != nil {
		t.Fatalf("create first same uuid peer: %v", err)
	}
	secondPeer := &model.Peer{Id: "same-uuid-peer-b", Uuid: "same-logout-uuid", UserId: user.Id}
	if err := db.Create(secondPeer).Error; err != nil {
		t.Fatalf("create second same uuid peer: %v", err)
	}

	router := gin.New()
	router.POST("/api/logout", middleware.RustAuth(), (&Login{}).Logout)

	logout := postCompatibilityJSON(router, "/api/logout", `{}`, "same-uuid-current-token")
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d; body=%q", logout.Code, http.StatusOK, logout.Body.String())
	}
	if logout.Body.String() != "null" {
		t.Fatalf("logout body = %q, want null", logout.Body.String())
	}

	var currentTokenCount int64
	if err := db.Model(&model.UserToken{}).Where("user_id = ? and token = ?", user.Id, "same-uuid-current-token").Count(&currentTokenCount).Error; err != nil {
		t.Fatalf("count current token: %v", err)
	}
	if currentTokenCount != 0 {
		t.Fatalf("current token count = %d, want 0", currentTokenCount)
	}

	var otherTokenCount int64
	if err := db.Model(&model.UserToken{}).Where("user_id = ? and token = ?", user.Id, "same-uuid-other-token").Count(&otherTokenCount).Error; err != nil {
		t.Fatalf("count other token: %v", err)
	}
	if otherTokenCount != 1 {
		t.Fatalf("other same uuid token count = %d, want 1", otherTokenCount)
	}

	var firstAfter model.Peer
	if err := db.Where("id = ?", firstPeer.Id).First(&firstAfter).Error; err != nil {
		t.Fatalf("find first same uuid peer: %v", err)
	}
	if firstAfter.UserId != 0 {
		t.Fatalf("first same uuid peer user_id = %d, want 0", firstAfter.UserId)
	}

	var secondAfter model.Peer
	if err := db.Where("id = ?", secondPeer.Id).First(&secondAfter).Error; err != nil {
		t.Fatalf("find second same uuid peer: %v", err)
	}
	if secondAfter.UserId != user.Id {
		t.Fatalf("second same uuid peer user_id = %d, want %d", secondAfter.UserId, user.Id)
	}
}
