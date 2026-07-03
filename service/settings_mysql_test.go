package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type mysqlSettingsQueryDriver struct{}
type mysqlSettingsQueryConn struct{}
type mysqlSettingsQueryRows struct{}

var mysqlSettingsLastQuery string

func (mysqlSettingsQueryDriver) Open(string) (driver.Conn, error) {
	return mysqlSettingsQueryConn{}, nil
}

func (mysqlSettingsQueryConn) Prepare(query string) (driver.Stmt, error) {
	mysqlSettingsLastQuery = query
	return mysqlSettingsQueryStmt{}, nil
}

func (mysqlSettingsQueryConn) Close() error              { return nil }
func (mysqlSettingsQueryConn) Begin() (driver.Tx, error) { return nil, nil }

func (mysqlSettingsQueryConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	mysqlSettingsLastQuery = query
	return mysqlSettingsQueryRows{}, nil
}

type mysqlSettingsQueryStmt struct{}

func (mysqlSettingsQueryStmt) Close() error  { return nil }
func (mysqlSettingsQueryStmt) NumInput() int { return -1 }
func (mysqlSettingsQueryStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (mysqlSettingsQueryStmt) Query([]driver.Value) (driver.Rows, error) {
	return mysqlSettingsQueryRows{}, nil
}

func (mysqlSettingsQueryRows) Columns() []string {
	return []string{"id", "key", "value", "is_secret", "updated_by", "created_at", "updated_at"}
}
func (mysqlSettingsQueryRows) Close() error              { return nil }
func (mysqlSettingsQueryRows) Next([]driver.Value) error { return driver.ErrBadConn }

func init() {
	sql.Register("mysql_settings_query_capture", mysqlSettingsQueryDriver{})
}

func TestSettingsServiceAuthPolicyQueryQuotesKeyForMysql(t *testing.T) {
	sqlDB, err := sql.Open("mysql_settings_query_capture", "")
	if err != nil {
		t.Fatalf("open captured mysql sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open captured mysql gorm db: %v", err)
	}
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })
	svc := &SettingsService{}

	mysqlSettingsLastQuery = ""
	_, _ = svc.GetAuthPolicy()
	if mysqlSettingsLastQuery == "" {
		t.Fatalf("MySQL settings query was not captured")
	}
	if strings.Contains(mysqlSettingsLastQuery, "WHERE key = ?") {
		t.Fatalf("MySQL settings query leaves reserved column unquoted: %s", mysqlSettingsLastQuery)
	}
	if !strings.Contains(mysqlSettingsLastQuery, "WHERE `key` = ?") {
		t.Fatalf("MySQL settings query does not quote reserved key column: %s", mysqlSettingsLastQuery)
	}
}

func TestSettingsServiceSaveRoundTripsAuthPolicy(t *testing.T) {
	db := setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	if err := svc.SaveAuthPolicy(AuthPolicySettings{DisablePasswordLogin: false}, 1); err != nil {
		t.Fatalf("SaveAuthPolicy error: %v", err)
	}

	var count int64
	if err := db.Model(&model.Setting{}).Where("`key` = ?", SettingKeyAuthPolicy).Count(&count).Error; err != nil {
		t.Fatalf("count auth policy setting: %v", err)
	}
	if count != 1 {
		t.Fatalf("auth policy setting count = %d, want 1", count)
	}
	got, err := svc.GetAuthPolicy()
	if err != nil {
		t.Fatalf("GetAuthPolicy after save error: %v", err)
	}
	if got.DisablePasswordLogin {
		t.Fatalf("auth policy DisablePasswordLogin = true, want false")
	}
}
