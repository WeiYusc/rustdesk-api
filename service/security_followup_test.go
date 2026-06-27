package service

import (
	"regexp"
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSecurityFollowupDB(t *testing.T, models ...interface{}) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	New(&config.Config{}, db, logrus.New(), jwt.NewJwt("", 0), nil)
	return db
}

func TestGenerateTokenFallbackUsesStrongRandomToken(t *testing.T) {
	setupSecurityFollowupDB(t)
	user := &model.User{IdModel: model.IdModel{Id: 7}, Username: "alice"}

	token := AllService.UserService.GenerateToken(user)
	if token == "" {
		t.Fatal("fallback token is empty")
	}
	if regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(token) {
		t.Fatalf("fallback token still looks like weak md5 timestamp token: %q", token)
	}
	if len(token) < 43 {
		t.Fatalf("fallback token length = %d, want at least 43 base64url chars", len(token))
	}
	if token == AllService.UserService.GenerateToken(user) {
		t.Fatal("two fallback tokens for the same user were identical")
	}
}

func TestUserDeleteCleansAssociatedSecurityStateAndUnlinksPeers(t *testing.T) {
	db := setupSecurityFollowupDB(t,
		&model.User{},
		&model.UserThird{},
		&model.AddressBook{},
		&model.AddressBookCollection{},
		&model.AddressBookCollectionRule{},
		&model.UserToken{},
		&model.LoginLog{},
		&model.ShareRecord{},
		&model.Peer{},
	)
	isAdmin := false
	user := &model.User{Username: "delete-me", IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	rows := []interface{}{
		&model.UserThird{UserId: user.Id},
		&model.AddressBook{UserId: user.Id, Id: "ab-1"},
		&model.AddressBookCollection{UserId: user.Id, Name: "collection"},
		&model.AddressBookCollectionRule{UserId: user.Id, CollectionId: 1, Rule: 1, Type: 1, ToId: user.Id},
		&model.AddressBookCollectionRule{UserId: 999, CollectionId: 2, Rule: 1, Type: model.ShareAddressBookRuleTypePersonal, ToId: user.Id},
		&model.UserToken{UserId: user.Id, Token: "delete-token", ExpiredAt: time.Now().Add(time.Hour).Unix()},
		&model.LoginLog{UserId: user.Id, UserTokenId: 1},
		&model.ShareRecord{UserId: user.Id, PeerId: "peer-1", ShareToken: "share-token"},
		&model.Peer{Id: "peer-1", Uuid: "uuid-1", UserId: user.Id},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create related row %#v: %v", row, err)
		}
	}

	if err := AllService.UserService.Delete(user); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	assertSecurityFollowupCount(t, db, &model.UserToken{}, "user_id = ?", int64(0), user.Id)
	assertSecurityFollowupCount(t, db, &model.LoginLog{}, "user_id = ?", int64(0), user.Id)
	assertSecurityFollowupCount(t, db, &model.ShareRecord{}, "user_id = ?", int64(0), user.Id)
	assertSecurityFollowupCount(t, db, &model.AddressBookCollectionRule{}, "type = ? AND to_id = ?", int64(0), model.ShareAddressBookRuleTypePersonal, user.Id)
	var peer model.Peer
	if err := db.Where("id = ?", "peer-1").First(&peer).Error; err != nil {
		t.Fatalf("find peer: %v", err)
	}
	if peer.UserId != 0 {
		t.Fatalf("peer UserId = %d, want unlinked", peer.UserId)
	}
}

func TestUpdateAddressBookRollsBackOnCreateError(t *testing.T) {
	db := setupSecurityFollowupDB(t, &model.AddressBook{})
	if err := db.Create(&model.AddressBook{UserId: 99, Id: "keep", Alias: "keep"}).Error; err != nil {
		t.Fatalf("create existing address book: %v", err)
	}
	db.Callback().Create().Before("gorm:create").Register("test_address_book_create_error", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.AddressBook); ok {
			tx.AddError(gorm.ErrInvalidData)
		}
	})

	err := AllService.AddressBookService.UpdateAddressBook([]*model.AddressBook{{Id: "new", Alias: "invalid"}}, 99)
	if err == nil {
		t.Fatal("UpdateAddressBook returned nil after invalid create")
	}
	assertSecurityFollowupCount(t, db, &model.AddressBook{}, "user_id = ? AND id = ?", int64(1), 99, "keep")
	assertSecurityFollowupCount(t, db, &model.AddressBook{}, "user_id = ? AND id = ?", int64(0), 99, "new")
}

func TestDeleteCollectionRollsBackOnDeleteError(t *testing.T) {
	db := setupSecurityFollowupDB(t, &model.AddressBookCollection{}, &model.AddressBook{})
	collection := &model.AddressBookCollection{UserId: 7, Name: "owned"}
	if err := db.Create(collection).Error; err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if err := db.Create(&model.AddressBook{UserId: 7, CollectionId: collection.Id, Id: "kept"}).Error; err != nil {
		t.Fatalf("create address book: %v", err)
	}
	if err := db.Migrator().DropTable(&model.AddressBookCollection{}); err != nil {
		t.Fatalf("drop collection table: %v", err)
	}

	err := AllService.AddressBookService.DeleteCollection(collection)
	if err == nil {
		t.Fatal("DeleteCollection returned nil after collection delete failed")
	}
	assertSecurityFollowupCount(t, db, &model.AddressBook{}, "collection_id = ?", int64(1), collection.Id)
}

func TestUpdateTagsRollsBackOnCreateError(t *testing.T) {
	db := setupSecurityFollowupDB(t, &model.Tag{})
	if err := db.Create(&model.Tag{UserId: 3, Name: "keep", Color: 1}).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	db.Callback().Create().Before("gorm:create").Register("test_tag_create_error", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.Tag); ok {
			tx.AddError(gorm.ErrInvalidData)
		}
	})
	err := AllService.TagService.UpdateTags(3, map[string]uint{"new": 2})
	if err == nil {
		t.Fatal("UpdateTags returned nil after invalid create")
	}
	assertSecurityFollowupCount(t, db, &model.Tag{}, "user_id = ? AND name = ?", int64(1), 3, "keep")
	assertSecurityFollowupCount(t, db, &model.Tag{}, "user_id = ? AND name = ?", int64(0), 3, "new")
}

func assertSecurityFollowupCount(t *testing.T, db *gorm.DB, modelValue interface{}, query string, want int64, args ...interface{}) {
	t.Helper()
	var count int64
	if err := db.Model(modelValue).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatalf("count %T: %v", modelValue, err)
	}
	if count != want {
		t.Fatalf("count %T = %d, want %d", modelValue, count, want)
	}
}
