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

// =============================================================================

type Dsc interface {
	Ext() Ext        // 默认上下文执行器
	Exc() ExtContext // 指定上下文执行器
	Ctx() context.Context

	Offset() int64        // 用于快速判断是否还需要继续查询
	Prepro(string) string // 修复sql文，如增加增强内容或分页
}

var _ Dsc = (*Dsx)(nil)

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

// =============================================================================

type Nuller interface {
	sql.Scanner
	driver.Valuer
}

type Tabler interface {
	TableName() string
}

// 获取 Table Name， 这样的优势在于，可以通过 obj 的值进行分表操作
func TableName(obj any) string {
	if tabler, ok := obj.(Tabler); ok {
		return tabler.TableName()
	} else {
		typ := reflect.TypeOf(obj)
		return strings.ToLower(typ.Name())
	}
}

func Colx[T any](chk func(*FieldInfo) (string, bool), cols ...string) *Cols {
	return ColsBy[T](nil, chk, cols...)
}

// cols 第一个可能是别名， 格式为 xxx. 以 . 结尾
func ColsBy[T any](stm *StructMap, chk func(*FieldInfo) (string, bool), cols ...string) *Cols {
	if stm == nil {
		typ := reflect.TypeFor[T]()
		stm = mapper().TypeMap(typ)
	}
	alias := ""
	if len(cols) > 0 && strings.HasSuffix(cols[0], ".") {
		alias = cols[0][:len(cols[0])-1]
		cols = cols[1:]
	}
	if len(cols) == 0 || chk == nil {
		dest := NewCols(alias, stm)
		for _, val := range stm.GetIndexName() {
			dest.Append(Col{CName: val.Name, Field: val.GetFieldName()})
		}
		return dest
	}
	dest := NewCols(alias, stm)
	emap := ExistMap(cols...)
	for _, val := range stm.GetIndexName() {
		kk, rr := chk(val)
		if _, ok := emap[kk]; rr == ok {
			dest.Append(Col{CName: val.Name, Field: val.GetFieldName()})
		}
	}
	return dest
}

// 查询独享
func GetBy[T any](dsc Dsc, cols *Cols, data *T, cond string, args ...any) (*T, error) {
	// stmt = fmt.Sprintf("select %s from %s where %s", Colx[T](nil).Select(), TableName(data), cond)
	// Get(sqlx.*DB, data, stmt, args...)
	if cols == nil {
		cols = ColsBy[T](nil, nil)
	} else if len(cols.Cols) == 0 {
		cols = ColsBy[T](nil, nil, cols.As+".")
	}
	if data == nil {
		data = new(T)
	}
	stmt := SQL_SELECT + cols.Select() + SQL_FROM + TableName(data) + SQL_WHERE + cond
	stmt = dsc.Prepro(stmt)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	if ctx := dsc.Ctx(); ctx != nil {
		return data, GetContext(ctx, dsc.Exc(), data, stmt, args...)
	}
	return data, Get(dsc.Ext(), data, stmt, args...)
}

// 查询列表
func SelectBy[T any](dsc Dsc, cols *Cols, cond string, args ...any) ([]T, error) {
	if cols == nil {
		cols = ColsBy[T](nil, nil)
	} else if len(cols.Cols) == 0 {
		cols = ColsBy[T](nil, nil, cols.As+".")
	}
	stmt := SQL_SELECT + cols.Select() + SQL_FROM + TableName(new(T))
	if cols.As != "" {
		stmt += " " + cols.As // 别名
	}
	if cond != "" {
		stmt += SQL_WHERE + cond // 条件
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
			z.Printf("[_showsql]: %s --> %s | %s", stmt, stm, z.ToStr(arg))
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

// 插入数据
func InsertBy[T any](dsc Dsc, cols *Cols, data *T, fnid func(int64)) error {
	if data == nil {
		return errors.New("data is nil")
	}
	if cols == nil {
		cols = ColsBy[T](nil, func(val *FieldInfo) (string, bool) { return val.Name, false }, "id")
	}
	stmt, args := cols.InsertArgs(data, true)
	stmt = SQL_INSERT + TableName(data) + stmt
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
	if err != nil {
		return err
	}
	if fnid != nil {
		if eid, err := rst.LastInsertId(); err != nil {
			return err
		} else {
			fnid(eid)
		}
	}
	return nil
}

// 更新数据
func UpdateBy[T any](dsc Dsc, data *T, cols *Cols, cond string, args ...any) error {
	if data == nil {
		return errors.New("data is nil")
	}
	if cond == "" {
		fid, ok := reflect.TypeFor[T]().FieldByName("ID")
		if !ok {
			return errors.New("condition is emtpy")
		}
		cond = fmt.Sprintf("id=%v", reflect.ValueOf(data).Elem().FieldByIndex(fid.Index).Interface())
	}
	// cols.DelByCName("id") // id 必须删除
	stmt, argv := cols.UpdateArgs(data, true)
	stmt = SQL_UPDATE + TableName(data) + stmt + SQL_WHERE + cond
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

// 删除数据
func DeleteBy[T any](dsc Dsc, cond string, args ...any) error {
	stmt := SQL_DELETE + SQL_FROM + TableName(new(T)) + SQL_WHERE + cond
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

// nv: true保留 nil 值, dv: true处理 driver.Valuer -> any
func ToMapBy[T any](stm *StructMap, obj *T, nv bool, dv bool) map[string]any {
	if stm == nil {
		stm = mapper().TypeMap(reflect.TypeFor[T]())
	}
	rst := map[string]any{}
	if obj != nil {
		val := reflect.ValueOf(obj).Elem()
		for _, fi := range stm.GetIndexName() {
			vv := val.FieldByIndex(fi.Index).Interface()
			if dv {
				if v2, ok := vv.(driver.Valuer); ok {
					vv, _ = v2.Value()
				}
			}
			if nv || vv != nil {
				rst[fi.Name] = vv
			}
		}
	}
	return rst
}

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
