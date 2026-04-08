package log

import (
	logfile "github.com/suisrc/zgg/z/ze/log/file"
	logsyslog "github.com/suisrc/zgg/z/ze/log/syslog"
)

var (
	NewFileWriter   = logfile.NewWriter
	NewSyslogWriter = logsyslog.NewWriter
)
