package sqlx_test

import (
	"fmt"
	"testing"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/z/ze/sqlx"
)

// go test -v z/ze/sqlx/ksql_test.go -run TestParser1
func TestParser1(t *testing.T) {
	// Register a filter
	sqlx.RegKsqlFilter("NOTBLANK", (&notBlankFilter{}).Filter)

	sql := `select * from table_name tbl where 1=1
{:filter-NOTBLANK-id 
	{:id  and tbl.id=:id } 
}
{:name and tbl.name=:name }
{:tst1 and tbl.name= "x" }
{:tst2 and tbl.name= "z" }
`

	inc := map[string]any{
		"id":   123,
		"name": "cccc",
		"test": "tttt",
		"tst1": sqlx.KsqlFilter(func(key string, inc map[string]any) bool { return true }),
		"tst2": func(inc map[string]any) bool { return true },
	}
	out := make(map[string]any)

	stm, err := sqlx.KsqlParserSimple(sql, inc, out)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Result:", stm)
		fmt.Println("Params:", z.ToStr2(out))
	}
}

type notBlankFilter struct{}

func (nbf *notBlankFilter) Filter(key string, inc map[string]any) bool {
	if val, ok := inc["id"]; ok {
		return val != nil && val != ""
	}
	return false
}

// go test -v z/ze/sqlx/ksql_test.go -run TestParser2
func TestParser2(t *testing.T) {
	// Register a filter
	sqlx.RegKsqlFilter("NOTBLANK", (&notBlankFilter{}).Filter)

	sql := `select * from table_name tbl where 1=1
{:filter-NOTBLANK-id 
	{:id  and tbl.id=:id } 
}
{:name and tbl.name=:name1 }
`

	inc := map[string]any{
		"id":   123,
		"name": "cccc",
		"test": "tttt",
	}
	out := make(map[string]any)

	stm, err := sqlx.KsqlParserSimple(sql, inc, out)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Result:", stm)
		fmt.Println("Params:", z.ToStr2(out))
	}
}

// go test -v z/ze/sqlx/ksql_test.go -run TestParser3
func TestParser3(t *testing.T) {

	sqlx.RegKsqlEvalue("kusr", sqlx.KsqlExt(func(key string, obj any) any {
		z.Println("[__debug_]: KsqlExt, ", key)
		if key == "tenantCode.substring(0,3)" {
			return "租户编码"
		}
		return nil
	}))

	sql := `SELECT m0.id AS id
FROM
	{::env.namex.prefix}affiliates_{::kind} t1
	JOIN {::entity.AffiliatesMasterDO} m0 ON t1.id = m0.id
WHERE 1=1
{:tenantCode AND m0.ten_code = {::kusr.tenantCode.substring(0,3)?:tenantCode}}
{:code!=null AND t1.code = :code}
{:code1=null AND t1.code = :code}
`

	inc := map[string]any{
		"id":   123,
		"name": "cccc",
		"code": "tttt",
		"kind": "customer",
	}
	out := make(map[string]any)

	stm, err := sqlx.KsqlParser(sql, inc, out, nil, sqlx.KSQL_NFN_PRINT)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Result:", stm)
		fmt.Println("Params:", z.ToStr2(out))
	}
}

// go test -v z/ze/sqlx/ksql_test.go -run TestParser4
func TestParser4(t *testing.T) {
	// register a evalue

	sql := `SELECT m0.id AS id
FROM
	{:kind="customer" {::env.namex.prefix}affiliates_customer t1 }
	{:kind="executor" {::env.namex.prefix}affiliates_executor t1 }
	{:kind="provider" {::env.namex.prefix}affiliates_provider t1 }
	{:kind="reseller" {::env.namex.prefix}affiliates_reseller t1 }
	{:kind!=["customer","executor","provider","reseller"]! "bad params, kind is underfind: :kind" }
	JOIN {::entity.AffiliatesMasterDO} m0 ON t1.id = m0.id
WHERE m0.ten_code = :tenCode AND t1.code = :code
`

	inc := map[string]any{
		"id":   123,
		"name": "cccc",
		"code": "tttt",
		"kind": "customer",
	}
	out := make(map[string]any)

	stm, err := sqlx.KsqlParser(sql, inc, out, nil, sqlx.KSQL_NFN_PRINT)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Result:", stm)
		fmt.Println("Params:", z.ToStr2(out))
	}
}

// authz data object
type AffiliatesMasterDO struct {
	ID int64 `db:"id"`
}

func (AffiliatesMasterDO) TableName() string {
	// return "affiliates_master_def"
	return sqlx.GetTableByEnv("AffiliatesMasterDO", "affiliates_master_def")
}

type AffiliatesMasterRepo struct {
	sqlx.Repo[AffiliatesMasterDO]
}

// go test -v z/ze/sqlx/ksql_test.go -run TestParser5
func TestParser5(t *testing.T) {
	// 该配置是框架构建时候配置的，因此需要硬编码代码在项目中
	sqlx.RegKsqlEvalue("entity", sqlx.KsqlTblExt)
	sqlx.RegKsqlEvalue("env", sqlx.KsqlPreExt("sqlx.namex."))

	// 模拟配置文件配置
	sqlx.C.Sqlx.TblName.Prefix = "sqlx_"
	// sqlx.C.Sqlx.TblName.Mapping = map[string]string{ "AffiliatesMasterDO": "affiliates_master"}
	sqlx.C.Sqlx.KsqlTbl = true
	zc.C.Cache = true
	zc.C.Print = true
	zc.LoadConfig("") // 加载配置，激活环境

	_ = sqlx.NewRepo[AffiliatesMasterRepo](nil) // 注册仓库
	z.Println(z.ToStr(sqlx.GetKsqlEnts()))
	// z.Println(z.ToStr(zc.GetByPre("")))

	sql := `SELECT m0.id AS id
FROM
	{:kind="customer" {::env.sqlx.namex.prefix}affiliates_customer t1 }
	{:kind="executor" {::env.sqlx.namex.prefix}affiliates_executor t1 }
	{:kind="provider" {::env.sqlx.namex.prefix}affiliates_provider t1 }
	{:kind="reseller" {::env.sqlx.namex.prefix}affiliates_reseller t1 }
	{:kind!=["customer","executor","provider","reseller"]! "bad params, kind is underfind: :kind" }
	JOIN {::entity.AffiliatesMasterDO} m0 ON t1.id = m0.id
WHERE m0.ten_code = :tenCode AND t1.code = :code
`

	inc := map[string]any{
		"id":   123,
		"name": "cccc",
		"code": "tttt",
		"kind": "customer",
	}
	out := make(map[string]any)

	stm, err := sqlx.KsqlParser(sql, inc, out, nil, sqlx.KSQL_NFN_PRINT)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Result:", stm)
		fmt.Println("Params:", z.ToStr2(out))
	}
}
