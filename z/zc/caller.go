package zc

import (
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

var (
	rePtrCaller = regexp.MustCompile(`^\(\*(.*)\)\.(.*)$`)
	reValCaller = regexp.MustCompile(`^(.*)\.(.*)$`)
)

// MethodInfo 存储方法信息
type MethodInfo struct {
	PackageName string // 包名
	StructName  string // 结构体名（如果是方法）
	MethodName  string // 方法名或函数名
	IsPointer   bool   // 是否是指针方法
	FilePath    string // 文件路径
	FileName    string // 文件名
	FileLine    int    // 行号
}

// GetCurrentMethodInfo 获取当前方法的信息
func GetCurrentMethodInfo() *MethodInfo {
	return GetCallerMethodInfo(2)
}

// GetCallerMethodInfo 获取调用者的方法信息
func GetCallerMethodInfo(idx int) *MethodInfo {
	if idx < 1 {
		idx = 1
	}
	pc, file, line, ok := runtime.Caller(idx)
	if !ok {
		return nil
	}
	finfo := runtime.FuncForPC(pc).Name()
	minfo := ParseMethodInfo(finfo)
	minfo.FileLine = line
	minfo.FilePath = file
	if slash := strings.LastIndex(file, "/"); slash >= 0 {
		file = file[slash+1:]
	}
	minfo.FileName = file
	return minfo
}

// ParseMethodInfo 解析完整函数名
func ParseMethodInfo(finfo string) *MethodInfo {
	info := &MethodInfo{}
	// 分割包名和函数名
	parts := strings.Split(finfo, ".")
	if len(parts) < 2 {
		info.MethodName = finfo
		return info
	}
	info.PackageName = parts[0]
	rest := strings.Join(parts[1:], ".")
	// 匹配指针方法：(*User).GetName
	matches := rePtrCaller.FindStringSubmatch(rest)
	if len(matches) == 3 {
		info.StructName = matches[1]
		info.MethodName = matches[2]
		info.IsPointer = true
		return info
	}
	// 匹配值方法：User.GetName
	matches = reValCaller.FindStringSubmatch(rest)
	if len(matches) == 3 {
		info.StructName = matches[1]
		info.MethodName = matches[2]
		info.IsPointer = false
		return info
	}
	// 普通函数
	info.MethodName = rest
	return info
}

func GetFuncInfo(obj any) string {
	if obj == nil {
		return "<nil>"
	}
	fnValue := reflect.ValueOf(obj)
	pc := fnValue.Pointer()
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "<nfn>"
	}
	fnName := fn.Name()
	if idx := strings.LastIndexByte(fnName, '/'); idx > 0 {
		fnName = fnName[idx+1:]
	}
	return fnName
}
