package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type usersPeersFixture struct {
	defaultGroup *model.Group
	shareGroup   *model.Group
	self         *model.User
	teammate     *model.User
	shareUser    *model.User
	shareMate    *model.User
}

func createUsersPeersFixtureUser(t *testing.T, db *gorm.DB, username string, groupID uint, isAdmin bool, token string) *model.User {
	t.Helper()

	user := &model.User{
		Username: username,
		GroupId:  groupID,
		Status:   model.COMMON_STATUS_ENABLE,
		IsAdmin:  &isAdmin,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: token, ExpiredAt: serviceTokenExpiry()}).Error; err != nil {
		t.Fatalf("create token for %s: %v", username, err)
	}
	return user
}

func serviceTokenExpiry() int64 {
	return 32503680000
}

func setupUsersPeersFixture(t *testing.T) (*gorm.DB, usersPeersFixture) {
	t.Helper()

	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.Group{}, &model.DeviceGroup{}); err != nil {
		t.Fatalf("migrate group models: %v", err)
	}

	defaultGroup := &model.Group{Name: "default", Type: model.GroupTypeDefault}
	shareGroup := &model.Group{Name: "share", Type: model.GroupTypeShare}
	if err := db.Create(defaultGroup).Error; err != nil {
		t.Fatalf("create default group: %v", err)
	}
	if err := db.Create(shareGroup).Error; err != nil {
		t.Fatalf("create share group: %v", err)
	}

	fixture := usersPeersFixture{defaultGroup: defaultGroup, shareGroup: shareGroup}
	fixture.self = createUsersPeersFixtureUser(t, db, "self", defaultGroup.Id, false, "self-token")
	fixture.teammate = createUsersPeersFixtureUser(t, db, "teammate", defaultGroup.Id, false, "teammate-token")
	fixture.shareUser = createUsersPeersFixtureUser(t, db, "share-self", shareGroup.Id, false, "share-token")
	fixture.shareMate = createUsersPeersFixtureUser(t, db, "share-mate", shareGroup.Id, false, "share-mate-token")

	peers := []*model.Peer{
		{Id: "self-peer", Uuid: "self-uuid", UserId: fixture.self.Id, Hostname: "self-host", Os: "Windows"},
		{Id: "mate-peer", Uuid: "mate-uuid", UserId: fixture.teammate.Id, Hostname: "mate-host", Os: "Linux"},
		{Id: "share-peer", Uuid: "share-uuid", UserId: fixture.shareUser.Id, Hostname: "share-host", Os: "Windows"},
		{Id: "share-mate-peer", Uuid: "share-mate-uuid", UserId: fixture.shareMate.Id, Hostname: "share-mate-host", Os: "Linux"},
	}
	for _, peer := range peers {
		if err := db.Create(peer).Error; err != nil {
			t.Fatalf("create peer %s: %v", peer.Id, err)
		}
	}

	return db, fixture
}

func setupUsersPeersRouter() *gin.Engine {
	router := gin.New()
	group := &Group{}
	router.GET("/api/users", middleware.RustAuth(), group.Users)
	router.GET("/api/peers", middleware.RustAuth(), group.Peers)
	return router
}

func TestUsersReturnsOnlySelfForDefaultGroupNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, _ = setupUsersPeersFixture(t)
	router := setupUsersPeersRouter()

	recorder := getCompatibilityJSON(router, "/api/users", "self-token")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Total int `json:"total"`
		Data  []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal users response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Total != 1 || len(payload.Data) != 1 || payload.Data[0].Name != "self" {
		t.Fatalf("users payload = %#v, want only self", payload)
	}
}

func TestUsersReturnsGroupMembersForShareGroupNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, _ = setupUsersPeersFixture(t)
	router := setupUsersPeersRouter()

	recorder := getCompatibilityJSON(router, "/api/users", "share-token")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Total int `json:"total"`
		Data  []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal users response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Total != 2 || len(payload.Data) != 2 {
		t.Fatalf("users payload = %#v, want two share-group users", payload)
	}
	names := map[string]bool{}
	for _, user := range payload.Data {
		names[user.Name] = true
	}
	if !names["share-self"] || !names["share-mate"] || names["self"] || names["teammate"] {
		t.Fatalf("user names = %#v, want only share group users", names)
	}
}

func TestPeersReturnsOnlyOwnedPeersForDefaultGroupNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, _ = setupUsersPeersFixture(t)
	router := setupUsersPeersRouter()

	recorder := getCompatibilityJSON(router, "/api/peers", "self-token")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Total int `json:"total"`
		Data  []struct {
			Id       string `json:"id"`
			UserName string `json:"user_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal peers response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Total != 1 || len(payload.Data) != 1 {
		t.Fatalf("peers payload = %#v, want one owned peer", payload)
	}
	if payload.Data[0].Id != "self-peer" || payload.Data[0].UserName != "self" {
		t.Fatalf("peer = %#v, want self-peer owned by self", payload.Data[0])
	}
}

func TestPeersReturnsGroupPeersForShareGroupNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, _ = setupUsersPeersFixture(t)
	router := setupUsersPeersRouter()

	recorder := getCompatibilityJSON(router, "/api/peers", "share-token")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Total int `json:"total"`
		Data  []struct {
			Id       string `json:"id"`
			UserName string `json:"user_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal peers response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Total != 2 || len(payload.Data) != 2 {
		t.Fatalf("peers payload = %#v, want two share-group peers", payload)
	}
	peers := map[string]string{}
	for _, peer := range payload.Data {
		peers[peer.Id] = peer.UserName
	}
	if peers["share-peer"] != "share-self" || peers["share-mate-peer"] != "share-mate" || peers["self-peer"] != "" || peers["mate-peer"] != "" {
		t.Fatalf("peer map = %#v, want only share group peers", peers)
	}
}
