package sqlx

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/suisrc/zgg/z/zc"
)

var (
	C = struct {
		Sqlx Config `json:"sqlx"`
	}{}
)

func init() {
	zc.Register(&C)
}

type Config struct {
	KsqlDebug bool `json:"ksqldebug"` // 默认 false，不适用缓存器，用于调试

	ShowSQL bool `json:"showsql"`
	KsqlTbl bool `json:"ksqltbl"` // 默认 false， 是否支持 ksql 收集 table type 和 table name 的映射关系
	TblName struct {
		Prefix  string            `json:"prefix"`
		Mapping map[string]string `json:"mapping"`
	} `json:"namex"` // 表名映射，可以在 DO 的 TableName 方法中调用 GetTableByEnv("XxxDO", "xxxxx") 方法
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

func GetTableByEnv(typ, def string) string {
	if C.Sqlx.TblName.Mapping == nil {
	} else if tbl, ok := C.Sqlx.TblName.Mapping[typ]; ok {
		return C.Sqlx.TblName.Prefix + tbl
	}
	return C.Sqlx.TblName.Prefix + def
}
