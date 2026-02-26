# zgg

zgg(z? golang google)  
为简约而生, 核心文件只有3个  

## 框架介绍

这是一极精简的 web 服务框架， 默认没有 routing， 使用 map 进行 action 检索 ，为单接口而生。  
通过 query.action(query参数) 或者 path=action(path就是action) 两种方式确定 handle 函数。  

  
为什么需要它？  
在很多项目中，可能只需要几个接口， 而为这些接口无论使用 gin, echo, iris, fasthttp...我认为都是不值当的。因此它就诞生了。  

  
自动注入wire?  
wire 是一个依赖注入框架， 但是考虑到框架本身就比较小，本身不依赖任何第三方库，所以不会集成wire， 如果需要，可以考虑自行增加。  
但是，实现了一个简单的注入封装，`svckit:"auto"`, 可以自动注入依赖。例如：


## 快速开发

```go
package app

import (
	"github.com/suisrc/zgg/z"
)

func init() {
	z.Register("50-hello", func(zgg *z.Zgg) z.Closed {
		api := z.Inject(zgg.SvcKit, &HelloApi{})
		zgg.AddRouter("hello", api.hello)

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return func() {
			z.Println("api-hello closed")
		}
	})
	z.Register("zz-world", func(zgg *z.Zgg) z.Closed {
		api := zgg.SvcKit.Get("HelloApi").(*HelloApi)
		z.GET("world", api.world, zgg)
		z.GET("token", z.TokenAuth(z.Ptr("123"), api.token), zgg)
		return nil
	})
}

type HelloApi struct {
	FA any                           // 标记不注入，默认
	FB any      `svckit:"-"`         // 标记不注入，默认
	CM z.Module `svckit:"type"`      // 根据【类型】自动注入
	SK z.SvcKit `svckit:"type"`      // 根据【类型】自动注入
	AH any      `svckit:"api-hello"` // 根据【名称】自动注入
	AW any      `svckit:"api-world"` // 根据【名称】自动注入
	TK z.TplKit `svckit:"auto"`      // 根据【类型】自动注入
	tt z.TplKit `svckit:"auto"`      // 私有【属性】不能注入
}

```
  
## 执行命令

```sh
# 命令
xxx [command] [arguments]

xxx web (default)
  -c     string # 配置文件
  -debug bool   # debug mode 
  -local bool   # local mode， addr = 127.0.0.1
  -addr  string # 服务绑定的ip， (default "0.0.0.0")
  -port  int    # 服务绑定的 port，(default 80)
  -crt   string # 服务绑定的 crt file，https 模式
  -key   string # 服务绑定的 key file，https 模式
  -eng   string # 路由引擎， 默认 map， 其他： mux， rdx
  -api   string # 服务绑定的 api root path

xxx version # 查看应用版本

xxx cert # 生成证书

xxx hello # 测试 hello world

xxx -h # 查看帮助(仅限web模式)


# 示例, 默认 web 模式
xxx -debug -local

```

## 项目列表

[zgg](https://github.com/suisrc/zgg.git) 基于 zgg 的工具包
- front2: 前端部署服务
- kwdog2: 鉴权网关服务
- proxy2: 正向代理服务
- kwlog2: 日志存储服务

[k8skit](https://github.com/suisrc/k8skit.git) k8s工具包
- sidecar: k8s 边车注入服务
- front3: 增强前端部署服务
- fluent: 日志收集服务