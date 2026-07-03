package orm

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestNewMysqlWithErrorReturnsConnectionError(t *testing.T) {
	_, err := NewMysqlWithError(&MysqlConfig{
		Dsn:          "invalid:mysql@tcp(127.0.0.1:1)/missing?timeout=1ms",
		MaxIdleConns: 1,
		MaxOpenConns: 1,
	}, logrus.New())
	if err == nil {
		t.Fatalf("NewMysqlWithError returned nil error for invalid DSN")
	}
}

func TestNewMysqlCompatibilityWrapperPreservesPanicOnConnectionError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("NewMysql did not panic for invalid DSN")
		} else {
			// The panic payload should be an error; exact driver error text is not stable.
			if _, ok := r.(error); !ok {
				t.Fatalf("NewMysql panic payload type = %T, want error", r)
			}
		}
	}()
	_ = NewMysql(&MysqlConfig{
		Dsn:          "invalid:mysql@tcp(127.0.0.1:1)/missing?timeout=1ms",
		MaxIdleConns: 1,
		MaxOpenConns: 1,
	}, logrus.New())
}

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
