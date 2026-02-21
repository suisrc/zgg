package sqlx_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/sqlx"

	_ "github.com/go-sql-driver/mysql"
)

// authz data object
type AuthzDO struct {
	ID      int64          `db:"id"`
	Name    sql.NullString `db:"name"`
	AppKey  sql.NullString `db:"appkey"`
	Secret  sql.NullString `db:"secret"`
	Permiss sql.NullString `db:"permiss"`
	Remarks sql.NullString `db:"remarks"`

	Disable bool           `db:"disable"`
	Deleted bool           `db:"deleted"`
	Updated sql.NullTime   `db:"updated"`
	Updater sql.NullString `db:"updater"`
	Created sql.NullTime   `db:"created"`
	Creater sql.NullString `db:"creater"`
	Version sql.NullInt64  `db:"version"`

	Expired sql.NullTime   `db:"expired"`
	String1 sql.NullString `db:"string1"`
	String2 sql.NullString `db:"string2"`
	String3 sql.NullString `db:"string3"`
}

func (AuthzDO) TableName() string {
	return "authz"
}

type AuthzRepo struct {
	sqlx.Repo[AuthzDO]
}

func genDB() *sqlx.DB {
	sqlx.C.Sqlx.ShowSQL = true
	cfg := sqlx.DatabaseConfig{
		Driver:     "mysql",
		DataSource: "cfg:xxx@tcp(mysql.base.svc:3306)/cfg?charset=utf8&parseTime=True&loc=Asia%2FShanghai",
	}
	dsc, err := sqlx.ConnectDatabase(&cfg)
	if err != nil {
		panic(err)
	} else {
		dsn := cfg.DataSource
		if idx := strings.Index(dsn, "@"); idx > 0 {
			dsn = dsn[idx+1:]
		}
		z.Println("[database]: connect ok,", dsn)
	}

	return dsc
}

// go test -v z/ze/sqlx/model_test.go -run TestSelectAll
func TestSelectAll(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()
	z.Println(repo.ColsBy(nil).Select())

	datas, err := repo.SelectAll(dsc)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestSelectGet
func TestSelectGet(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := repo.Get(dsc, nil, 2)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestSelectGet2
func TestSelectGet2(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := repo.Get(dsc, nil, 2, "ID", "Name", "AppKey", "secret")
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestSelect
func TestSelect(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := repo.Select(dsc, "id < ?", 3)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestSelect1
func TestSelect1(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := repo.SelectBy(dsc, repo.ColsBy(nil, "_a."), "_a.id < 3")
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestInsert1
func TestInsert1(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()
	data := AuthzDO{
		Name: sqlx.NewString("test"),
	}

	err := repo.Insert(dsc, &data)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(data))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestUpdate1
func TestUpdate1(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()
	data := AuthzDO{
		ID:   12,
		Name: sqlx.NewString("test12"),
	}

	err := repo.Update(dsc, &data)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(data))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestUpdate2
func TestUpdate2(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()
	data := AuthzDO{
		ID:   12,
		Name: sqlx.NewString("test12"),
	}

	err := repo.UpdateBy(dsc, &data, repo.ColsByExc("disable", "deleted"), "id = ?", 13)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(data))
	}

	repo.Get(dsc, &data, 13)
	z.Println(z.ToStr2(data))
}

// go test -v z/ze/sqlx/model_test.go -run TestDelete1
func TestDelete1(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()
	data := AuthzDO{
		ID:   12,
		Name: sqlx.NewString("test12"),
	}

	err := repo.Delete(dsc, &data)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(data))
	}
}

// go test -v z/ze/sqlx/model_test.go -run TestTx1
func TestTx1(t *testing.T) {
	dsc := genDB()

	repo := sqlx.NewRepo[AuthzRepo]()
	data := AuthzDO{
		ID:   13,
		Name: sqlx.NewString("test123456"),
	}

	err := sqlx.WithTx(dsc, func(tx *sqlx.Tx) error {
		err := repo.Update(tx, &data)
		return err
	})
	if err != nil {
		z.Println(err.Error())
		return
	}
	data.Name.String = ""
	repo.Get(dsc, &data, 13)
	z.Println(z.ToStr2(data))
}
