package orm

import (
	"database/sql"
	"reflect"
	"testing"
	"time"
)

func TestApplyMysqlConnPoolConfigSetsSafeDefaultLifetimes(t *testing.T) {
	sqlDB := &sql.DB{}

	applyMysqlConnPoolConfig(sqlDB, &MysqlConfig{
		MaxIdleConns: 10,
		MaxOpenConns: 100,
	})

	if got := sqlDB.Stats().MaxOpenConnections; got != 100 {
		t.Fatalf("MaxOpenConnections = %d, want 100", got)
	}
	if got := sqlDBDurationField(t, sqlDB, "maxIdleTime"); got != defaultMysqlConnMaxIdleTime {
		t.Fatalf("maxIdleTime = %s, want %s", got, defaultMysqlConnMaxIdleTime)
	}
	if got := sqlDBDurationField(t, sqlDB, "maxLifetime"); got != defaultMysqlConnMaxLifetime {
		t.Fatalf("maxLifetime = %s, want %s", got, defaultMysqlConnMaxLifetime)
	}
	if defaultMysqlConnMaxLifetime >= 8*time.Hour {
		t.Fatalf("defaultMysqlConnMaxLifetime = %s, want below MySQL default wait_timeout", defaultMysqlConnMaxLifetime)
	}
}

func TestApplyMysqlConnPoolConfigHonorsExplicitLifetimes(t *testing.T) {
	sqlDB := &sql.DB{}

	applyMysqlConnPoolConfig(sqlDB, &MysqlConfig{
		ConnMaxIdleTime: 30 * time.Minute,
		ConnMaxLifetime: 2 * time.Hour,
	})

	if got := sqlDBDurationField(t, sqlDB, "maxIdleTime"); got != 30*time.Minute {
		t.Fatalf("maxIdleTime = %s, want 30m", got)
	}
	if got := sqlDBDurationField(t, sqlDB, "maxLifetime"); got != 2*time.Hour {
		t.Fatalf("maxLifetime = %s, want 2h", got)
	}
}

func sqlDBDurationField(t *testing.T, sqlDB *sql.DB, fieldName string) time.Duration {
	t.Helper()
	field := reflect.ValueOf(sqlDB).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("database/sql.DB has no field %q", fieldName)
	}
	return time.Duration(field.Int())
}
