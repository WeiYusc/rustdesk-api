package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type addressBookFixture struct {
	user   *model.User
	guid   string
	router *gin.Engine
}

func setupAddressBookFixture(t *testing.T) (*gorm.DB, addressBookFixture) {
	t.Helper()

	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.Group{}, &model.Tag{}, &model.AddressBook{}, &model.AddressBookCollection{}, &model.AddressBookCollectionRule{}); err != nil {
		t.Fatalf("migrate address book models: %v", err)
	}
	global.Config.Rustdesk.Personal = 1

	group := &model.Group{Name: "default", Type: model.GroupTypeDefault}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	user := createUsersPeersFixtureUser(t, db, "ab-user", group.Id, false, "ab-token")

	ab := &Ab{}
	router := gin.New()
	router.POST("/api/ab/personal", middleware.RustAuth(), ab.Personal)
	router.POST("/api/ab/settings", middleware.RustAuth(), ab.Settings)
	router.POST("/api/ab/shared/profiles", middleware.RustAuth(), ab.SharedProfiles)
	router.POST("/api/ab/tags/:guid", middleware.RustAuth(), ab.PTags)
	router.POST("/api/ab/tag/add/:guid", middleware.RustAuth(), ab.TagAdd)
	router.POST("/api/ab/peers", middleware.RustAuth(), ab.Peers)
	router.POST("/api/ab/peer/add/:guid", middleware.RustAuth(), ab.PeerAdd)
	router.DELETE("/api/ab/peer/:guid", middleware.RustAuth(), ab.PeerDel)
	router.PUT("/api/ab/peer/update/:guid", middleware.RustAuth(), ab.PeerUpdate)

	return db, addressBookFixture{
		user:   user,
		guid:   ab.ComposeGuid(group.Id, user.Id, 0),
		router: router,
	}
}

func requestAddressBookJSON(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAddressBookPersonalAndSettingsExposePersonalGuid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, fixture := setupAddressBookFixture(t)

	personal := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/personal", `{}`, "ab-token")
	if personal.Code != http.StatusOK {
		t.Fatalf("personal status = %d, want %d; body=%q", personal.Code, http.StatusOK, personal.Body.String())
	}
	var personalPayload struct {
		Guid string `json:"guid"`
		Name string `json:"name"`
		Rule int    `json:"rule"`
	}
	if err := json.Unmarshal(personal.Body.Bytes(), &personalPayload); err != nil {
		t.Fatalf("unmarshal personal response: %v; body=%q", err, personal.Body.String())
	}
	if personalPayload.Guid != fixture.guid || personalPayload.Name != fixture.user.Username || personalPayload.Rule != model.ShareAddressBookRuleRuleFullControl {
		t.Fatalf("personal payload = %#v, want guid=%q name=%q rule=3", personalPayload, fixture.guid, fixture.user.Username)
	}

	settings := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/settings", `{}`, "ab-token")
	if settings.Code != http.StatusOK {
		t.Fatalf("settings status = %d, want %d; body=%q", settings.Code, http.StatusOK, settings.Body.String())
	}
	var settingsPayload struct {
		MaxPeerOneAB int `json:"max_peer_one_ab"`
	}
	if err := json.Unmarshal(settings.Body.Bytes(), &settingsPayload); err != nil {
		t.Fatalf("unmarshal settings response: %v; body=%q", err, settings.Body.String())
	}
	if settingsPayload.MaxPeerOneAB != 0 {
		t.Fatalf("max_peer_one_ab = %d, want 0", settingsPayload.MaxPeerOneAB)
	}
}

func TestAddressBookTagAndPeerPersonalFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, fixture := setupAddressBookFixture(t)

	addTag := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/tag/add/"+fixture.guid, `{"name":"ops","color":4281558681}`, "ab-token")
	if addTag.Code != http.StatusOK {
		t.Fatalf("tag add status = %d, want %d; body=%q", addTag.Code, http.StatusOK, addTag.Body.String())
	}
	var tag model.Tag
	if err := db.Where("user_id = ? and name = ? and collection_id = ?", fixture.user.Id, "ops", 0).First(&tag).Error; err != nil {
		t.Fatalf("find added tag: %v", err)
	}
	if tag.Color != 4281558681 {
		t.Fatalf("tag color = %d, want 4281558681", tag.Color)
	}

	listTags := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/tags/"+fixture.guid, `{}`, "ab-token")
	if listTags.Code != http.StatusOK {
		t.Fatalf("tag list status = %d, want %d; body=%q", listTags.Code, http.StatusOK, listTags.Body.String())
	}
	var tags []struct {
		Name  string `json:"name"`
		Color uint   `json:"color"`
	}
	if err := json.Unmarshal(listTags.Body.Bytes(), &tags); err != nil {
		t.Fatalf("unmarshal tags response: %v; body=%q", err, listTags.Body.String())
	}
	if len(tags) != 1 || tags[0].Name != "ops" || tags[0].Color != 4281558681 {
		t.Fatalf("tags = %#v, want ops tag", tags)
	}

	addPeer := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/peer/add/"+fixture.guid, `{"id":"peer-1","username":"rd","hostname":"host-1","platform":"Windows","alias":"old-alias","tags":["ops"],"forceAlwaysRelay":"true"}`, "ab-token")
	if addPeer.Code != http.StatusOK {
		t.Fatalf("peer add status = %d, want %d; body=%q", addPeer.Code, http.StatusOK, addPeer.Body.String())
	}

	updatePeer := requestAddressBookJSON(fixture.router, http.MethodPut, "/api/ab/peer/update/"+fixture.guid, `{"id":"peer-1","alias":"new-alias","tags":["ops"],"hostname":"ignored-host"}`, "ab-token")
	if updatePeer.Code != http.StatusOK {
		t.Fatalf("peer update status = %d, want %d; body=%q", updatePeer.Code, http.StatusOK, updatePeer.Body.String())
	}

	listPeers := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/peers?ab="+fixture.guid, `{}`, "ab-token")
	if listPeers.Code != http.StatusOK {
		t.Fatalf("peer list status = %d, want %d; body=%q", listPeers.Code, http.StatusOK, listPeers.Body.String())
	}
	var peerPayload struct {
		Total int `json:"total"`
		Data  []struct {
			Id       string   `json:"id"`
			Username string   `json:"username"`
			Hostname string   `json:"hostname"`
			Alias    string   `json:"alias"`
			Tags     []string `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listPeers.Body.Bytes(), &peerPayload); err != nil {
		t.Fatalf("unmarshal peers response: %v; body=%q", err, listPeers.Body.String())
	}
	if peerPayload.Total != 1 || len(peerPayload.Data) != 1 {
		t.Fatalf("peer payload = %#v, want one peer", peerPayload)
	}
	peer := peerPayload.Data[0]
	if peer.Id != "peer-1" || peer.Username != "rd" || peer.Hostname != "host-1" || peer.Alias != "new-alias" || len(peer.Tags) != 1 || peer.Tags[0] != "ops" {
		t.Fatalf("peer = %#v, want updated alias with original hostname and ops tag", peer)
	}
}

func TestAddressBookSharedProfilesAndRulePrivileges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, fixture := setupAddressBookFixture(t)

	ownerGroup := &model.Group{Name: "owner-group", Type: model.GroupTypeDefault}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("create owner group: %v", err)
	}
	owner := createUsersPeersFixtureUser(t, db, "owner-user", ownerGroup.Id, false, "owner-token")
	ownedCollection := &model.AddressBookCollection{UserId: fixture.user.Id, Name: "viewer-owned"}
	if err := db.Create(ownedCollection).Error; err != nil {
		t.Fatalf("create owned collection: %v", err)
	}
	readCollection := &model.AddressBookCollection{UserId: owner.Id, Name: "owner-read-shared"}
	if err := db.Create(readCollection).Error; err != nil {
		t.Fatalf("create read collection: %v", err)
	}
	writeCollection := &model.AddressBookCollection{UserId: owner.Id, Name: "owner-write-shared"}
	if err := db.Create(writeCollection).Error; err != nil {
		t.Fatalf("create write collection: %v", err)
	}
	if err := db.Create(&model.AddressBook{Id: "read-peer", UserId: owner.Id, CollectionId: readCollection.Id, Username: "rd", Hostname: "read-host", Platform: "Linux"}).Error; err != nil {
		t.Fatalf("create read peer: %v", err)
	}
	if err := db.Create(&model.AddressBookCollectionRule{UserId: owner.Id, CollectionId: readCollection.Id, Type: model.ShareAddressBookRuleTypePersonal, ToId: fixture.user.Id, Rule: model.ShareAddressBookRuleRuleRead}).Error; err != nil {
		t.Fatalf("create read personal rule: %v", err)
	}
	if err := db.Create(&model.AddressBookCollectionRule{UserId: owner.Id, CollectionId: writeCollection.Id, Type: model.ShareAddressBookRuleTypePersonal, ToId: fixture.user.Id, Rule: model.ShareAddressBookRuleRuleRead}).Error; err != nil {
		t.Fatalf("create weak write personal rule: %v", err)
	}
	if err := db.Create(&model.AddressBookCollectionRule{UserId: owner.Id, CollectionId: writeCollection.Id, Type: model.ShareAddressBookRuleTypeGroup, ToId: fixture.user.GroupId, Rule: model.ShareAddressBookRuleRuleReadWrite}).Error; err != nil {
		t.Fatalf("create strong write group rule: %v", err)
	}

	readGuid := (&Ab{}).ComposeGuid(owner.GroupId, owner.Id, readCollection.Id)
	writeGuid := (&Ab{}).ComposeGuid(owner.GroupId, owner.Id, writeCollection.Id)
	ownedGuid := (&Ab{}).ComposeGuid(fixture.user.GroupId, fixture.user.Id, ownedCollection.Id)

	profiles := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/shared/profiles", `{}`, "ab-token")
	if profiles.Code != http.StatusOK {
		t.Fatalf("shared profiles status = %d, want %d; body=%q", profiles.Code, http.StatusOK, profiles.Body.String())
	}
	var profilePayload struct {
		Total int `json:"total"`
		Data  []struct {
			Guid  string `json:"guid"`
			Name  string `json:"name"`
			Owner string `json:"owner"`
			Rule  int    `json:"rule"`
		} `json:"data"`
	}
	if err := json.Unmarshal(profiles.Body.Bytes(), &profilePayload); err != nil {
		t.Fatalf("unmarshal shared profiles: %v; body=%q", err, profiles.Body.String())
	}
	if profilePayload.Total != 0 {
		t.Fatalf("shared profiles total = %d, want current quirk 0", profilePayload.Total)
	}
	profilesByGuid := map[string]struct {
		Name  string
		Owner string
		Rule  int
	}{}
	for _, profile := range profilePayload.Data {
		profilesByGuid[profile.Guid] = struct {
			Name  string
			Owner string
			Rule  int
		}{Name: profile.Name, Owner: profile.Owner, Rule: profile.Rule}
	}
	if profilesByGuid[ownedGuid].Name != "viewer-owned" || profilesByGuid[ownedGuid].Owner != fixture.user.Username || profilesByGuid[ownedGuid].Rule != model.ShareAddressBookRuleRuleFullControl {
		t.Fatalf("owned profile = %#v", profilesByGuid[ownedGuid])
	}
	if profilesByGuid[readGuid].Name != "owner-read-shared" || profilesByGuid[readGuid].Owner != owner.Username || profilesByGuid[readGuid].Rule != model.ShareAddressBookRuleRuleRead {
		t.Fatalf("read shared profile = %#v", profilesByGuid[readGuid])
	}
	if profilesByGuid[writeGuid].Name != "owner-write-shared" || profilesByGuid[writeGuid].Owner != owner.Username || profilesByGuid[writeGuid].Rule != model.ShareAddressBookRuleRuleReadWrite {
		t.Fatalf("write shared profile = %#v", profilesByGuid[writeGuid])
	}

	readPeers := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/peers?ab="+readGuid, `{}`, "ab-token")
	if readPeers.Code != http.StatusOK {
		t.Fatalf("read peers status = %d, want %d; body=%q", readPeers.Code, http.StatusOK, readPeers.Body.String())
	}
	var readPeerPayload struct {
		Total int `json:"total"`
		Data  []struct {
			Id       string `json:"id"`
			Hostname string `json:"hostname"`
		} `json:"data"`
	}
	if err := json.Unmarshal(readPeers.Body.Bytes(), &readPeerPayload); err != nil {
		t.Fatalf("unmarshal read peers: %v; body=%q", err, readPeers.Body.String())
	}
	if readPeerPayload.Total != 1 || len(readPeerPayload.Data) != 1 || readPeerPayload.Data[0].Id != "read-peer" || readPeerPayload.Data[0].Hostname != "read-host" {
		t.Fatalf("read peer payload = %#v", readPeerPayload)
	}

	readOnlyAdd := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/peer/add/"+readGuid, `{"id":"blocked-peer","username":"rd","hostname":"blocked","platform":"Linux"}`, "ab-token")
	if readOnlyAdd.Code != http.StatusBadRequest {
		t.Fatalf("read-only add status = %d, want %d; body=%q", readOnlyAdd.Code, http.StatusBadRequest, readOnlyAdd.Body.String())
	}
	var blockedCount int64
	if err := db.Model(&model.AddressBook{}).Where("id = ?", "blocked-peer").Count(&blockedCount).Error; err != nil {
		t.Fatalf("count blocked peer: %v", err)
	}
	if blockedCount != 0 {
		t.Fatalf("blocked peer count = %d, want 0", blockedCount)
	}

	writeAdd := requestAddressBookJSON(fixture.router, http.MethodPost, "/api/ab/peer/add/"+writeGuid, `{"id":"write-peer","username":"rd","hostname":"write-host","platform":"Linux"}`, "ab-token")
	if writeAdd.Code != http.StatusOK {
		t.Fatalf("write add status = %d, want %d; body=%q", writeAdd.Code, http.StatusOK, writeAdd.Body.String())
	}
	var writtenPeer model.AddressBook
	if err := db.Where("id = ?", "write-peer").First(&writtenPeer).Error; err != nil {
		t.Fatalf("find written peer: %v", err)
	}
	if writtenPeer.UserId != owner.Id || writtenPeer.CollectionId != writeCollection.Id || writtenPeer.Hostname != "write-host" {
		t.Fatalf("written peer = %#v", writtenPeer)
	}
}

func TestAddressBookPeerDeleteRequiresFullControlAndDeletesSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, fixture := setupAddressBookFixture(t)

	ownerGroup := &model.Group{Name: "delete-owner-group", Type: model.GroupTypeDefault}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("create owner group: %v", err)
	}
	owner := createUsersPeersFixtureUser(t, db, "delete-owner", ownerGroup.Id, false, "delete-owner-token")

	readWriteCollection := &model.AddressBookCollection{UserId: owner.Id, Name: "delete-read-write"}
	if err := db.Create(readWriteCollection).Error; err != nil {
		t.Fatalf("create read-write collection: %v", err)
	}
	fullControlCollection := &model.AddressBookCollection{UserId: owner.Id, Name: "delete-full-control"}
	if err := db.Create(fullControlCollection).Error; err != nil {
		t.Fatalf("create full-control collection: %v", err)
	}
	readWritePeer := &model.AddressBook{Id: "rw-delete-blocked", UserId: owner.Id, CollectionId: readWriteCollection.Id, Username: "rd", Hostname: "rw-host", Platform: "Linux"}
	if err := db.Create(readWritePeer).Error; err != nil {
		t.Fatalf("create read-write peer: %v", err)
	}
	fullControlPeer := &model.AddressBook{Id: "fc-delete-target", UserId: owner.Id, CollectionId: fullControlCollection.Id, Username: "rd", Hostname: "fc-host", Platform: "Linux"}
	if err := db.Create(fullControlPeer).Error; err != nil {
		t.Fatalf("create full-control peer: %v", err)
	}
	fullControlKeep := &model.AddressBook{Id: "fc-delete-keep", UserId: owner.Id, CollectionId: fullControlCollection.Id, Username: "rd", Hostname: "fc-keep", Platform: "Linux"}
	if err := db.Create(fullControlKeep).Error; err != nil {
		t.Fatalf("create full-control keep peer: %v", err)
	}
	if err := db.Create(&model.AddressBookCollectionRule{UserId: owner.Id, CollectionId: readWriteCollection.Id, Type: model.ShareAddressBookRuleTypePersonal, ToId: fixture.user.Id, Rule: model.ShareAddressBookRuleRuleReadWrite}).Error; err != nil {
		t.Fatalf("create read-write rule: %v", err)
	}
	if err := db.Create(&model.AddressBookCollectionRule{UserId: owner.Id, CollectionId: fullControlCollection.Id, Type: model.ShareAddressBookRuleTypePersonal, ToId: fixture.user.Id, Rule: model.ShareAddressBookRuleRuleFullControl}).Error; err != nil {
		t.Fatalf("create full-control rule: %v", err)
	}

	readWriteGuid := (&Ab{}).ComposeGuid(owner.GroupId, owner.Id, readWriteCollection.Id)
	fullControlGuid := (&Ab{}).ComposeGuid(owner.GroupId, owner.Id, fullControlCollection.Id)

	blockedDelete := requestAddressBookJSON(fixture.router, http.MethodDelete, "/api/ab/peer/"+readWriteGuid, `["rw-delete-blocked"]`, "ab-token")
	if blockedDelete.Code != http.StatusBadRequest {
		t.Fatalf("read-write delete status = %d, want %d; body=%q", blockedDelete.Code, http.StatusBadRequest, blockedDelete.Body.String())
	}
	assertAddressBookPeerCount(t, db, "rw-delete-blocked", 1)

	allowedDelete := requestAddressBookJSON(fixture.router, http.MethodDelete, "/api/ab/peer/"+fullControlGuid, `["fc-delete-target"]`, "ab-token")
	if allowedDelete.Code != http.StatusOK {
		t.Fatalf("full-control delete status = %d, want %d; body=%q", allowedDelete.Code, http.StatusOK, allowedDelete.Body.String())
	}
	if allowedDelete.Body.String() != "" {
		t.Fatalf("full-control delete body = %q, want empty", allowedDelete.Body.String())
	}
	assertAddressBookPeerCount(t, db, "fc-delete-target", 0)
	assertAddressBookPeerCount(t, db, "fc-delete-keep", 1)
}

func assertAddressBookPeerCount(t *testing.T, db *gorm.DB, id string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.AddressBook{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count address-book peer %s: %v", id, err)
	}
	if count != want {
		t.Fatalf("address-book peer %s count = %d, want %d", id, count, want)
	}
}
