package zc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	Logf = func(format string, v ...any) {
		if C.Logger.File {
			slog.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"), "file", LogTrace(1, -1))
		} else {
			slog.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
		}
	}

	Logn = func(v ...any) {
		if C.Logger.File {
			slog.Info(strings.TrimSuffix(LogSprint(" ", v...), "\n"), "file", LogTrace(1, -1))
		} else {
			slog.Info(strings.TrimSuffix(LogSprint(" ", v...), "\n"))
		}
	}

	Logz = func(depth int, v ...any) {
		if C.Logger.File {
			slog.Info(strings.TrimSuffix(LogSprint(" ", v...), "\n"), "file", LogTrace(depth+1, -1))
		} else {
			slog.Info(strings.TrimSuffix(LogSprint(" ", v...), "\n"))
		}
	}

	Exit = func(v ...any) {
		if C.Logger.File {
			slog.Error(strings.TrimSuffix(LogSprint(" ", v...), "\n"), "file", LogTrace(1, -1))
		} else {
			slog.Error(strings.TrimSuffix(LogSprint(" ", v...), "\n"))
		}
		os.Exit(1) // panic(fmt.Sprint(v...))
	}

	//----------------------------------------------------------------------------------------

	// 控制台日志输出，注意， 在替换时候，需要注意改日志不应该被替换掉
	stdLogger = slog.New(NewLogStdHandler(os.Stdout, nil))
	// bufPool 用于 LogLineHandler 的缓冲区池
	bufPool = sync.Pool{New: func() any { return new([]byte) }}
)

// 基础颜色函数
func LogRed(s string) string    { return "\033[31m" + s + "\033[0m" }
func LogGreen(s string) string  { return "\033[32m" + s + "\033[0m" }
func LogYellow(s string) string { return "\033[33m" + s + "\033[0m" }
func LogBlue(s string) string   { return "\033[34m" + s + "\033[0m" }
func LogPurple(s string) string { return "\033[35m" + s + "\033[0m" }
func LogCyan(s string) string   { return "\033[36m" + s + "\033[0m" }
func LogGray(s string) string   { return "\033[90m" + s + "\033[0m" }

// 带样式的颜色函数
func LogBold(s string) string  { return "\033[1m" + s + "\033[0m" }
func LogUnder(s string) string { return "\033[4m" + s + "\033[0m" }

// 控制台日志输出， 标准日志输出请使用 slog 包
func Stdout() *slog.Logger {
	return stdLogger
}

// 向默认控制台输出
func LogTty(v ...any) {
	if C.Logger.File {
		stdLogger.Info(strings.TrimSuffix(LogSprint(" ", v...), "\n"), "file", LogTrace(1, -1))
	} else {
		stdLogger.Info(strings.TrimSuffix(LogSprint(" ", v...), "\n"))
	}
}

// 向默认控制台输出
func ErrTty(v ...any) {
	if C.Logger.File {
		stdLogger.Error(strings.TrimSuffix(LogSprint(" ", v...), "\n"), "file", LogTrace(1, -1))
	} else {
		stdLogger.Error(strings.TrimSuffix(LogSprint(" ", v...), "\n"))
	}
}

func LogSprint(sep string, v ...any) string {
	buf := strings.Builder{}
	for i, item := range v {
		if i > 0 {
			buf.WriteString(sep)
		}
		fmt.Fprint(&buf, item)
	}
	slog.Default()
	return buf.String()
}

//----------------------------------------------------------------------------------------

func NewLogStdHandler(output io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &logStdHandler{
		output: output,
		option: opts,
	}
}

type logStdHandler struct {
	output io.Writer
	option *slog.HandlerOptions

	attrs  []slog.Attr
	groups []string
}

func (log *logStdHandler) GetBuffer() *[]byte {
	return bufPool.Get().(*[]byte)
}

func (log *logStdHandler) PutBuffer(buf *[]byte) {
	// See https://go.dev/issue/23199
	if cap(*buf) > 64<<10 {
		*buf = nil
	}
	*buf = (*buf)[:0]
	bufPool.Put(buf)
}

func (h *logStdHandler) Enabled(_ context.Context, l slog.Level) bool {
	if h.option == nil || h.option.Level == nil {
		return l >= slog.LevelInfo
	}
	return l >= h.option.Level.Level()
}

// Collect the level, attributes and message in a string and
// write it with the default log.Logger.
// Let the log.Logger handle time and file/line.
func (h *logStdHandler) Handle(ctx context.Context, r slog.Record) error {
	buf := h.GetBuffer()
	defer h.PutBuffer(buf)

	// 时间格式化
	if r.Time.IsZero() {
		r.Time = time.Now()
	}
	LogTimeWith(buf, r.Time)

	// 日志级别
	*buf = append(*buf, ' ', '[', r.Level.String()[0], ']')

	// 扩展字段
	if r.NumAttrs() > 0 {
		*buf = append(*buf, ' ')
		*buf = append(*buf, '[')
		sep := false
		for _, attr := range h.attrs {
			if sep {
				*buf = append(*buf, ' ')
			} else {
				sep = true
			}
			*buf = append(*buf, attr.Key...)
			*buf = append(*buf, '=')
			*buf = append(*buf, fmt.Sprint(attr.Value)...)
		}
		r.Attrs(func(a slog.Attr) bool {
			if sep {
				*buf = append(*buf, ' ')
			} else {
				sep = true
			}
			*buf = append(*buf, a.Key...)
			*buf = append(*buf, '=')
			if a.Key == "file" {
				*buf = append(*buf, '`')
			}
			*buf = append(*buf, fmt.Sprint(a.Value)...)
			return true
		})
		*buf = append(*buf, ']')
	}

	// 消息内容
	*buf = append(*buf, ' ')
	*buf = append(*buf, r.Message...)

	if (*buf)[len(*buf)-1] != '\n' {
		*buf = append(*buf, '\n')
	}

	_, err := h.output.Write(*buf)
	return err
}

func (h *logStdHandler) WithAttrs(as []slog.Attr) slog.Handler {
	handler := *h
	handler.attrs = append(handler.attrs, as...)
	return &handler
}

func (h *logStdHandler) WithGroup(name string) slog.Handler {
	handler := *h
	handler.groups = append(handler.groups, name)
	return &handler
}

//----------------------------------------------------------------------------------------

// LogTrace 获取调用者的文件名和行号字符串
func LogTrace(depth, width int, sep ...byte) string {
	file, line := GetTraceFile(depth + 1)
	if len(sep) == 0 {
		return file + ":" + LogItoa(line, width)
	}
	buf := strings.Builder{}
	if len(sep) > 0 {
		buf.WriteByte(sep[0])
	}
	buf.WriteString(file)
	buf.WriteByte(':')
	buf.WriteString(LogItoa(line, width))
	if len(sep) > 1 {
		buf.WriteByte(sep[1])
	}
	return buf.String()
}

// LogItoa 将整数转换为字符串，宽度不足时左侧补零
func LogItoa(i int, wid int) string {
	// Assemble decimal in reverse order.
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	return string(b[bp:])
}

func LogItoaWith(buf *[]byte, i int, wid int) {
	// Assemble decimal in reverse order.
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	*buf = append(*buf, b[bp:]...)
}

func LogTimeWith(buf *[]byte, t time.Time) {
	// *buf = append(*buf, t.Format("2006-01-02T15:04:05.000Z07:00")...)
	// 使用自定义格式化，减少内存分配，提升性能
	// 年月日
	year, month, day := t.Date()
	LogItoaWith(buf, year, 4)
	*buf = append(*buf, '-')
	LogItoaWith(buf, int(month), 2)
	*buf = append(*buf, '-')
	LogItoaWith(buf, day, 2)
	*buf = append(*buf, 'T')
	// 时分秒
	hour, min, sec := t.Clock()
	LogItoaWith(buf, hour, 2)
	*buf = append(*buf, ':')
	LogItoaWith(buf, min, 2)
	*buf = append(*buf, ':')
	LogItoaWith(buf, sec, 2)
	*buf = append(*buf, '.')
	LogItoaWith(buf, t.Nanosecond()/1e6, 3)
	// 时区
	_, offset := t.Zone()
	if offset == 0 {
		*buf = append(*buf, "+00:00"...)
	} else {
		if offset < 0 {
			*buf = append(*buf, '-')
			offset = -offset
		} else {
			*buf = append(*buf, '+')
		}
		LogItoaWith(buf, offset/3600, 2)
		*buf = append(*buf, ':')
		LogItoaWith(buf, (offset%3600)/60, 2)
	}
}
