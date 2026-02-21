package sqlx

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ( // separator
	SQL_INSERT = "INSERT INTO "
	SQL_UPDATE = "UPDATE "
	SQL_DELETE = "DELETE "
	SQL_SELECT = "SELECT "
	SQL_FROM   = " FROM "
	SQL_WHERE  = " WHERE "
	SQL_ORDER  = " ORDER BY "
	SQL_GROUP  = " GROUP BY "
	SQL_LIMIT  = " LIMIT "
)

func ExistMap(keys ...string) map[string]int {
	rst := map[string]int{}
	for idx, key := range keys {
		rst[key] = idx
	}
	return rst
}

// ===================================================================

type Column struct {
	CName string // 对应数据库列名
	Field string // 对应结构体字段
	Alias string // 对应结构体字段
	Condi string // 条件condition
	Value any    // 值
}

func (c *Column) VName(alias string) string {
	if c.Field == "" {
		if alias == "" {
			return "_" + c.CName
		}
		return alias + "_" + c.CName
	}
	if alias == "" {
		return "_" + c.Alias
	}
	return alias + "_" + c.Alias

}

func (c *Column) Select(alias string) string {
	if c.Alias == "" {
		if alias == "" {
			return fmt.Sprintf("`%s`", c.CName)
		}
		return fmt.Sprintf("%s.`%s`", alias, c.CName)
	}
	if alias == "" {
		return fmt.Sprintf("`%s` AS %s", c.CName, c.Alias)
	}
	return fmt.Sprintf("%s.`%s` AS %s", alias, c.CName, c.Alias)
}

// 暂不支持三目运算符， 三目运算符可以拆解为二目运算符
// vanme == "", 使用 ? 参数方式，反之使用 Named 参数方式
func (c *Column) Symbol(alias, vname string) (string, string) {
	condi := c.Condi
	if condi == "" {
		condi = "="
	}
	if vname == "" {
		if alias == "" {
			return "", fmt.Sprintf("`%s`%s?", c.CName, condi)
		}
		return "", fmt.Sprintf("%s.`%s`%s?", alias, c.CName, condi)
	}
	if alias == "" {
		return vname, fmt.Sprintf("`%s`%s:%s", c.CName, condi, vname)
	}
	return vname, fmt.Sprintf("%s.`%s`%s:%s", alias, c.CName, condi, vname)
}

// ===================================================================

func NewColumns(alias string, stm *StructMap) *Columns {
	return &Columns{
		As:   alias,
		Stm:  stm,
		Cols: []Column{},
	}
}

type Columns struct {
	As   string
	Stm  *StructMap
	Cols []Column
}

func (s *Columns) SetAs(alias string) *Columns {
	s.As = alias
	return s
}

// where 增加条件
func (s *Columns) Cond(fld, con string, val any) *Columns {
	_ = s.Add("", fld, con, val, false)
	return s
}

// field 允许为空，如果为空， 同 cname; cname 为空，会使用 field 查询
func (s *Columns) Add(cname, field, condi string, value any, cover bool) error {
	ignore := false
	if cname == "" && field != "" && s.Stm != nil {
		if fld := s.Stm.Fields[field]; fld != nil {
			cname = fld.Name
			ignore = true
		}
	}
	if cname == "" {
		return errors.New("cname or field is emtpy")
	} else if ignore || field == "" || s.Stm == nil {
		// pass, 已经验证过了
	} else if fld := s.Stm.Names[cname]; fld == nil {
		return errors.New("field is not found")
	} else if fld.GetFieldName() != field {
		return errors.New("fpath is not match")
	}
	if cover {
		for idx, col := range s.Cols {
			if col.CName == cname {
				s.Cols[idx].Field = field
				s.Cols[idx].Value = value
				s.Cols[idx].Condi = condi
				return nil // 已经存在，直接返回
			}
		}
	}
	// 追加到末尾
	s.Cols = append(s.Cols, Column{CName: cname, Field: field, Condi: condi, Value: value})
	return nil
}

// 直接增加，跳过校验, 一般用于遍历 Field 的场景下
func (s *Columns) Append(col Column) *Columns {
	s.Cols = append(s.Cols, col)
	return s
}

func (s *Columns) DelByCName(cname ...string) *Columns {
	cols := []Column{}
	emap := ExistMap(cname...)
	for _, col := range s.Cols {
		if _, ok := emap[col.CName]; !ok {
			cols = append(cols, col)
		}
	}
	s.Cols = cols
	return s
}

func (s *Columns) DelByField(fields ...string) *Columns {
	cols := []Column{}
	emap := ExistMap(fields...)
	for _, col := range s.Cols {
		if _, ok := emap[col.Field]; !ok {
			cols = append(cols, col)
		}
	}
	s.Cols = cols
	return s
}

func (s *Columns) GetByCName(cname string) *Column {
	for _, col := range s.Cols {
		if col.CName == cname {
			return &col
		}
	}
	return nil
}

func (s *Columns) GetByField(field string) *Column {
	for _, col := range s.Cols {
		if col.Field == field {
			return &col
		}
	}
	return nil
}

func (s *Columns) Select() string {
	sbr := strings.Builder{}
	for _, col := range s.Cols {
		if sbr.Len() > 0 {
			sbr.WriteByte(',')
		}
		sbr.WriteString(col.Select(s.As))
	}
	return sbr.String()
}

// obj == nil 只会处理 cols 中的内容
func (s *Columns) EachArgs(obj any, ign bool, fn func(Column, any)) {
	if obj == nil {
		for _, col := range s.Cols {
			if ign && col.Value == nil {
				continue
			}
			fn(col, col.Value)
		}
	} else {
		ref := reflect.ValueOf(obj)
		if ref.Kind() == reflect.Pointer {
			ref = ref.Elem()
		}
		for _, col := range s.Cols {
			var value any
			if col.Value != nil {
				// col 值优先使用
				value = col.Value
			} else {
				field := s.Stm.Names[col.CName]
				value = ref.FieldByIndex(field.Field.Index).Interface()
				if val, ok := value.(driver.Valuer); ok {
					value, _ = val.Value()
				}
			}
			if ign && value == nil {
				continue
			}
			fn(col, value)
		}
	}

}

// 效率不高，不建议使用，但是推荐在 Create or Update 上使用, sep 默认为 " AND ", update 默认为 ","
func (s *Columns) NamedArgs(obj any, rst map[string]any, ign bool, sep string) (string, map[string]any) {
	sbr := strings.Builder{}
	if rst == nil {
		rst = map[string]any{}
	}
	if sep == "" {
		sep = " AND "
	}
	s.EachArgs(obj, ign, func(col Column, value any) {
		vname, vtext := col.Symbol(s.As, col.VName(s.As))
		rst[vname] = value
		if sbr.Len() > 0 {
			sbr.WriteString(sep)
		}
		sbr.WriteString(vtext)
	})
	return sbr.String(), rst
	// rst = ref.Convert(reflect.TypeOf(rst)).Interface().(map[string]any)
}

func (s *Columns) ArrayArgs(obj any, ign bool, sep string) (string, []any) {
	sbr := strings.Builder{}
	rst := []any{}
	if sep == "" {
		sep = " AND "
	}
	s.EachArgs(obj, ign, func(col Column, value any) {
		_, vtext := col.Symbol(s.As, "")
		rst = append(rst, value)
		if sbr.Len() > 0 {
			sbr.WriteString(sep)
		}
		sbr.WriteString(vtext)
	})
	return sbr.String(), rst
}

func (s *Columns) InsertArgs(obj any, ign bool) (string, []any) {
	sbr := strings.Builder{}
	rsv := strings.Builder{}
	rst := []any{}
	s.EachArgs(obj, ign, func(col Column, value any) {
		rst = append(rst, value)
		if sbr.Len() > 0 {
			sbr.WriteByte(',')
			rsv.WriteByte(',')
		}
		sbr.WriteString(col.CName)
		rsv.WriteByte('?')
	})
	return fmt.Sprintf(" (%s) VALUES (%s)", sbr.String(), rsv.String()), rst
}

func (s *Columns) UpdateArgs(obj any, ign bool) (string, []any) {
	// vsql, args := s.ArrayArgs(obj, ign, ",")
	// return " SET " + vsql, args
	sbr := strings.Builder{}
	rst := []any{}
	s.EachArgs(obj, ign, func(col Column, value any) {
		if col.CName == "id" {
			return // 忽略 id
		}
		_, vtext := col.Symbol(s.As, "")
		rst = append(rst, value)
		if sbr.Len() > 0 {
			sbr.WriteByte(',')
		}
		sbr.WriteString(vtext)
	})
	return " SET " + sbr.String(), rst
}
