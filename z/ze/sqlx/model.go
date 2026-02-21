package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/suisrc/zgg/z"
)

// datasource connect
type DSC Ext

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

// 忽略 没有被 "db" 标记的属性, 即 Repo 对应的 DO 必须具有 "db" 标签
var RepoMpr = NewMapperFunc("db", func(s string) string { return "-" })

// repo 初始化方法
type RepoInf interface {
	InitRepo()
}

func NewRepo[T any]() *T {
	repo := new(T)
	if ri, ok := any(repo).(RepoInf); ok {
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

func (r *Repo[T]) InitRepo() {
	r.Typ = reflect.TypeFor[T]()
	r.Stm = RepoMpr.TypeMap(r.Typ)
}

// 获取 Table Name， 这样的优势在于，可以通过 obj 的值进行分表操作
func (r *Repo[T]) TN(obj any) string {
	if tabler, ok := obj.(Tabler); ok {
		return tabler.TableName()
	} else {
		return strings.ToLower(r.Typ.Name())
	}
}

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
		for _, val := range r.Stm.Index {
			rst.Append(Column{CName: val.Name, Field: val.GetFieldName()})
		}
		return rst
	}
	rst := NewColumns(alias, r.Stm)
	emap := ExistMap(cols...)
	for _, val := range r.Stm.Index {
		if chk(emap, val) {
			rst.Append(Column{CName: val.Name, Field: val.GetFieldName()})
		}
	}
	return rst
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

func (r *Repo[T]) Get(dsc DSC, data *T, id int64, flds ...string) (*T, error) {
	if data == nil {
		data = new(T)
	}
	stmt := SQL_SELECT + r.ColsByInf(flds...).Select() + SQL_FROM + r.TN(data) + SQL_WHERE + fmt.Sprintf("id=%d", id)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s", stmt)
	}
	return data, Get(dsc, data, stmt)
}

func (r *Repo[T]) GetBy(dsc DSC, data *T, cond string, args ...any) (*T, error) {
	if data == nil {
		data = new(T)
	}
	stmt := SQL_SELECT + r.ColsByExf().Select() + SQL_FROM + r.TN(data) + SQL_WHERE + cond
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	return data, Get(dsc, data, stmt, args...)
}

func (r *Repo[T]) SelectBy(dsc DSC, cols *Columns, cond string, args ...any) ([]T, error) {
	stmt := SQL_SELECT + cols.Select() + SQL_FROM + r.TN(new(T))
	if cols.As != "" {
		// 别名
		stmt += " " + cols.As
	}
	if cond != "" {
		// 条件
		stmt += SQL_WHERE + cond
	}
	if C.Sqlx.ShowSQL {
		// 显示SQL
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	var rows *Rows
	var rerr error
	if len(args) == 0 {
		// 无参数
		rows, rerr = dsc.Queryx(stmt)
	} else if emap, ok := args[0].(map[string]any); ok {
		// 命名参数
		rows, rerr = NamedQuery(dsc, stmt, emap)
	} else {
		// 数组参数， 暂时不支持 sql.Named 命名参数
		rows, rerr = dsc.Queryx(stmt, args...)
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

func (r *Repo[T]) SelectAll(dsc DSC) ([]T, error) {
	return r.SelectBy(dsc, r.ColsBy(nil), "")
}

func (r *Repo[T]) Select(dsc DSC, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsBy(nil), cond, args...)
}

func (r *Repo[T]) SelectByExc(dsc DSC, cols []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByExc(cols...), cond, args...)
}

func (r *Repo[T]) SelectByInc(dsc DSC, cols []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByInc(cols...), cond, args...)
}

func (r *Repo[T]) SelectByExf(dsc DSC, flds []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByExf(flds...), cond, args...)
}

func (r *Repo[T]) SelectByInf(dsc DSC, flds []string, cond string, args ...any) ([]T, error) {
	return r.SelectBy(dsc, r.ColsByInf(flds...), cond, args...)
}

// =============================================================================
// Insert

func (r *Repo[T]) InsertBy(dsc DSC, data *T, setid func(int64)) error {
	if data == nil {
		return errors.New("data is nil")
	}
	cols := r.ColsByExc("id")
	stmt, args := cols.InsertArgs(data, true)
	stmt = SQL_INSERT + r.TN(data) + stmt
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	rst, err := dsc.Exec(stmt, args...)
	var iid int64
	if err != nil {
	} else if setid == nil {
	} else if iid, err = rst.LastInsertId(); err != nil {
	} else {
		setid(iid)
	}
	return err
}

func (r *Repo[T]) Insert(dsc DSC, data *T) error {
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

func (r *Repo[T]) UpdateBy(dsc DSC, data *T, cols *Columns, cond string, args ...any) error {
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
	stmt = SQL_UPDATE + r.TN(data) + stmt + SQL_WHERE + cond
	argv = append(argv, args...)
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(argv))
	}
	_, err := dsc.Exec(stmt, argv...)
	return err
}

func (r *Repo[T]) Update(dsc DSC, data *T) error {
	return r.UpdateBy(dsc, data, r.ColsByExc("id", "created", "creater"), "")
}

func (r *Repo[T]) UpdateByExc(dsc DSC, data *T, cols ...string) error {
	keys := cols[:]
	keys = append(keys, "id", "created", "creater")
	return r.UpdateBy(dsc, data, r.ColsByExc(keys...), "")
}

func (r *Repo[T]) UpdateByInc(dsc DSC, data *T, cols ...string) error {
	return r.UpdateBy(dsc, data, r.ColsByInc(cols...), "")
}

func (r *Repo[T]) UpdateByExf(dsc DSC, data *T, flds ...string) error {
	keys := flds[:]
	keys = append(keys, "ID", "Created", "Creater")
	return r.UpdateBy(dsc, data, r.ColsByExf(keys...), "")
}

func (r *Repo[T]) UpdateByInf(dsc DSC, data *T, flds ...string) error {
	return r.UpdateBy(dsc, data, r.ColsByInf(flds...), "")
}

// =============================================================================
// Delete

func (r *Repo[T]) Delete(dsc DSC, data *T) error {
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

func (r *Repo[T]) DeleteBy(dsc DSC, cond string, args ...any) error {
	stmt := SQL_DELETE + SQL_FROM + r.TN(new(T)) + SQL_WHERE + cond
	if C.Sqlx.ShowSQL {
		z.Printf("[_showsql]: %s | %s", stmt, z.ToStr(args))
	}
	_, err := dsc.Exec(stmt, args...)
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
