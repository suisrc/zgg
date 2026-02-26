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
	Patch(string, any, Page) (string, error)
}

func NewDsc(ex interface {
	Ext
	ExtContext
}) Dsc {
	return &Dsx{Ex: ex}
}

type Dsx struct {
	Ex interface {
		Ext
		ExtContext
	}
	Cx context.Context
}

func (r *Dsx) Ext() Ext {
	return r.Ex
}

func (r *Dsx) Exc() ExtContext {
	return r.Ex
}

func (r *Dsx) Ctx() context.Context {
	return r.Cx
}

func (r *Dsx) Patch(stmt string, args any, page Page) (string, error) {
	// 扩展， 用于对 sql 打补丁，比如打印等
	if page != nil {
		stmt = page.Patch(stmt, r.Ex.DriverName())
	}
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	return stmt, nil
}

type Page interface {
	First() int64
	Limit() int64
	Stats() bool
	Patch(stmt, driver string) string
}

// 小于 0 是禁用分页， 等于 0 是使用默认值
func NewPage(page, size int64) Page {
	return &Pagx{Page: page, Size: size}
}

type Pagx struct {
	Page int64 // <0 First = -Page, 自定义偏移
	Size int64 // <0 不限制， =0 默认值 10
	IsTS bool  // 统计总量 Total Statistics
}

func (p *Pagx) First() int64 {
	if p.Page < 0 {
		return -p.Page
	}
	if p.Page < 1 {
		return 0
	}
	return (p.Page - 1) * p.Limit()
}

func (p *Pagx) Limit() int64 {
	if p.Size < 0 {
		return 0
	}
	if p.Size == 0 {
		return 10
	}
	return p.Size
}

func (p *Pagx) Stats() bool {
	return p.IsTS
}

func (r *Pagx) Patch(stmt, driver string) string {
	first, limit := r.First(), r.Limit()
	if first <= 0 && limit <= 0 {
		return stmt
	}
	// 分页, 只针对 select 语句
	switch driver {
	case "mysql", "sqlite3", "sqlite", "postgres", "pgx", "pg": // LIMIT ... OFFSET 是数据库扩展语法，仅在部分数据库中支持
		// 大偏移量场景下都建议使用键值分页替代：
		// SELECT * FROM users  WHERE id < (SELECT id FROM users ORDER BY id DESC LIMIT 1 OFFSET 10) ORDER BY id DESC LIMIT 10;
		if first > 0 && limit > 0 {
			return stmt + fmt.Sprintf(" LIMIT %d OFFSET %d", limit, first)
		} else if limit > 0 {
			return stmt + fmt.Sprintf(" LIMIT %d", limit)
		} else if first > 0 {
			return stmt + fmt.Sprintf(" OFFSET %d", first)
		}
	case "sqlserver", "ibmdb", "ora", "godror", "dm": // postgres, OFFSET ... FETCH NEXT 是 SQL:2008 标准语法
		if first > 0 && limit > 0 {
			return stmt + fmt.Sprintf(" OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", first, limit)
		} else if limit > 0 {
			return stmt + fmt.Sprintf(" OFFSET 0 ROWS FETCH NEXT %d ROWS ONLY", limit)
		} else if first > 0 {
			return stmt + fmt.Sprintf(" OFFSET %d", first)
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
	var err error
	stmt, err = dsc.Patch(stmt, args, nil)
	if err != nil {
		return nil, err
	}
	if ctx := dsc.Ctx(); ctx != nil {
		return data, GetContext(ctx, dsc.Exc(), data, stmt, args...)
	}
	return data, Get(dsc.Ext(), data, stmt, args...)
}

// 查询列表, page 必须是最后一个参数
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
	var page Page
	if len(args) > 0 {
		if _page := args[len(args)-1]; _page == nil {
			// 最后一个参数可能是分页对象
		} else if pg, ok := _page.(Page); ok {
			page = pg
			args = args[:len(args)-1]
		}
	}
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
		stm, err = dsc.Patch(stm, arg, page)
		if err != nil {
			return nil, err
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
		stmt, rerr = dsc.Patch(stmt, args, page)
		if rerr != nil {
			return nil, rerr
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
	var err error
	stmt, err = dsc.Patch(stmt, args, nil)
	if err != nil {
		return err
	}
	var rst sql.Result
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
	var err error
	stmt, err = dsc.Patch(stmt, argv, nil)
	if err != nil {
		return err
	}
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
	var err error
	stmt, err = dsc.Patch(stmt, args, nil)
	if err != nil {
		return err
	}
	if ctx := dsc.Ctx(); ctx != nil {
		_, err = dsc.Exc().ExecContext(ctx, stmt, args...)
	} else {
		_, err = dsc.Ext().Exec(stmt, args...)
	}
	return err
}

// =============================================================================

// snil: true 保留 nil 值, dval: true 处理 driver.Valuer -> any
func ToMapBy(stm *StructMap, arg any, snil bool, dval bool) (map[string]any, error) {
	rst := map[string]any{}
	if arg == nil {
		return rst, errors.New("arg is <nil>")
	}
	typ := reflect.TypeOf(arg)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	// MAP -> map[string]any
	if typ.Kind() == reflect.Map && typ.Key().Kind() == reflect.String {
		// if m, ok := arg.(map[string]any); ok {
		// 	return m, nil
		// }
		ref := reflect.ValueOf(arg)
		for _, key := range ref.MapKeys() {
			vv := ref.MapIndex(key).Interface()
			if dval {
				if v2, ok := vv.(driver.Valuer); ok {
					vv, _ = v2.Value()
				}
			}
			if snil || vv != nil {
				rst[key.String()] = vv
			}
		}
		return rst, nil
	} else if typ.Kind() != reflect.Struct {
		return rst, errors.New("arg is not struct")
	}
	// STRUCT TAG['db'] -> map[string]any
	if stm == nil {
		stm = mapper().TypeMap(typ)
	}
	val := reflect.ValueOf(arg)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	for _, fi := range stm.GetIndexName() {
		vv := val.FieldByIndex(fi.Index).Interface()
		if dval {
			if v2, ok := vv.(driver.Valuer); ok {
				vv, _ = v2.Value()
			}
		}
		if snil || vv != nil {
			rst[fi.Name] = vv
		}
	}
	return rst, nil
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
