package sqlx

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	C = struct {
		Sqlx Config
	}{}
)

type Config struct {
	ShowSQL bool `json:"showsql"`
}

type DatabaseConfig struct {
	Driver       string `json:"driver"` // mysql
	DataSource   string `json:"dsn"`    // user:pass@tcp(host:port)/dbname?params
	Host         string `json:"host"`
	Port         int    `json:"port" default:"3306"`
	DBName       string `json:"dbname"`
	Params       string `json:"params"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	MaxOpenConns int    `json:"max_open_conns"`
	MaxIdleConns int    `json:"max_idle_conns"`
	MaxIdleTime  int    `json:"max_idle_time"` // 单位秒
	MaxLifetime  int    `json:"max_lifetime"`
	TablePrefix  string `json:"table_prefix"`
}

func ConnectDatabase(cfg *DatabaseConfig) (*DB, error) {
	if cfg.DataSource == "" {
		if cfg.Host != "" {
			cfg.DataSource = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", //
				cfg.Username, cfg.Password, //
				cfg.Host, cfg.Port, //
				cfg.DBName, cfg.Params, //
			)
		} else {
			return nil, errors.New("database dsn is empty")
		}
	}
	// dbs, err := sql.Open("mysql", "")
	cds, err := Connect(cfg.Driver, cfg.DataSource)
	if err != nil {
		dsn := cfg.DataSource
		if idx := strings.Index(dsn, "@"); idx > 0 {
			dsn = dsn[idx:]
		}
		return nil, errors.New("database connect error [***" + dsn + "]" + err.Error())
	}
	// 设置数据库连接参数
	if cfg.MaxOpenConns > 0 {
		cds.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		cds.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxIdleTime > 0 {
		cds.SetConnMaxIdleTime(time.Duration(cfg.MaxIdleTime) * time.Second)
	}
	if cfg.MaxLifetime > 0 {
		cds.SetConnMaxLifetime(time.Duration(cfg.MaxLifetime) * time.Second)
	}
	return cds, nil
}
