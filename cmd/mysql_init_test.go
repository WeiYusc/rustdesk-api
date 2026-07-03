package main

import "testing"

func TestMysqlDatabaseIdentifierRejectsUnsafeNames(t *testing.T) {
	cases := []string{
		"",
		"rustdesk-api",
		"rustdesk.api",
		"rustdesk api",
		"rustdesk`api",
		"rustdesk;drop database mysql",
	}
	for _, name := range cases {
		if _, err := mysqlDatabaseIdentifier(name); err == nil {
			t.Fatalf("mysqlDatabaseIdentifier(%q) succeeded, want validation error", name)
		}
	}
}

func TestMysqlDatabaseIdentifierQuotesSafeNames(t *testing.T) {
	got, err := mysqlDatabaseIdentifier("rustdesk_api_2026")
	if err != nil {
		t.Fatalf("mysqlDatabaseIdentifier safe name error: %v", err)
	}
	if got != "`rustdesk_api_2026`" {
		t.Fatalf("quoted identifier = %q, want `rustdesk_api_2026`", got)
	}
}

func TestMysqlDSNBuildersSeparateServerAndDatabaseConnections(t *testing.T) {
	cfg := mysqlDSNConfig{
		Username: "user",
		Password: "pass",
		Addr:     "127.0.0.1:3306",
		Dbname:   "rustdesk_api",
		Tls:      "skip-verify",
	}

	serverDSN := mysqlServerDSN(cfg)
	if serverDSN != "user:pass@(127.0.0.1:3306)/?charset=utf8mb4&parseTime=True&loc=Local&tls=skip-verify" {
		t.Fatalf("server DSN = %q", serverDSN)
	}
	databaseDSN := mysqlDatabaseDSN(cfg)
	if databaseDSN != "user:pass@(127.0.0.1:3306)/rustdesk_api?charset=utf8mb4&parseTime=True&loc=Local&tls=skip-verify" {
		t.Fatalf("database DSN = %q", databaseDSN)
	}
}
