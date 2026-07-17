package service

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAddressBookOnlineTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AddressBook{}, &model.Peer{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	New(nil, db, logrus.New(), nil, nil)
	return db
}

func TestAddressBookListAppliesRecentPeerHeartbeatOnlineState(t *testing.T) {
	db := setupAddressBookOnlineTestDB(t)
	now := time.Now().Unix()
	abs := []*model.AddressBook{
		{Id: "recent-peer", UserId: 1, Online: false},
		{Id: "stale-peer", UserId: 1, Online: true},
		{Id: "missing-peer", UserId: 1, Online: true, SameServer: true},
	}
	for _, ab := range abs {
		if err := db.Create(ab).Error; err != nil {
			t.Fatalf("create address book %s: %v", ab.Id, err)
		}
	}
	peers := []*model.Peer{
		{Id: "recent-peer", LastOnlineTime: now - 10},
		{Id: "stale-peer", LastOnlineTime: now - 120},
	}
	for _, peer := range peers {
		if err := db.Create(peer).Error; err != nil {
			t.Fatalf("create peer %s: %v", peer.Id, err)
		}
	}

	res := AllService.AddressBookService.List(1, 100, func(tx *gorm.DB) {
		tx.Where("user_id = ?", 1)
	})

	got := map[string]*model.AddressBook{}
	for _, ab := range res.AddressBooks {
		got[ab.Id] = ab
	}
	if !got["recent-peer"].Online || !got["recent-peer"].SameServer {
		t.Fatalf("recent peer online/sameServer = %v/%v, want true/true", got["recent-peer"].Online, got["recent-peer"].SameServer)
	}
	if got["stale-peer"].Online || !got["stale-peer"].SameServer {
		t.Fatalf("stale peer online/sameServer = %v/%v, want false/true", got["stale-peer"].Online, got["stale-peer"].SameServer)
	}
	if got["missing-peer"].Online || got["missing-peer"].SameServer {
		t.Fatalf("missing peer online/sameServer = %v/%v, want false/false", got["missing-peer"].Online, got["missing-peer"].SameServer)
	}
}

func TestApplyPeerOnlineStateUsesDefaultTTL(t *testing.T) {
	db := setupAddressBookOnlineTestDB(t)
	now := time.Now().Unix()
	if err := db.Create(&model.Peer{Id: "recent-peer", LastOnlineTime: now - 30}).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}
	abs := []*model.AddressBook{{Id: "recent-peer"}}

	AllService.AddressBookService.ApplyPeerOnlineState(abs, 0)

	if !abs[0].Online || !abs[0].SameServer {
		t.Fatalf("online/sameServer = %v/%v, want true/true", abs[0].Online, abs[0].SameServer)
	}
}
