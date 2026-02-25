package sqlx

import (
	"errors"
	"fmt"
	"reflect"
)

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

// =============================================================================

type Repo[T any] struct {
	Typ reflect.Type // data type
	Stm *StructMap   // struct map
}

// 忽略 没有被 "db" 标记的属性, 即 Repo 对应的 DO 必须具有 "db" 标签
// var RepoMpr = NewMapperFunc("db", func(s string) string { return "-" })

func (r *Repo[T]) InitRepo() {
	r.Typ = reflect.TypeFor[T]()
	r.Stm = mapper().TypeMap(r.Typ)
	RegKsqlEnt(r.Typ.Name(), TableName(new(T)))
}

// func (r *Repo[T]) Entity() *T { return new(T) }
// func (r *Repo[T]) Arrays() []T { return []T{} }

func (r *Repo[T]) ToMap(obj *T) map[string]any {
	return ToMapBy(r.Stm, obj, false, true)
}

// =============================================================================

func (r *Repo[T]) Cols() *Cols {
	return ColsBy[T](r.Stm, nil)
}

func (r *Repo[T]) ColsByExc(cols ...string) *Cols {
	return ColsBy[T](r.Stm, func(val *FieldInfo) (string, bool) { return val.Name, false }, cols...)
}

func (r *Repo[T]) ColsByInc(cols ...string) *Cols {
	return ColsBy[T](r.Stm, func(val *FieldInfo) (string, bool) { return val.Name, true }, cols...)
}

func (r *Repo[T]) ColsByExf(flds ...string) *Cols {
	return ColsBy[T](r.Stm, func(val *FieldInfo) (string, bool) { return val.GetFieldName(), false }, flds...)
}

func (r *Repo[T]) ColsByInf(flds ...string) *Cols {
	return ColsBy[T](r.Stm, func(val *FieldInfo) (string, bool) { return val.GetFieldName(), true }, flds...)
}

// =============================================================================
// Select

func (r *Repo[T]) Get(dsc Dsc, id int64, flds ...string) (*T, error) {
	return GetBy(dsc, r.ColsByInf(flds...), (*T)(nil), fmt.Sprintf("id=%d", id))
}

func (r *Repo[T]) Getx(dsc Dsc, id int64, data *T, flds ...string) (*T, error) {
	return GetBy(dsc, r.ColsByInf(flds...), data, fmt.Sprintf("id=%d", id))
}

func (r *Repo[T]) SelectAll(dsc Dsc) ([]T, error) {
	return SelectBy[T](dsc, r.Cols(), "")
}

func (r *Repo[T]) Select(dsc Dsc, cond string, args ...any) ([]T, error) {
	return SelectBy[T](dsc, r.Cols(), cond, args...)
}

func (r *Repo[T]) SelectByExc(dsc Dsc, cols []string, cond string, args ...any) ([]T, error) {
	return SelectBy[T](dsc, r.ColsByExc(cols...), cond, args...)
}

func (r *Repo[T]) SelectByInc(dsc Dsc, cols []string, cond string, args ...any) ([]T, error) {
	return SelectBy[T](dsc, r.ColsByInc(cols...), cond, args...)
}

func (r *Repo[T]) SelectByExf(dsc Dsc, flds []string, cond string, args ...any) ([]T, error) {
	return SelectBy[T](dsc, r.ColsByExf(flds...), cond, args...)
}

func (r *Repo[T]) SelectByInf(dsc Dsc, flds []string, cond string, args ...any) ([]T, error) {
	return SelectBy[T](dsc, r.ColsByInf(flds...), cond, args...)
}

// =============================================================================
// Insert

func (r *Repo[T]) Insert(dsc Dsc, data *T) error {
	fid, _ := r.Stm.Names["id"]
	if fid == nil {
		return InsertBy(dsc, r.ColsByExc("id"), data, nil)
	}
	setid := func(id int64) {
		reflect.ValueOf(data).Elem().FieldByIndex(fid.Field.Index).Set(reflect.ValueOf(id))
	}
	return InsertBy(dsc, r.ColsByExc("id"), data, setid)
}

// =============================================================================
// Update

func (r *Repo[T]) Update(dsc Dsc, data *T) error {
	return UpdateBy(dsc, data, r.ColsByExc("id", "created", "creater"), "")
}

func (r *Repo[T]) UpdateByExc(dsc Dsc, data *T, cols ...string) error {
	keys := cols[:]
	keys = append(keys, "id", "created", "creater")
	return UpdateBy(dsc, data, r.ColsByExc(keys...), "")
}

func (r *Repo[T]) UpdateByInc(dsc Dsc, data *T, cols ...string) error {
	return UpdateBy(dsc, data, r.ColsByInc(cols...), "")
}

func (r *Repo[T]) UpdateByExf(dsc Dsc, data *T, flds ...string) error {
	keys := flds[:]
	keys = append(keys, "ID", "Created", "Creater")
	return UpdateBy(dsc, data, r.ColsByExf(keys...), "")
}

func (r *Repo[T]) UpdateByInf(dsc Dsc, data *T, flds ...string) error {
	return UpdateBy(dsc, data, r.ColsByInf(flds...), "")
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
	return DeleteBy[T](dsc, fmt.Sprintf("id=%v", id))
}
