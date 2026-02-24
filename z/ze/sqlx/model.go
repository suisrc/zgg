package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

var (
	reNamedStm = regexp.MustCompile(`:\w+`) // `[:@]\w+`
)

func WithTx(dsc *DB, fn func(tx *Tx) error) error {
	tx, err := dsc.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // 重新抛出 panic
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()
	err = fn(tx)
	return err
}

func WithTxCtx(dsc *DB, ctx context.Context, opt *sql.TxOptions, fn func(tx *Tx) error) error {
	tx, err := dsc.BeginTxx(ctx, opt)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // 重新抛出 panic
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()
	err = fn(tx)
	return err
}

// datasource connect

type Dsx struct {
	Ex interface {
		Ext
		ExtContext
	}
	Cx context.Context

	First int64
	Limit int64
}

func (r *Dsx) Ctx() context.Context {
	return r.Cx
}

func (r *Dsx) Ext() Ext {
	return r.Ex
}

func (r *Dsx) Exc() ExtContext {
	return r.Ex
}

// 小于 0 是禁用分页， 等于 0 是使用默认值
func (r *Dsx) Page(page, size int64) {
	if size < 0 || page < 0 {
		// 禁用分页，需要查询全部
		r.First = 0
		r.Limit = 0
		return
	}
	if size == 0 {
		size = 10
	}
	r.Limit = size
	if page == 0 {
		r.First = 0
	} else {
		r.First = (page - 1) * size
	}
}

func (r *Dsx) Offset() int64 {
	return r.First
}

func (r *Dsx) Prepro(stmt string) string {
	if r.First <= 0 && r.Limit <= 0 || !zc.HasPrefixFold(stmt, SQL_SELECT) {
		return stmt
	}
	// 分页, 只针对 select 语句
	switch r.Ex.DriverName() {
	case "mysql", "sqlite3", "sqlite", "postgres", "pgx", "pg": // LIMIT ... OFFSET 是数据库扩展语法，仅在部分数据库中支持
		// 大偏移量场景下都建议使用键值分页替代：
		// SELECT * FROM users  WHERE id < (SELECT id FROM users ORDER BY id DESC LIMIT 1 OFFSET 10) ORDER BY id DESC LIMIT 10;
		if r.First > 0 && r.Limit > 0 {
			return stmt + fmt.Sprintf(" LIMIT %d OFFSET %d", r.Limit, r.First)
		} else if r.Limit > 0 {
			return stmt + fmt.Sprintf(" LIMIT %d", r.Limit)
		} else if r.First > 0 {
			return stmt + fmt.Sprintf(" OFFSET %d", r.First)
		}
	case "sqlserver", "ibmdb", "ora", "godror", "dm": // postgres, OFFSET ... FETCH NEXT 是 SQL:2008 标准语法
		if r.First > 0 && r.Limit > 0 {
			return stmt + fmt.Sprintf(" OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", r.First, r.Limit)
		} else if r.Limit > 0 {
			return stmt + fmt.Sprintf(" OFFSET 0 ROWS FETCH NEXT %d ROWS ONLY", r.Limit)
		} else if r.First > 0 {
			return stmt + fmt.Sprintf(" OFFSET %d", r.First)
		}
	}
	return stmt
}

type Dsc interface {
	Ext() Ext        // 默认上下文执行器
	Exc() ExtContext // 指定上下文执行器
	Ctx() context.Context

	Offset() int64        // 用于快速判断是否还需要继续查询
	Prepro(string) string // 修复sql文，如增加增强内容或分页
}

// repo 初始化方法
type RepoInit interface {
	InitRepo()
}

func NewRepo[T any]() *T {
	repo := new(T)
	if ri, ok := any(repo).(RepoInit); ok {
		ri.InitRepo()
	}
	return repo
}

// =========================================================

type Tabler interface {
	TableName() string
}

type Nuller interface {
	sql.Scanner
	driver.Valuer
}

type Repo[T any] struct {
	Typ reflect.Type // data type
	Stm *StructMap   // struct map
}

// 忽略 没有被 "db" 标记的属性, 即 Repo 对应的 DO 必须具有 "db" 标签
// var RepoMpr = NewMapperFunc("db", func(s string) string { return "-" })

func (r *Repo[T]) InitRepo() {
	r.Typ = reflect.TypeFor[T]()
	r.Stm = mapper().TypeMap(r.Typ)
	RegKsqlEnt(r.Typ.Name(), r.Table(new(T)))
}

// 获取 Table Name， 这样的优势在于，可以通过 obj 的值进行分表操作
func (r *Repo[T]) Table(obj any) string {
	if tabler, ok := obj.(Tabler); ok {
		return tabler.TableName()
	} else {
		return strings.ToLower(r.Typ.Name())
	}
}

// func (r *Repo[T]) Entity() *T { return new(T) }
// func (r *Repo[T]) Arrays() []T { return []T{} }

// =============================================================================

// cols 第一个可能是别名， 格式为 xxx. 以 . 结尾
func (r *Repo[T]) ColsBy(chk func(map[string]int, *FieldInfo) bool, cols ...string) *Columns {
	alias := ""
	if len(cols) > 0 && strings.HasSuffix(cols[0], ".") {
		alias = cols[0][:len(cols[0])-1]
		cols = cols[1:]
	}
	if len(cols) == 0 || chk == nil {
		rst := NewColumns(alias, r.Stm)
		for _, val := range r.Stm.GetIndexName() {
			rst.Append(Column{CName: val.Name, Field: val.GetFieldName()})
		}
		return rst
	}
	rst := NewColumns(alias, r.Stm)
	emap := ExistMap(cols...)
	for _, val := range r.Stm.GetIndexName() {
		if chk(emap, val) {
			rst.Append(Column{CName: val.Name, Field: val.GetFieldName()})
		}
	}
	return rst
}

func (r *Repo[T]) Cols() *Columns {
	return r.ColsBy(nil)
}

func (r *Repo[T]) ColsByExc(cols ...string) *Columns {
	return r.ColsBy(func(emap map[string]int, val *FieldInfo) bool {
		_, ok := emap[val.Name]
		return !ok
	}, cols...)
}

func (r *Repo[T]) ColsByInc(cols ...string) *Columns {
	return r.ColsBy(func(emap map[string]int, val *FieldInfo) bool {
		_, ok := emap[val.Name]
		return ok
	}, cols...)
}

func (r *Repo[T]) ColsByExf(flds ...string) *Columns {
	return r.ColsBy(func(emap map[string]int, val *FieldInfo) bool {
		_, ok := emap[val.GetFieldName()]
		return !ok
	}, flds...)
}

func (r *Repo[T]) ColsByInf(flds ...string) *Columns {
	return r.ColsBy(func(emap map[string]int, val *FieldInfo) bool {
		_, ok := emap[val.GetFieldName()]
		return ok
	}, flds...)
}

// =============================================================================
// Select

func (r *Repo[T]) Get(dsc Dsc, data *T, id int64, flds ...string) (*T, error) {
	if data == nil {
		data = new(T)
	}
	stmt := SQL_SELECT + r.ColsByInf(flds...).Select() + SQL_FROM + r.Table(data) + SQL_WHERE + fmt.Sprintf("id=%d", id)
	stmt = dsc.Prepro(stmt)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s", stmt)
	}
	if ctx := dsc.Ctx(); ctx != nil {
		return data, GetContext(ctx, dsc.Exc(), data, stmt)
	}
	return data, Get(dsc.Ext(), data, stmt)
}

func (r *Repo[T]) GetBy(dsc Dsc, data *T, cond string, args ...any) (*T, error) {
	if data == nil {
		data = new(T)
	}
	stmt := SQL_SELECT + r.ColsByExf().Select() + SQL_FROM + r.Table(data) + SQL_WHERE + cond
	stmt = dsc.Prepro(stmt)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	if ctx := dsc.Ctx(); ctx != nil {
		return data, GetContext(ctx, dsc.Exc(), data, stmt, args...)
	}
	return data, Get(dsc.Ext(), data, stmt, args...)
}

func (r *Repo[T]) SelectBy(dsc Dsc, cols *Columns, cond string, args ...any) ([]T, error) {
	stmt := SQL_SELECT + cols.Select() + SQL_FROM + r.Table(new(T))
	if cols.As != "" {
		// 别名
		stmt += " " + cols.As
	}
	if cond != "" {
		// 条件
		stmt += SQL_WHERE + cond
	}
	stmt = dsc.Prepro(stmt)
	var rows *Rows
	var rerr error
	if len(args) != 1 || !reNamedStm.MatchString(stmt) {
		// ignore index parameters 忽略索引参数
	} else if typ := Deref(reflect.TypeOf(args[0])); typ.Kind() == reflect.Struct || //
		typ.Kind() == reflect.Map && typ.Key().Kind() == reflect.String {
		// 命名参数, 处理逻辑本身来自 NamedQuery 函数
		e := dsc.Ext() // Ext & Exc 来自同一个 DB | TX
		stm, arg, err := bindNamedMapper(BindType(e.DriverName()), stmt, args[0], mapperFor(e))
		if err != nil {
			return nil, err
		}
		if C.Sqlx.ShowSQL {
			z.Printf("[_showsql]: %s -------- %s | %s", stmt, stm, z.ToStr(arg))
		}
		if ctx := dsc.Ctx(); ctx != nil {
			// rows, rerr = NamedQueryContext(ctx, dsc.Exc(), stmt, args[0])
			rows, rerr = dsc.Exc().QueryxContext(ctx, stm, arg...)
		} else {
			// rows, rerr = NamedQuery(dsc.Ext(), stmt, args[0])
			rows, rerr = dsc.Ext().Queryx(stm, arg...)
		}
	}
	if rows == nil && rerr == nil {
		// 未执行任何查询， 使用索引参数
		if C.Sqlx.ShowSQL {
			z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
		}
		if ctx := dsc.Ctx(); ctx != nil {
			rows, rerr = dsc.Exc().QueryxContext(ctx, stmt, args...)
		} else {
			rows, rerr = dsc.Ext().Queryx(stmt, args...)
		}
	}
	if rerr != nil {
		return nil, rerr
	}
	defer rows.Close()
	dest := []T{}
	if err := scanAll(rows, &dest, false); err != nil {
		return nil, err
	}
	return dest, nil
}

func (r *Repo[T]) SelectAll(dsc Dsc) ([]T, error) {
	return r.SelectBy(dsc, r.Cols(), "")
}

func (r *Repo[T]) Select(dsc Dsc, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.Cols(), cond, args...)
}

func (r *Repo[T]) SelectByExc(dsc Dsc, cols []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByExc(cols...), cond, args...)
}

func (r *Repo[T]) SelectByInc(dsc Dsc, cols []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByInc(cols...), cond, args...)
}

func (r *Repo[T]) SelectByExf(dsc Dsc, flds []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByExf(flds...), cond, args...)
}

func (r *Repo[T]) SelectByInf(dsc Dsc, flds []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByInf(flds...), cond, args...)
}

// =============================================================================
// Insert

func (r *Repo[T]) InsertBy(dsc Dsc, data *T, setid func(int64)) error {
	if data == nil {
		return errors.New("data is nil")
	}
	cols := r.ColsByExc("id")
	stmt, args := cols.InsertArgs(data, true)
	stmt = SQL_INSERT + r.Table(data) + stmt
	stmt = dsc.Prepro(stmt)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	var rst sql.Result
	var err error
	if ctx := dsc.Ctx(); ctx != nil {
		rst, err = dsc.Exc().ExecContext(ctx, stmt, args...)
	} else {
		rst, err = dsc.Ext().Exec(stmt, args...)
	}
	var iid int64
	if err != nil {
	} else if setid == nil {
	} else if iid, err = rst.LastInsertId(); err != nil {
	} else {
		setid(iid)
	}
	return err
}

func (r *Repo[T]) Insert(dsc Dsc, data *T) error {
	fid, _ := r.Stm.Names["id"]
	if fid == nil {
		return r.InsertBy(dsc, data, nil)
	}
	setid := func(id int64) {
		reflect.ValueOf(data).Elem().FieldByIndex(fid.Field.Index).Set(reflect.ValueOf(id))
	}
	return r.InsertBy(dsc, data, setid)
}

// =============================================================================
// Update

func (r *Repo[T]) UpdateBy(dsc Dsc, data *T, cols *Columns, cond string, args ...any) error {
	if data == nil {
		return errors.New("data is nil")
	}
	if cond == "" {
		fid, _ := r.Stm.Names["id"]
		if fid == nil {
			return errors.New("condition is emtpy")
		}
		cond = fmt.Sprintf("id=%v", reflect.ValueOf(data).Elem().FieldByIndex(fid.Field.Index).Interface())
	}
	// cols.DelByCName("id") // id 必须删除
	stmt, argv := cols.UpdateArgs(data, true)
	stmt = SQL_UPDATE + r.Table(data) + stmt + SQL_WHERE + cond
	argv = append(argv, args...)
	stmt = dsc.Prepro(stmt)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(argv))
	}
	var err error
	if ctx := dsc.Ctx(); ctx != nil {
		_, err = dsc.Exc().ExecContext(ctx, stmt, argv...)
	} else {
		_, err = dsc.Ext().Exec(stmt, argv...)
	}
	return err
}

func (r *Repo[T]) Update(dsc Dsc, data *T) error {
	return r.UpdateBy(dsc, data, r.ColsByExc("id", "created", "creater"), "")
}

func (r *Repo[T]) UpdateByExc(dsc Dsc, data *T, cols ...string) error {
	keys := cols[:]
	keys = append(keys, "id", "created", "creater")
	return r.UpdateBy(dsc, data, r.ColsByExc(keys...), "")
}

func (r *Repo[T]) UpdateByInc(dsc Dsc, data *T, cols ...string) error {
	return r.UpdateBy(dsc, data, r.ColsByInc(cols...), "")
}

func (r *Repo[T]) UpdateByExf(dsc Dsc, data *T, flds ...string) error {
	keys := flds[:]
	keys = append(keys, "ID", "Created", "Creater")
	return r.UpdateBy(dsc, data, r.ColsByExf(keys...), "")
}

func (r *Repo[T]) UpdateByInf(dsc Dsc, data *T, flds ...string) error {
	return r.UpdateBy(dsc, data, r.ColsByInf(flds...), "")
}

// =============================================================================
// Delete

func (r *Repo[T]) Delete(dsc Dsc, data *T) error {
	if data == nil {
		return errors.New("data is nil")
	}
	fid, _ := r.Stm.Names["id"]
	if fid == nil {
		return errors.New("id field is nil")
	}
	id := reflect.ValueOf(data).Elem().FieldByIndex(fid.Field.Index).Interface()
	return r.DeleteBy(dsc, fmt.Sprintf("id=%v", id))
}

func (r *Repo[T]) DeleteBy(dsc Dsc, cond string, args ...any) error {
	stmt := SQL_DELETE + SQL_FROM + r.Table(new(T)) + SQL_WHERE + cond
	stmt = dsc.Prepro(stmt)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	var err error
	if ctx := dsc.Ctx(); ctx != nil {
		_, err = dsc.Exc().ExecContext(ctx, stmt, args...)
	} else {
		_, err = dsc.Ext().Exec(stmt, args...)
	}
	return err
}

// =============================================================================

// IsNotFound of sqlx
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	str := err.Error()
	if ok := strings.HasSuffix(str, "no record"); ok {
		return true // 数据不存在
	}
	if ok := strings.HasSuffix(str, " doesn't exist"); ok {
		return true // 数据表不存在，也可以理解成为没有数据
	}
	if ok := strings.HasSuffix(str, " no rows in result set"); ok {
		return true // 数据不存在
	}
	if ok := strings.HasSuffix(str, " no documents in result"); ok {
		return true // 数据不存在(mongo专用)
	}
	return false // 无法处理的内容
}

// Duplicate entry
func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "Error 1062: Duplicate entry ")
}

// 重开事务
func IsReTransaction(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasSuffix(err.Error(), " try restarting transaction")
}
