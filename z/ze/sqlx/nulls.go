package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

func NewNull[T any](value T) sql.Null[T] {
	return sql.Null[T]{V: value, Valid: true}
}

func NewBool(value bool) sql.NullBool {
	return sql.NullBool{Bool: value, Valid: true}
}

func NewByte(value byte) sql.NullByte {
	return sql.NullByte{Byte: value, Valid: true}
}

func NewFloat64(value float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: value, Valid: true}
}

func NewInt16(value int16) sql.NullInt16 {
	return sql.NullInt16{Int16: value, Valid: true}
}

func NewInt32(value int32) sql.NullInt32 {
	return sql.NullInt32{Int32: value, Valid: true}
}

func NewInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: true}
}

func NewString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func NewTime(value time.Time) sql.NullTime {
	return sql.NullTime{Time: value, Valid: true}
}

// ===========================================================

func NewJstr(v map[string]string) NullJstr {
	if v == nil {
		v = make(map[string]string)
	}
	return NullJstr{
		Json:  v,
		Valid: true,
	}
}

// 缓存
type NullJstr struct {
	Json  map[string]string
	Valid bool
}

func (ns NullJstr) Interface() any {
	if !ns.Valid {
		return nil
	}
	return ns.Json
}

// Scan implements the Scanner interface.
func (ns *NullJstr) Scan(value any) error {
	n := sql.NullString{}
	err := n.Scan(value)
	ns.Valid = n.Valid
	if err == nil && ns.Valid && n.String != "" {
		return json.Unmarshal([]byte(n.String), &ns.Json)
	}
	return err
}

// Value implements the driver Valuer interface.
func (ns NullJstr) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	if ns.Json == nil {
		return nil, nil
	}
	return json.Marshal(ns.Json)
}

func (ns NullJstr) Int64(key string) (int64, bool) {
	if str, ok := ns.Json[key]; !ok || str == "" {
		return 0, false
	} else if num, err := strconv.ParseInt(str, 10, 64); err != nil {
		return 0, false
	} else {
		return num, true
	}
}

// ===========================================================

func NewJson(v map[string]any) NullJson {
	if v == nil {
		v = make(map[string]any)
	}
	return NullJson{
		Json:  v,
		Valid: true,
	}
}

// 缓存
type NullJson struct {
	Json  map[string]any
	Valid bool
}

func (ns NullJson) Interface() any {
	if !ns.Valid {
		return nil
	}
	return ns.Json
}

// Scan implements the Scanner interface.
func (ns *NullJson) Scan(value any) error {
	n := sql.NullString{}
	err := n.Scan(value)
	ns.Valid = n.Valid
	if err == nil && ns.Valid && n.String != "" {
		return json.Unmarshal([]byte(n.String), &ns.Json)
	}
	return err
}

// Value implements the driver Valuer interface.
func (ns NullJson) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	if ns.Json == nil {
		return nil, nil
	}
	return json.Marshal(ns.Json)
}

func (ns NullJson) Int64(key string) (int64, bool) {
	if str, ok := ns.Json[key]; !ok || str == "" {
		return 0, false
	} else {
		switch v := str.(type) {
		case string:
			if num, err := strconv.ParseInt(v, 10, 64); err == nil {
				return num, true
			}
		case []byte:
			if num, err := strconv.ParseInt(string(v), 10, 64); err == nil {
				return num, true
			}
		case int:
			return int64(v), true
		case int8:
			return int64(v), true
		case int16:
			return int64(v), true
		case int32:
			return int64(v), true
		case int64:
			return v, true
		case float64:
			return int64(v), true
		case float32:
			return int64(v), true
		default:
			return 0, false
		}
	}
	return 0, false
}

func (ns NullJson) String(key string) (string, bool) {
	if str, ok := ns.Json[key]; !ok {
		return "", false
	} else {
		switch v := str.(type) {
		case string:
			return v, true
		case []byte:
			return string(v), true
		default:
			return fmt.Sprintf("%v", str), true
		}
	}
}

func (ns NullJson) ToJson() (map[string]string, bool) {
	m := make(map[string]string)
	for k := range ns.Json {
		if v, ok := ns.String(k); ok {
			m[k] = v // 可以转换
		}
	}
	return m, true
}
