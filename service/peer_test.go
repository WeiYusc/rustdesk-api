package service

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPeerServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Peer{}); err != nil {
		t.Fatalf("migrate peer: %v", err)
	}
	DB = db
	return db
}

func TestPeerServiceUuidBindUserIdAssignsExistingPeer(t *testing.T) {
	setupPeerServiceTestDB(t)

	peerService := &PeerService{}
	peer := &model.Peer{Id: "peer-1", Uuid: "uuid-1"}
	if err := peerService.Create(peer); err != nil {
		t.Fatalf("create peer: %v", err)
	}

	peerService.UuidBindUserId("peer-1", "uuid-1", 42)

	updated := peerService.FindByUuid("uuid-1")
	if updated.RowId == 0 {
		t.Fatalf("updated peer not found")
	}
	if updated.UserId != 42 {
		t.Fatalf("UserId = %d, want 42", updated.UserId)
	}
}

func TestPeerServiceUpdateCanClearUserId(t *testing.T) {
	setupPeerServiceTestDB(t)

	peerService := &PeerService{}
	peer := &model.Peer{Id: "peer-1", Uuid: "uuid-1", UserId: 42}
	if err := peerService.Create(peer); err != nil {
		t.Fatalf("create peer: %v", err)
	}

	if err := peerService.Update(&model.Peer{RowId: peer.RowId, UserId: 0}); err != nil {
		t.Fatalf("clear peer user id: %v", err)
	}

	updated := peerService.FindByUuid("uuid-1")
	if updated.UserId != 0 {
		t.Fatalf("UserId = %d, want 0", updated.UserId)
	}
}
