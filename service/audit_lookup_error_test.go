package service

import (
	"errors"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The HTTP RED fixtures first proved swallowed query errors. This service
// characterization additionally freezes the new lookup and the legacy method.
func TestFindAuditConnByPeerIdAndConnIdPreservesErrorsAndLegacyLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.AuditConn{}); err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })
	svc := &AuditService{}
	seed := &model.AuditConn{PeerId: "target", ConnId: 42}
	if err := db.Create(seed).Error; err != nil {
		t.Fatal(err)
	}
	row, err := svc.FindAuditConnByPeerIdAndConnId("target", 42)
	if err != nil || row.Id != seed.Id {
		t.Fatalf("found %#v err=%v", row, err)
	}
	if legacy := svc.InfoByPeerIdAndConnId("target", 42); legacy.Id != seed.Id {
		t.Fatalf("legacy %#v", legacy)
	}
	row, err = svc.FindAuditConnByPeerIdAndConnId("target", 99)
	if !errors.Is(err, gorm.ErrRecordNotFound) || row.Id != 0 {
		t.Fatalf("missing %#v err=%v", row, err)
	}
	injected := errors.New("synthetic lookup failure")
	if err := db.Callback().Query().Before("gorm:query").Register("audit:lookup_failure", func(tx *gorm.DB) { tx.AddError(injected) }); err != nil {
		t.Fatal(err)
	}
	row, err = svc.FindAuditConnByPeerIdAndConnId("target", 42)
	if !errors.Is(err, injected) || row.Id != 0 {
		t.Fatalf("failed %#v err=%v", row, err)
	}
	if legacy := svc.InfoByPeerIdAndConnId("target", 42); legacy == nil || legacy.Id != 0 {
		t.Fatalf("legacy failure %#v", legacy)
	}
	if err := db.Callback().Query().Remove("audit:lookup_failure"); err != nil {
		t.Fatal(err)
	}
	row, err = svc.FindAuditConnByPeerIdAndConnId("target", 42)
	if err != nil || row.Id != seed.Id {
		t.Fatalf("recovered %#v err=%v", row, err)
	}
}
