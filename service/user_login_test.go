package service

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserLoginFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.LoginLog{}, &model.Peer{}); err != nil {
		t.Fatalf("migrate user login models: %v", err)
	}
	cfg := &config.Config{}
	cfg.App.TokenExpire = time.Hour
	global.Config = *cfg
	global.Logger = logrus.New()
	global.Jwt = jwt.NewJwt("", 0)
	New(cfg, db, global.Logger, global.Jwt, nil)
	return db
}

func TestUserLoginStoresUserTokenRowIDInLoginLog(t *testing.T) {
	db := setupUserLoginFixture(t)
	isAdmin := false
	user := &model.User{Username: "login-log-user", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "preexisting-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create preexisting token: %v", err)
	}

	token := AllService.UserService.Login(user, &model.LoginLog{UserId: user.Id, Client: model.LoginLogClientWebAdmin, Type: model.LoginLogTypeAccount})
	if token.Id == 0 {
		t.Fatalf("created token has zero id")
	}

	var log model.LoginLog
	if err := db.First(&log).Error; err != nil {
		t.Fatalf("find login log: %v", err)
	}
	if log.UserTokenId != token.Id {
		t.Fatalf("login log user_token_id = %d, want token row id %d", log.UserTokenId, token.Id)
	}
}
