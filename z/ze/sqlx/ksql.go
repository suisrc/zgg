/*

 ### KSQL
 1. 整体保留SQL原有的语法不变
 2. {:xxx yyy}, 如果xxx的断言有效，即保留yyy的部分
 3. {::xxx(?:zzz)} / {#xxx(?:zzz)}(已过期), 预处理模块，在执行sql处理前，会替换环境中对xxx的定义, ?:zzz, 如果存在，替换将变为参数替换，
    而非直接替换, 详细说明一下?:zzz情况, 没有zzz, 就会直接替换为xxx的值，如果有zzz, 就会替换为参数名，将参数内容放入到参数列表中。见：TestParser3
 4. {:xxx(=zzz) yyy}, 简单的断言， zzz类型为null, boolean, integer, string, array([1,2,3]), 符号支持=，!=。
    "{:xxx(=zzz)"部分不能出现空白符, 默认断言是xxx不是null，即xxx!=null, 可以存在在 sql 的任意位置，支持嵌套
 5. {:xxx=zzz! "err str"}, 异常断言， 如果xxx和zzz断言成功，将直接抛出"err str"的异常。见：TestParser4
 6. 如果需要复杂的断言，xxx 对应 sqlx.KsqlFilter func(map[string]any)bool 参数完成断言内容, 与 {:filter-xxx} 不同， 前者是 inc 参数中， 后者是全局过滤器中
 7. {::env.xxx}, 获取系统环境变量, 需要 sqlx.RegKsqlEvalue("env", sqlx.KsqlPreExt("sqlx.namex.")) 经编码支持

 ksql 本来是 转为 select 语句设计的，起始模块是 SelectBulk, 不过目前在 Insert, Update, Delete 语句中也通过测试。
*/

package sqlx

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

// KSQL 语句执行函数
// argm = zc.ToMap(any, "db", false)， 可以将任何结构体转换为 map[string]any 形式
// 待优化： 1. ksql 语句没好缓存器， 2. 不提供查询单个对象， 3. select 内容需要一对一指定
// dsc: 数据库链接， ksql: 语句， argm: 参数, size: 是否统计所有行数（只有在 select 语句中有效）
// 这是一个复杂的处理逻辑，简单的调用可以使用 sqlx.KsqlParserSimple 完成
func Ksql[T any](dsc Dsc, ksql string, argm map[string]any, size bool) ([]T, int64, error) {
	if argm == nil {
		argm = make(map[string]any)
	}
	argv := make(map[string]any)
	// --------------------------------------------------------------------------
	// stmt, err := KsqlParserSimple(ksql, argm, argv)
	blk := &SelectBulk{}
	if err := blk.Build(ksql, argm); err != nil {
		return nil, 0, err
	}
	kstm, err := blk.Handle(argm, argv, nil, KSQL_NFN_ERROR)
	// --------------------------------------------------------------------------
	if err != nil {
		return nil, 0, err
	}
	// e := dsc.Ext() // Ext & Exc 来自同一个 DB | TX
	stmt, args, err := bindMap(BindType(dsc.Ext().DriverName()), kstm, argv)
	if err != nil {
		return nil, 0, err
	}
	if zc.HasPrefixFold(stmt, SQL_SELECT) || zc.HasPrefixFold(ksql, "-- query") {
		// select statement ------------------------------------------------
		// 如果使用了 hint 标记， 因为不是 SELECT 开头，所以需要标记 -- query 开头
		var cunt int64 = -1
		if size {
			// 优先统计行数，该语句没有预处理功能，如果需要预处理，可以单独执行
			stmc, err := blk.Count_(stmt)
			if C.Sqlx.ShowSQL {
				z.Printf("[_showsql]: %s | %s", stmc, z.ToStr(args))
			}
			if err != nil {
				return nil, 0, err
			}
			var row *Row
			if ctx := dsc.Ctx(); ctx != nil {
				row = dsc.Exc().QueryRowxContext(ctx, stmc, args...)
			} else {
				row = dsc.Ext().QueryRowx(stmc, args...)
			}
			if err := row.scanAny(&cunt, false); err != nil {
				return nil, 0, err
			}
			if cunt >= 0 && dsc.Offset() >= cunt {
				return []T{}, cunt, nil // 没有数据，直接快速返回即可
			}
		}
		stmt = dsc.Prepro(stmt) // 预处理
		if C.Sqlx.ShowSQL {
			z.Printf("[_showsql]: %s --> %s | %s", kstm, stmt, z.ToStr(args))
		}
		var rows *Rows
		var rerr error
		if ctx := dsc.Ctx(); ctx != nil {
			rows, rerr = dsc.Exc().QueryxContext(ctx, stmt, args...)
		} else {
			rows, rerr = dsc.Ext().Queryx(stmt, args...)
		}
		if rerr != nil {
			return nil, 0, rerr
		}
		defer rows.Close()
		dest := []T{}
		if err := scanAll(rows, &dest, false); err != nil {
			return nil, 0, err
		}
		if cunt < 0 {
			cunt = int64(len(dest))
		}
		return dest, cunt, nil
	} else if zc.HasPrefixFold(stmt, SQL_INSERT) {
		// insert statement ------------------------------------------------
		stmt = dsc.Prepro(stmt)
		if C.Sqlx.ShowSQL {
			z.Printf("[_showsql]: %s --> %s | %s", kstm, stmt, z.ToStr(args))
		}
		var rowi int64
		var rerr error
		if ctx := dsc.Ctx(); ctx != nil {
			res, rerr := dsc.Exc().ExecContext(ctx, stmt, args...)
			if rerr == nil {
				rowi, rerr = res.LastInsertId()
			}
		} else {
			res, rerr := dsc.Ext().Exec(stmt, args...)
			if rerr == nil {
				rowi, rerr = res.LastInsertId()
			}
		}
		return nil, rowi, rerr
	} else {
		// update / delete / unknow statement -----------------------------
		// 如果遇到特殊语句，只是用来查询， 可以在语句开头标记 "-- query" 来规避
		stmt = dsc.Prepro(stmt)
		if C.Sqlx.ShowSQL {
			z.Printf("[_showsql]: %s --> %s | %s", kstm, stmt, z.ToStr(args))
		}
		var rows int64
		var rerr error
		if ctx := dsc.Ctx(); ctx != nil {
			res, rerr := dsc.Exc().ExecContext(ctx, stmt, args...)
			if rerr == nil {
				rows, rerr = res.RowsAffected()
			}
		} else {
			res, rerr := dsc.Ext().Exec(stmt, args...)
			if rerr == nil {
				rows, rerr = res.RowsAffected()
			}
		}
		return nil, rows, rerr
	}
}

// ==========================================================================================
// ==========================================================================================
// ==========================================================================================

func KsqlParser(sql string, inc, out map[string]any, ext KsqlExt, nfn KsqlNfn) (string, error) {
	blk := &SelectBulk{}
	if err := blk.Build(sql, inc); err != nil {
		return "", err
	}
	return blk.Handle(inc, out, ext, nfn)
}

// KsqlParserSimple is a simplified version of Parser
func KsqlParserSimple(sql string, inc, out map[string]any) (string, error) {
	return KsqlParser(sql, inc, out, nil, KSQL_NFN_ERROR)
}

type (
	// KsqlFilter interface for filtering SQL conditions
	KsqlFilter func(key string, inc map[string]any) bool
	// KsqlSupply interface for supplying values
	KsqlSupply func(key string, inc map[string]any) string
	// KsqlFmc interface for formatting parameter values
	KsqlExt func(key string, obj any) any
	// KsqlNfn interface for handling missing parameters
	KsqlNfn func(string) error
)

var (
	KSQL_NFN_ERROR = func(key string) error { return errors.New("missing parameter: " + key) }
	KSQL_NFN_PANIC = func(key string) error { panic("missing parameter: " + key) }
	KSQL_NFN_PRINT = func(key string) error { fmt.Println("missing parameter: ", key); return nil }

	ksqlc = &KsqlCache{
		filters: make(map[string]KsqlFilter),
		evalues: make(map[string]any),
		etables: make(map[string]string),
	}
)

var (

	// 通过系统环境变量获取参数， PS： 该方法默认禁用，需要主动配置启用, 因为获取系统环境变量比较敏感
	// sqlx.RegKsqlEvalue("env", sqlx.KsqlEnv)， 也可以 修改 key = "entity." + key,限制获取范围
	KsqlEnvExt KsqlExt = func(key string, obj any) any { return zc.GetByKey[any](key, nil) }

	// 限制获取配置的前缀
	// sqlx.RegKsqlEvalue("env", sqlx.KsqlEnvPre("sqlx.namex."))
	KsqlPreExt = func(pre string) KsqlExt {
		return func(key string, obj any) any {
			if !strings.HasPrefix(key, pre) {
				return nil
			}
			// z.Println("[__debug_]: KsqlEnv, ", key, zc.GetByKey[any](key, nil))
			return zc.GetByKey[any](key, nil)
		}
	}

	// 在 ksql 文中，使用 {::entity.xxx} 获取对象对应的 TableName，用于动态绑定表名, 需要启动 sqlx.C.Sqlx.KsqlTbl = true
	// sqlx.RegKsqlEvalue("entity", sqlx.KsqlTbl)
	KsqlTblExt KsqlExt = func(key string, obj any) any {
		if tbl := GetKsqlEnt(key); tbl != "" {
			return tbl
		} else {
			return strings.ToLower(key)
		}
	}
)

// ==========================================================================================
// ==========================================================================================
// ==========================================================================================

type KsqlCache struct {
	// filters Mutex protects the filters map
	filterx sync.RWMutex
	filters map[string]KsqlFilter
	// evalues Mutex protects the gvalues map
	evaluex sync.RWMutex
	evalues map[string]any
	// etables Mutex protects the etables map
	etablex sync.RWMutex
	etables map[string]string
}

// GetKsqlFilter retrieves a filter by name
func GetKsqlFilter(name string) KsqlFilter {
	ksqlc.filterx.RLock()
	defer ksqlc.filterx.RUnlock()
	return ksqlc.filters[name]
}

// ClsKsqlFilter clears all filters
func ClsKsqlFilter() {
	ksqlc.filterx.Lock()
	defer ksqlc.filterx.Unlock()
	ksqlc.filters = make(map[string]KsqlFilter)
}

// RegKsqlFilter registers a filter
func RegKsqlFilter(name string, filter KsqlFilter) {
	ksqlc.filterx.Lock()
	defer ksqlc.filterx.Unlock()
	if filter == nil {
		delete(ksqlc.filters, name)
	} else {
		ksqlc.filters[name] = filter
	}
}

//===========================================================================================

// GetKsqlValue retrieves a value from the cache or the input map
func GetKsqlEvalue(key string, inc map[string]any) any {
	// z.Println("[__debug_]: GetKsqlEvalue, ", key)
	var rst string
	if idx := strings.LastIndex(key, "?:"); idx > 0 {
		rst = key[idx+1:]
		key = key[:idx]
	}
	var val any
	if inc != nil {
		val, _ = inc[key] // 优先到 inc 参数中查询
	}
	if val == nil && len(ksqlc.evalues) > 0 {
		// 从全局缓存中查询
		ksqlc.evaluex.RLock()
		defer ksqlc.evaluex.RUnlock()
		val, _ = ksqlc.evalues[key] // 全匹配
		if val == nil {
			// 使用 KsqlExt 扩展查询, xxx.zzz 格式， 其中 xxx 是 Ext 的 Key
			if idx := strings.IndexByte(key, '.'); idx > 0 {
				ext, _ := ksqlc.evalues[key[:idx]]
				if hdl, ok := ext.(KsqlExt); ok {
					val = hdl(key[idx+1:], inc)
				}
			}
		}
	}
	if val != nil {
		if vv, ok := val.(driver.Valuer); ok {
			val, _ = vv.Value()
		}
	}
	if rst == "" {
		return val
	}
	if inc != nil {
		inc[rst[1:]] = val // 赋值给 inc， 用于参数处理
	}
	return rst
}

// ClsKsqlEvalue clears all gvalues
func ClsKsqlEvalue() {
	ksqlc.evaluex.Lock()
	defer ksqlc.evaluex.Unlock()
	ksqlc.evalues = make(map[string]any)
}

// RegKsqlEvalue registers a gvalue， gvalue = KsqlFmt(key, inc), 支持外部扩展
func RegKsqlEvalue(name string, evalue any) {
	ksqlc.evaluex.Lock()
	defer ksqlc.evaluex.Unlock()
	if evalue == nil {
		delete(ksqlc.evalues, name)
	} else {
		ksqlc.evalues[name] = evalue
	}
}

// ==========================================================================================

// 获取 Entity 对应的 TableName
func GetKsqlEnt(typ string) string {
	if ksqlc.etables == nil || !C.Sqlx.KsqlTbl {
		return "" // 已被禁用
	}
	ksqlc.etablex.RLock()
	defer ksqlc.etablex.RUnlock()
	return ksqlc.etables[typ]
}

// 获取 Entity 对应的所有 TableName
func GetKsqlEnts() map[string]string {
	if ksqlc.etables == nil || !C.Sqlx.KsqlTbl {
		return make(map[string]string) // 已被禁用
	}
	ksqlc.etablex.RLock()
	defer ksqlc.etablex.RUnlock()
	rst := make(map[string]string)
	maps.Copy(rst, ksqlc.etables)
	return rst
}

// disable = false, 禁用
func ClsKsqlEnt(disable bool) {
	ksqlc.etablex.Lock()
	defer ksqlc.etablex.Unlock()
	if disable {
		ksqlc.etables = make(map[string]string)
	} else {
		ksqlc.etables = nil
	}
}

// 注册 Entity 对应的 TableName, model.InitRepo 默认会调用该方法
func RegKsqlEnt(typ, tbl string) {
	if ksqlc.etables == nil || !C.Sqlx.KsqlTbl {
		return // 已被禁用
	}
	ksqlc.etablex.Lock()
	defer ksqlc.etablex.Unlock()
	if tbl == "" {
		delete(ksqlc.etables, typ)
	} else {
		ksqlc.etables[typ] = tbl
	}
}

// ==========================================================================================
// ==========================================================================================
// ==========================================================================================

// buildStringBulk creates a StringBulk from SQL substring
func buildStringBulk(sql string, start, end int) (Bulk, error) {
	return &StringBulk{statement: sql[start:end]}, nil
}

// buildArraysBulk creates the appropriate ArraysBulk based on keyword
func buildArraysBulk(keyword string) (Bulks, error) {
	if strings.HasPrefix(keyword, "{:filter-") {
		return NewFilterBulk(keyword[9:])
	}
	return NewParamBulk(keyword[len("{:"):])
}

// Bulk represents a SQL statement block
type Bulk interface {
	// Handle formats the SQL content with given parameters
	Handle(inc, out map[string]any, ext KsqlExt, nfn KsqlNfn) (string, error)
}

// Bulks represents a collection of SQL statement blocks
type Bulks interface {
	Bulk
	Elements() []Bulk
	Build(sql string, start int) (int, error)
}

//===========================================================================================

// SelectBulk represents the main SELECT statement block
type SelectBulk struct {
	elements []Bulk
	countstm string // 统计语句
}

// Build constructs the SELECT bulk from SQL string
func (blk *SelectBulk) Build(osql string, inc map[string]any) error {
	// Remove comments
	nsql := reComment.ReplaceAllString(osql, " \n")
	// 处理 countstr 部分内容 /** count(...) */
	if m := reCountParam.FindStringIndex(osql); m != nil {
		blk.countstm = strings.TrimSpace(nsql[m[0]+3 : m[1]-2])
		nsql = nsql[:m[0]] + nsql[m[1]:]
	}
	// Process {#xxxxx} patterns, 删除该格式
	// nsql = blk.prep_(nsql, inc, rePrepParam2, func(match string) string {
	// 	return match[2 : len(match)-1]
	// })
	// Process {::xxxxx} patterns
	nsql = blk.prep_(nsql, inc, rePrepParam3, func(match string) string {
		return match[3 : len(match)-1]
	})

	offset := 0
	cursor := 0
	for {
		idx := strings.Index(nsql[offset:], "{:")
		if idx < 0 {
			break
		}
		cursor = offset + idx
		if offset < cursor-1 {
			if strblk, err := buildStringBulk(nsql, offset, cursor); err != nil {
				return err
			} else {
				blk.elements = append(blk.elements, strblk)
			}
		}
		offset2 := strings.IndexByte(nsql[cursor:], ' ')
		if offset2 < 0 {
			break
		}
		offset2 = cursor + offset2
		keyword := nsql[cursor:offset2]
		collblk, err := buildArraysBulk(keyword)
		if err != nil {
			return err
		}
		blk.elements = append(blk.elements, collblk)
		offset, err = collblk.Build(nsql, offset2+1)
		if err != nil {
			return err
		}
	}
	if offset < len(nsql) {
		if strblk, err := buildStringBulk(nsql, offset, len(nsql)); err != nil {
			return err
		} else {
			blk.elements = append(blk.elements, strblk)
		}
	}
	return nil
}

// Handle formats the SQL content
func (blk *SelectBulk) Handle(inc, out map[string]any, ext KsqlExt, nfn KsqlNfn) (string, error) {
	var sbir strings.Builder
	for _, elem := range blk.elements {
		if nbk, err := elem.Handle(inc, out, ext, nfn); err != nil {
			return "", err
		} else {
			sbir.WriteString(nbk)
		}
	}
	return removeWhitespace(sbir.String(), false), nil
}

// Count_ 该方法只是为了兼容，建议区别统计 Items 和 Count
// nsql，_ := Handle(inc, out, ext, nfn)
func (blk *SelectBulk) Count_(nsql string) (string, error) {
	cstm := blk.countstm
	if cstm == "" {
		cstm = "count(1)"
	}
	// fidx of FROM
	fidx := strings.Index(strings.ToUpper(nsql), SQL_FROM)
	if fidx < 0 {
		return "", errors.New("not found 'FROM' keyword")
	}
	// 需要追加 /*+ ... */ 内容
	var hint strings.Builder
	if matches := ReKsqlHint.FindAllStringIndex(nsql, -1); len(matches) > 0 {
		for _, m := range matches {
			hint.WriteString(nsql[m[0]:m[1]] + " ")
		}
	}
	return SQL_SELECT + hint.String() + cstm + nsql[fidx:], nil
}

// 预处理模块
func (blk *SelectBulk) prep_(osql string, inc map[string]any, regex *regexp.Regexp, getter func(string) string) string {
	matches := regex.FindAllStringIndex(osql, -1)
	if len(matches) == 0 {
		return removeWhitespace(osql, true)
	}
	var sbir strings.Builder
	offset := 0
	for _, m := range matches {
		sbir.WriteString(osql[offset:m[0]])
		match := osql[m[0]:m[1]]
		key := getter(match)
		val := GetKsqlEvalue(key, inc)
		if val != nil {
			if str, ok := val.(string); ok {
				sbir.WriteString(str)
			} else if su, ok := val.(KsqlSupply); ok {
				str := su(key, inc)
				sbir.WriteString(str)
			}
		}
		offset = m[1]
	}
	sbir.WriteString(osql[offset:])
	return removeWhitespace(sbir.String(), true)
}

// ===========================================================================================

// StringBulk represents a plain string statement block
type StringBulk struct {
	statement string
}

// Handle formats the SQL content
func (blk *StringBulk) Handle(inc, out map[string]any, ext KsqlExt, nfn KsqlNfn) (string, error) {
	matches := reNamedParam.FindAllStringIndex(blk.statement, -1)
	for _, m := range matches {
		name := blk.statement[m[0]+1 : m[1]]
		if _, exists := out[name]; !exists {
			if val, ok := inc[name]; ok {
				if fsu, ok := val.(KsqlSupply); ok {
					val = fsu(name, inc)
				}
				if ext != nil {
					val = ext(name, val)
				}
				if vv, ok := val.(driver.Valuer); ok {
					val, _ = vv.Value()
				}
				out[name] = val
				// if val != nil { out[name] = val } else if nfn != nil {
				// if err := nfn(name); err != nil { return "", err } }
			} else if nfn != nil {
				if err := nfn(name); err != nil {
					return "", err
				}
			}
		}
	}
	return blk.statement + " ", nil
}

func (blk *StringBulk) String() string {
	return blk.statement
}

//===========================================================================================

// ArraysBulk represents a collection of statement blocks
type ArraysBulk struct {
	elements []Bulk
}

// Elements returns the list of bulk elements
func (blk *ArraysBulk) Elements() []Bulk {
	return blk.elements
}

// Build constructs the collection bulk from SQL string
func (blk *ArraysBulk) Build(sql string, start int) (int, error) {
	end := blk.findEndOffset(sql, start)
	end2 := end - 1
	cursor := start
	offset := cursor
	for {
		idx := strings.Index(sql[offset:], "{:")
		if idx < 0 {
			cursor = -1
			break
		}
		cursor = offset + idx
		if cursor >= end2 {
			break
		}
		if offset < cursor-1 {
			if nbk, err := buildStringBulk(sql, offset, cursor); err != nil {
				return -1, err
			} else {
				blk.elements = append(blk.elements, nbk)
			}
		}
		offset2 := strings.IndexByte(sql[cursor:], ' ')
		if offset2 < 0 {
			offset2 = len(sql)
		} else {
			offset2 = cursor + offset2
		}
		keyword := sql[cursor:offset2]
		collblk, err := buildArraysBulk(keyword)
		if err != nil {
			return -1, err
		}
		blk.elements = append(blk.elements, collblk)
		offset, err = collblk.Build(sql, offset2+1)
		if err != nil {
			return -1, err
		}
	}
	if offset < end2 {
		if nbk, err := buildStringBulk(sql, offset, end2); err != nil {
			return -1, err
		} else {
			blk.elements = append(blk.elements, nbk)
		}
	}
	return end, nil
}

// findEndOffset finds the matching closing brace
func (blk *ArraysBulk) findEndOffset(sql string, start int) int {
	offset := start
	count := 1
	for count > 0 && offset < len(sql) {
		ch := sql[offset]
		offset++

		switch ch {
		case '{':
			if offset < len(sql) && sql[offset] == ':' {
				count++
			}
		case '}':
			count--
		}
	}
	return offset
}

//===========================================================================================

// ParamBulk represents a parameter statement block
type ParamBulk struct {
	ArraysBulk
	key         string
	assertor    func(any) bool
	isErrorBulk bool
}

// NewParamBulk creates a new ParamBulk
func NewParamBulk(keyword string) (*ParamBulk, error) {
	if keyword == "" {
		return nil, errors.New("keyword is null or empty")
	}
	blk := &ParamBulk{}
	if strings.HasSuffix(keyword, "!") {
		blk.isErrorBulk = true
		keyword = keyword[:len(keyword)-1]
	}
	if !strings.Contains(keyword, "=") {
		blk.key = keyword
		return blk, nil
	}
	not := false
	spk := "="
	if strings.Contains(keyword, "!=") {
		not = true
		spk = "!="
	}
	parts := strings.SplitN(keyword, spk, 2)
	blk.key = parts[0]
	val := parts[1]
	var err error
	blk.assertor, err = parseAssertor(val, not)
	return blk, err
}

// Handle formats the SQL content
func (blk *ParamBulk) Handle(inc, out map[string]any, ext KsqlExt, nfn KsqlNfn) (string, error) {
	obj := inc[blk.key]
	if blk.ignore(obj, inc) {
		return "", nil
	}
	var sbir strings.Builder
	for _, elem := range blk.Elements() {
		if nbk, err := elem.Handle(inc, out, ext, nfn); err != nil {
			return "", err
		} else {
			sbir.WriteString(nbk)
		}
	}
	if blk.isErrorBulk {
		errstr := strings.TrimSpace(sbir.String())
		if strings.HasPrefix(errstr, "\"") && strings.HasSuffix(errstr, "\"") {
			errstr = errstr[1 : len(errstr)-1]
		}
		return "", errors.New(ReplaceString(errstr, inc))
	}
	return sbir.String(), nil
}

// ignore tests whether data should be ignored
func (blk *ParamBulk) ignore(obj any, inc map[string]any) bool {
	if blk.assertor != nil {
		return !blk.assertor(obj)
	}
	if obj == nil {
		return true
	}
	if pred, ok := obj.(func(map[string]any) bool); ok {
		return !pred(inc)
	}
	if chk, ok := obj.(KsqlFilter); ok {
		return !chk(blk.key, inc)
	}
	if c, ok := obj.(interface{ Len() int }); ok {
		return c.Len() == 0
	}
	if arr, ok := obj.([]any); ok {
		return len(arr) == 0
	}
	return false
}

// ===========================================================================================
// FilterBulk represents a filter statement block
type FilterBulk struct {
	ArraysBulk
	name string
	key  string
}

// NewFilterBulk creates a new FilterBulk
func NewFilterBulk(keyword string) (*FilterBulk, error) {
	blk := &FilterBulk{}
	offset := strings.IndexByte(keyword, '-')
	if offset < 0 {
		blk.name = keyword
		blk.key = ""
	} else {
		blk.name = keyword[:offset]
		blk.key = keyword[offset:]
	}
	return blk, nil
}

// Handle formats the SQL content
func (blk *FilterBulk) Handle(inc, out map[string]any, ext KsqlExt, nfn KsqlNfn) (string, error) {
	filter := GetKsqlFilter(blk.name)
	if filter == nil || !filter(blk.key, inc) {
		return "", nil
	}
	var sbir strings.Builder
	for _, elem := range blk.Elements() {
		if nbk, err := elem.Handle(inc, out, ext, nfn); err != nil {
			return "", nil
		} else {
			sbir.WriteString(nbk)
		}
	}
	return sbir.String(), nil
}

//===========================================================================================
//===========================================================================================
//===========================================================================================
// Helper functions

// ReplaceString formats a string with parameters
func ReplaceString(str string, inc map[string]any) string {
	return rePosParam.ReplaceAllStringFunc(str, func(match string) string {
		key := match[1 : len(match)-1]
		if val, ok := inc[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})
}

var (
	// a regex matcher for parameters (:param)
	reNamedParam = regexp.MustCompile(`:\w+`)
	// a regex matcher for parameters ({param})
	rePosParam = regexp.MustCompile(`\{\w+\}`)
	// a regex matcher for comments
	reComment = regexp.MustCompile(`--.*\n`)
	// a regex matcher for whitespace
	reWhitespace = regexp.MustCompile(`\s{2,}`)
	// a regex matcher for prepared parameters
	rePrepParam3 = regexp.MustCompile(`\{::[\w\.\p{Han}\(\)\,]+(\?:\w+)?\}`) // `\{::[\w\.\u4e00-\u9fa5]+(\?:\w+)?\}`
	// a regex matcher for count bluk, e.g. /** count(...) */
	reCountParam = regexp.MustCompile(`/\*\*\s*count\(.*?\)\s*\*/`)
	// a regex matcher for hint, e.g. /*+ */, 如果起始形式 比如: /*! */需要统一替换 ReKsqlHint 方法
	ReKsqlHint = regexp.MustCompile(`/\*\+.*?\*/`)
)

func removeWhitespace(s string, b bool) string {
	if b {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

// parseAssertor parses the assertion function from the value string
func parseAssertor(val string, not bool) (func(any) bool, error) {
	var base func(any) bool
	if val == "null" {
		base = func(obj any) bool { return obj == nil }
	} else if val == "true" || val == "false" {
		value := val == "true"
		base = func(obj any) bool { return value == obj }
	} else if isInteger(val) {
		value, _ := strconv.Atoi(val)
		base = func(obj any) bool {
			if iv, ok := obj.(int); ok {
				return value == iv
			}
			return false
		}
	} else if isQuotedString(val) {
		value := val[1 : len(val)-1]
		base = func(obj any) bool { return value == obj }
	} else if isArray(val) {
		vals, err := parseArrays(val[1 : len(val)-1])
		if err != nil {
			return nil, err
		}
		base = func(obj any) bool { return slices.Contains(vals, obj) }
	} else {
		// panic(fmt.Sprintf("keyword is not support asser: %s", val))
		return nil, errors.New("keyword is not support asser: " + val)
	}
	if not {
		return func(obj any) bool { return !base(obj) }, nil
	}
	return base, nil
}

func isInteger(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func isQuotedString(s string) bool {
	return len(s) >= 2 && strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")
}

func isArray(s string) bool {
	return len(s) > 2 && strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")
}

func parseArrays(s string) ([]any, error) {
	parts := strings.Split(s, ",")
	vals := make([]any, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "null" {
			vals[i] = nil
		} else if part == "true" || part == "false" {
			vals[i] = part == "true"
		} else if isInteger(part) {
			v, _ := strconv.Atoi(part)
			vals[i] = v
		} else if isQuotedString(part) {
			vals[i] = part[1 : len(part)-1]
		} else {
			// panic(fmt.Sprintf("keyword is not support asser: %s", part))
			return nil, errors.New("keyword is not support asser: " + part)
		}
	}
	return vals, nil
}
