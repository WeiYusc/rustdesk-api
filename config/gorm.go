package config

import "time"

const (
	TypeSqlite     = "sqlite"
	TypeMysql      = "mysql"
	TypePostgresql = "postgresql"
)

type Gorm struct {
	Type            string        `mapstructure:"type"`
	MaxIdleConns    int           `mapstructure:"max-idle-conns"`
	MaxOpenConns    int           `mapstructure:"max-open-conns"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn-max-idle-time"`
	ConnMaxLifetime time.Duration `mapstructure:"conn-max-lifetime"`
}

type Mysql struct {
	Addr     string `mapstructure:"addr"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Dbname   string `mapstructure:"dbname"`
	Tls      string `mapstructure:"tls"` // true / false / skip-verify / custom
}

type Postgresql struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Dbname   string `mapstructure:"dbname"`
	Sslmode  string `mapstructure:"sslmode"`   // "disable", "require", "verify-ca", "verify-full"
	TimeZone string `mapstructure:"time-zone"` // e.g., "Asia/Shanghai"
}
