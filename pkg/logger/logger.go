package logger

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	registry = make(map[string]*zap.Logger)
	mu       sync.RWMutex
	logDir   = "./log"
	logConf  LogConf
)

type LogConf struct {
	Path       string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

func GetLogger(serviceName string) *zap.Logger {
	mu.RLock()
	l, ok := registry[serviceName]
	mu.RUnlock()
	if ok {
		return l
	}

	mu.Lock()
	defer mu.Unlock()
	if l, ok := registry[serviceName]; ok {
		return l
	}

	l = newLogger(serviceName)
	registry[serviceName] = l
	return l
}

func NewServiceLogger(serviceName, name string) *zap.Logger {
	return GetLogger(serviceName).Named(name)
}

type LoggerSet struct {
	Activity    *zap.Logger
	User        *zap.Logger
	Post        *zap.Logger
	Feed        *zap.Logger
	Comment     *zap.Logger
	Auditor     *zap.Logger
	Interaction *zap.Logger
	Bff         *zap.Logger
}

func NewLoggerSet() *LoggerSet {
	return &LoggerSet{
		Activity:    GetLogger("activity"),
		User:        GetLogger("user"),
		Post:        GetLogger("post"),
		Feed:        GetLogger("feed"),
		Comment:     GetLogger("comment"),
		Auditor:     GetLogger("auditor"),
		Interaction: GetLogger("interaction"),
		Bff:         GetLogger("bff"),
	}
}

func newLogger(serviceName string) *zap.Logger {
	_ = os.MkdirAll(logDir, 0755)

	writer := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, serviceName+".log"),
		MaxSize:    logConf.MaxSize,
		MaxBackups: logConf.MaxBackups,
		MaxAge:     logConf.MaxAge,
		Compress:   logConf.Compress,
	}

	eConf := zap.NewProductionEncoderConfig()
	eConf.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(eConf),
		zapcore.AddSync(writer),
		zapcore.InfoLevel,
	)

	return zap.New(core)
}

