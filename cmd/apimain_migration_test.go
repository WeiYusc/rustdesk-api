package main

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDatabaseAutoUpdateMigratesAuthUpgradeTablesFromPreviousVersion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite migration db: %v", err)
	}
	if err := db.AutoMigrate(&model.Version{}); err != nil {
		t.Fatalf("migrate version table: %v", err)
	}
	if err := db.Create(&model.Version{Version: 265}).Error; err != nil {
		t.Fatalf("seed previous version: %v", err)
	}

	global.DB = db
	global.Logger = logrus.New()
	global.Jwt = jwt.NewJwt("", 0)
	service.New(&config.Config{}, db, global.Logger, global.Jwt, nil)

	DatabaseAutoUpdate()

	for _, model := range []struct {
		name  string
		value any
	}{
		{name: "settings", value: &model.Setting{}},
		{name: "user_passkeys", value: &model.UserPasskey{}},
		{name: "auth_challenges", value: &model.AuthChallenge{}},
		{name: "email_verification_tokens", value: &model.EmailVerificationToken{}},
	} {
		if !db.Migrator().HasTable(model.value) {
			t.Fatalf("%s table was not migrated from previous database version", model.name)
		}
	}

	var latest model.Version
	if err := db.Last(&latest).Error; err != nil {
		t.Fatalf("load latest version: %v", err)
	}
	if latest.Version != DatabaseVersion {
		t.Fatalf("latest version = %d, want %d", latest.Version, DatabaseVersion)
	}
}
