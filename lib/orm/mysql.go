package orm

import (
	"database/sql"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultMysqlConnMaxIdleTime = time.Hour
	defaultMysqlConnMaxLifetime = 7*time.Hour + 30*time.Minute
)

type MysqlConfig struct {
	Dsn             string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

func NewMysql(mysqlConf *MysqlConfig, logwriter logger.Writer) *gorm.DB {
	db, err := NewMysqlWithError(mysqlConf, logwriter)
	if err != nil {
		panic(err)
	}
	return db
}

func NewMysqlWithError(mysqlConf *MysqlConfig, logwriter logger.Writer) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:               mysqlConf.Dsn, // DSN data source name
		DefaultStringSize: 256,           // string 类型字段的默认长度
		//DisableDatetimePrecision:  true,                    // 禁用 datetime 精度，MySQL 5.6 之前的数据库不支持
		//DontSupportRenameIndex:    true,                    // 重命名索引时采用删除并新建的方式，MySQL 5.7 之前的数据库和 MariaDB 不支持重命名索引
		//DontSupportRenameColumn:   true,                    // 用 `change` 重命名列，MySQL 8 之前的数据库和 MariaDB 不支持重命名列
		//SkipInitializeWithVersion: false,                   // 根据当前 MySQL 版本自动配置
	}), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.New(
			logwriter, // io writer
			logger.Config{
				SlowThreshold:             time.Second, // Slow SQL threshold
				LogLevel:                  logger.Warn, // Log level
				IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
				ParameterizedQueries:      true,        // Don't include params in the SQL log
				Colorful:                  true,
			},
		),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	applyMysqlConnPoolConfig(sqlDB, mysqlConf)

	return db, nil
}

func applyMysqlConnPoolConfig(sqlDB *sql.DB, mysqlConf *MysqlConfig) {
	// SetMaxIdleConns 设置空闲连接池中连接的最大数量
	sqlDB.SetMaxIdleConns(mysqlConf.MaxIdleConns)

	// SetMaxOpenConns 设置打开数据库连接的最大数量。
	sqlDB.SetMaxOpenConns(mysqlConf.MaxOpenConns)

	connMaxIdleTime := mysqlConf.ConnMaxIdleTime
	if connMaxIdleTime <= 0 {
		connMaxIdleTime = defaultMysqlConnMaxIdleTime
	}
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	connMaxLifetime := mysqlConf.ConnMaxLifetime
	if connMaxLifetime <= 0 {
		connMaxLifetime = defaultMysqlConnMaxLifetime
	}
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
}
