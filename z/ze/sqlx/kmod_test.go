package sqlx_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/sqlx"
)

type BaseDO struct {
	Disable sql.NullBool   `db:"disable"`
	Deleted sql.NullBool   `db:"deleted"`
	Updated sql.NullTime   `db:"updated"`
	Updater sql.NullString `db:"updater"`
	Created sql.NullTime   `db:"created"`
	Creater sql.NullString `db:"creater"`
	Version sql.NullInt64  `db:"version"`
}

// authz data object
type AuthzDO struct {
	ID      int64          `db:"id"`
	Name    sql.NullString `db:"name"`
	AppKey  sql.NullString `db:"appkey"`
	Secret  sql.NullString `db:"secret"`
	Permiss sql.NullString `db:"permiss"`
	Remarks sql.NullString `db:"remarks"`

	BaseDO

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
		Driver: "mysql",
		// DataSource: "xxx:xxx@tcp(mysql.base.svc:3306)/cfg?charset=utf8&parseTime=True&loc=Asia%2FShanghai",
	}
	dss, err := os.ReadFile("../../../__zmy.txt")
	if err != nil {
		panic(err)
	}
	cfg.DataSource = string(dss)
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

// go test -v z/ze/sqlx/kmod_test.go -run TestSelectAll
func TestSelectAll(t *testing.T) {

	repo := sqlx.NewRepo[AuthzRepo]()
	z.Println(repo.Cols().Select())

	// dsc := sqlx.NewDsc(genDB())
	// datas, err := repo.SelectAll(dsc)
	// if err != nil {
	// 	z.Println(err.Error())
	// } else {
	// 	z.Println(z.ToStr2(datas))
	// }
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelectGet
func TestSelectGet(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

	repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := repo.Get(dsc, 2)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelectGet2
func TestSelectGet2(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

	repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := repo.Get(dsc, 2, "ID", "Name", "AppKey", "secret")
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect0
func TestSelect0(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

	repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := repo.Select(dsc, "id < ?", 3)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect1
func TestSelect1(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

	// repo := sqlx.NewRepo[AuthzRepo]()

	datas, err := sqlx.SelectBy[AuthzDO](dsc, sqlx.NewAs("_a"), "_a.id < 3")
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestInsert1
func TestInsert1(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

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

// go test -v z/ze/sqlx/kmod_test.go -run TestUpdate1
func TestUpdate1(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

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

// go test -v z/ze/sqlx/kmod_test.go -run TestUpdate2
func TestUpdate2(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

	repo := sqlx.NewRepo[AuthzRepo]()
	data := &AuthzDO{
		ID:   12,
		Name: sqlx.NewString("test12"),
	}

	err := sqlx.UpdateBy(dsc, data, repo.ColsByExc("disable", "deleted"), "id = ?", 13)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(data))
	}

	data, _ = repo.Get(dsc, 13)
	z.Println(z.ToStr2(data))
}

// go test -v z/ze/sqlx/kmod_test.go -run TestDelete1
func TestDelete1(t *testing.T) {
	dsc := sqlx.NewDsc(genDB())

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

// go test -v z/ze/sqlx/kmod_test.go -run TestTx1
func TestTx1(t *testing.T) {

	repo := sqlx.NewRepo[AuthzRepo]()
	data := AuthzDO{
		ID:   13,
		Name: sqlx.NewString("test123456"),
	}
	// data.Version = sqlx.NewInt64(3)
	ds := genDB()

	err := sqlx.WithTx(ds, func(tx *sqlx.Tx) error {
		err := repo.Update(&sqlx.Dsx{Ex: tx}, &data)
		return err
	})
	if err != nil {
		z.Println(err.Error())
		return
	}
	data.Name.String = ""
	repo.Getx(&sqlx.Dsx{Ex: ds}, 13, &data)
	z.Println(z.ToStr2(data))
}

// go test -v z/ze/sqlx/kmod_test.go -run TestTx2
func TestTx2(t *testing.T) {

	repo := sqlx.NewRepo[AuthzRepo]()
	data := AuthzDO{
		ID:   13,
		Name: sqlx.NewString("test123456"),
	}
	data.Version = sqlx.NewInt64(4)
	ds := genDB()
	cx := context.TODO()

	err := sqlx.WithTxCtx(ds, cx, nil, func(tx *sqlx.Tx) error {
		err := repo.Update(&sqlx.Dsx{Ex: tx, Cx: cx}, &data)
		return err
	})
	if err != nil {
		z.Println(err.Error())
		return
	}
	data.Name.String = ""
	repo.Getx(&sqlx.Dsx{Ex: ds, Cx: cx}, 13, &data)
	z.Println(z.ToStr2(data))
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelectAll2
func TestSelectAll2(t *testing.T) {

	repo := sqlx.NewRepo[AuthzRepo]()

	dsc := sqlx.NewDsc(genDB())
	datas, err := repo.SelectAll(dsc)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect3
func TestSelect3(t *testing.T) {

	repo := sqlx.NewRepo[AuthzRepo]()

	dsc := sqlx.NewDsc(genDB())
	page := sqlx.NewPage(2, 3)
	data := AuthzDO{
		ID:   13,
		Name: sqlx.NewString("test123456"),
	}

	datas, err := repo.Select(dsc, "id = :id", &data, page)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect4
func TestSelect4(t *testing.T) {

	repo := sqlx.NewRepo[AuthzRepo]()

	dsc := sqlx.NewDsc(genDB())

	page := sqlx.NewPage(2, 3)
	data := map[string]any{
		"id": 13,
	}

	datas, err := repo.Select(dsc, "id = :id", data, page)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect5
func TestSelect5(t *testing.T) {

	repo := sqlx.NewRepo[AuthzRepo]()

	dsc := sqlx.NewDsc(genDB())

	page := sqlx.NewPage(2, 3)
	data := map[string]any{
		"id": 13,
	}

	datas, err := repo.Select(dsc, "id = @id", data, page)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr2(datas))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect6
func TestSelect6(t *testing.T) {
	sqlx.RegKsqlEvalue("entity", sqlx.KsqlTblExt)
	sqlx.C.Sqlx.KsqlTbl = true

	_ = sqlx.NewRepo[AuthzRepo]()
	dsc := sqlx.NewDsc(genDB())

	page := sqlx.NewPage(1, 3)
	stmt := `
SELECT /*+ xxx */ * FROM {::entity.AuthzDO} WHERE 1=1
{:id AND id=:id}
`
	argv := map[string]any{
		"id": 13,
	}

	dst, cnt, err := sqlx.Ksql[AuthzDO](dsc, stmt, argv, page)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println("count:", cnt, ", items:", z.ToStr2(dst))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect7
func TestSelect7(t *testing.T) {
	sqlx.RegKsqlEvalue("entity", sqlx.KsqlTblExt)
	sqlx.C.Sqlx.KsqlTbl = true

	_ = sqlx.NewRepo[AuthzRepo]()
	dsc := sqlx.NewDsc(genDB())

	page := sqlx.NewPage(1, 3)
	stmt := `
SELECT 
/** count(id) */
/*+ INDEX(id) */
* FROM {::entity.AuthzDO} WHERE 1=1
{:id AND id=:id}
{:xx AND xx=:xx}
`
	argv := map[string]any{
		"id": 13,
	}

	dst, cnt, err := sqlx.Ksql[AuthzDO](dsc, stmt, argv, page)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println("count:", cnt, ", items:", z.ToStr2(dst))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect8
func TestSelect8(t *testing.T) {
	sqlx.RegKsqlEvalue("entity", sqlx.KsqlTblExt)
	sqlx.C.Sqlx.KsqlTbl = true

	_ = sqlx.NewRepo[AuthzRepo]()
	dsc := sqlx.NewDsc(genDB())

	page := sqlx.NewPage(1, 3)
	stmt := `
SELECT 
/** count(id) */
id FROM {::entity.AuthzDO} WHERE 1=1
{:id AND id=:id}
{:xx AND xx=:xx}
ORDER BY id
`
	argv := map[string]any{
		// "id": 13,
	}

	dst, cnt, err := sqlx.Ksql[int](dsc, stmt, argv, page)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println("count:", cnt, ", items:", z.ToStr2(dst))
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestSelect9
func TestSelect9(t *testing.T) {
	sqlx.RegKsqlEvalue("entity", sqlx.KsqlTblExt)
	sqlx.C.Sqlx.KsqlTbl = true

	dsc := sqlx.NewDsc(genDB())

	repo := sqlx.NewRepo[AuthzRepo]()
	page := sqlx.NewPage(1, 3)
	data, _ := repo.Get(dsc, 13)
	// z.Println("[__test__]:", z.ToStr(data))

	data.Name = sqlx.NewString("demo123456")
	// argv := zc.ToMap(data, "db", false)
	argv := repo.ToMap(data)
	// argv := sqlx.ToMapBy(nil, data, true, true)
	z.Println("[__test__]:", z.ToStr(argv))

	stmt := `update {::entity.AuthzDO} set name=:name {:string1 string1=:string1} where id=:id`
	dst, cnt, err := sqlx.Ksql[int](dsc, stmt, argv, page)
	if err != nil {
		z.Println("[__test__]:", err.Error())
	} else {
		z.Println("[__test__]:", "count:", cnt, ", items:", z.ToStr2(dst))
	}
	item, _ := repo.Get(dsc, 13)
	z.Println("[__test__]:", z.ToStr(item))
}

// go test -v z/ze/sqlx/kmod_test.go -run TestMod1
func TestMod1(t *testing.T) {
	data := &AuthzDO{
		ID:   13,
		Name: sqlx.NewString("demo123456"),
	}
	// argv := zc.ToMap(data, "db", false)
	argv, err := sqlx.ToMapBy(nil, data, false, true)
	z.Println("[__test__]:", z.ToStr(argv), err)
}

// go test -v z/ze/sqlx/kmod_test.go -run TestMod2
func TestMod2(t *testing.T) {
	data := map[string]any{
		"id":   13,
		"name": sqlx.NewString("demo123456"),
	}
	// argv := zc.ToMap(data, "db", false)
	argv, err := sqlx.ToMapBy(nil, data, false, true)
	z.Println("[__test__]:", z.ToStr(argv), err)
}

func (r *AuthzRepo) GetByKsqlDemo(dsc sqlx.Dsc, isAny bool) ([]AuthzDO, int64, error) {
	kgr := func(key string) (string, error) {
		z.Println("[_ksqlget]: ========", key)
		return "SELECT /** count(id) */ * FROM {::entity.AuthzDO} WHERE 1=1 {:id AND id=:id}", nil
	}
	if isAny {
		data := &AuthzDO{ID: 13, Name: sqlx.NewString("demo123456")}
		return r.KsqlAny_(kgr, 0, dsc, data, sqlx.NewPage(1, 1))
	}
	page := &sqlx.Pagx{3, 1, true}
	data := map[string]any{"name": sqlx.NewString("demo123456"), "__page__": page}
	return r.KsqlMap_(kgr, 0, dsc, data, nil)
}

// go test -v z/ze/sqlx/kmod_test.go -run TestKsql1
func TestKsql1(t *testing.T) {
	sqlx.RegKsqlEvalue("entity", sqlx.KsqlTblExt)
	sqlx.C.Sqlx.KsqlTbl = true
	dsc := sqlx.NewDsc(genDB())

	repo := sqlx.NewRepo[AuthzRepo]()
	rst, siz, err := repo.GetByKsqlDemo(dsc, true)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr(rst), "size:", siz)
	}
}

// go test -v z/ze/sqlx/kmod_test.go -run TestKsql2
func TestKsql2(t *testing.T) {
	sqlx.RegKsqlEvalue("entity", sqlx.KsqlTblExt)
	sqlx.C.Sqlx.KsqlTbl = true
	dsc := sqlx.NewDsc(genDB())

	repo := sqlx.NewRepo[AuthzRepo]()
	rst, siz, err := repo.GetByKsqlDemo(dsc, false)
	if err != nil {
		z.Println(err.Error())
	} else {
		z.Println(z.ToStr(rst), "size:", siz)
	}
}
