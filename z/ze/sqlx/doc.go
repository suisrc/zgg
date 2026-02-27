// Package sqlx provides general purpose extensions to database/sql.
//
// It is intended to seamlessly wrap database/sql and provide convenience
// methods which are useful in the development of database driven applications.
// None of the underlying database/sql methods are changed.  Instead all extended
// behavior is implemented through new methods defined on wrapper types.
//
// Additions include scanning into structs, named query support, rebinding
// queries for different drivers, convenient shorthands for common error handling
// and more.

// https://github.com/jmoiron/sqlx

package sqlx

/**
Example:
// 创建数据库 -------------------------------------------------
dsc, err := sqlx.ConnectDB(&C.Database, z.Println)
if err != nil {
	zgg.ServeStop(err.Error())
	return nil
}
z.RegKey(zgg.SvcKit, false, "dsc", dsc)
NewDsc = func() sqlx.Dsc { return &sqlx.Dsx{Ex: dsc} }
if sqlx.C.Sqlx.KsqlDebug {
	ksgr = sqlx.Ksgr(os.DirFS("app/zdb/ksql"), "")
}
// 注册数据仓 -------------------------------------------------
z.RegKey(zgg.SvcKit, false, "", sqlx.NewRepo[AuthzRepo](ksgr))

-----------------------------------------------------------------
//go:embed ksql/*
var ksfs embed.FS
var ksgr = sqlx.Ksgr(ksfs, "ksql/") // if sqlx.C.Sqlx.KsqlDebug { ksgr = sqlx.Ksgr(os.DirFS("ksql"), "") }

rst, siz, err := sqlx.Ksgs("authz_find_all", ksgr, NewDsc(), karg, page)
-----------------------------------------------------------------
*/
