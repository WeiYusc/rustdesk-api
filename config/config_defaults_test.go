package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigEnablesLDAPTLSVerification(t *testing.T) {
	var cfg Config
	Init(&cfg, filepath.Join("..", "conf", "config.yaml"))

	if !cfg.Ldap.TlsVerify {
		t.Fatalf("default ldap.tls-verify = false, want true for safe LDAPS defaults")
	}
	if cfg.Gin.BodyMaxSizeMb != 10 {
		t.Fatalf("default gin.body-max-size-mb = %d, want 10", cfg.Gin.BodyMaxSizeMb)
	}
	if cfg.Gin.ReportingRateLimitPerMin != 120 {
		t.Fatalf("default gin.reporting-rate-limit-per-minute = %d, want 120", cfg.Gin.ReportingRateLimitPerMin)
	}
}

func TestOmittedLDAPTLSVerifyDefaultsToTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `lang: "en"
app: {}
admin: {}
gin: {}
gorm: {}
mysql: {}
postgresql: {}
rustdesk: {}
logger: {}
proxy: {}
jwt: {}
ldap:
  enable: false
  url: "ldaps://ldap.example.com:636"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	var cfg Config
	Init(&cfg, path)

	if !cfg.Ldap.TlsVerify {
		t.Fatalf("omitted ldap.tls-verify defaulted to false, want true")
	}
}
