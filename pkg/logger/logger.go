package logger

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	registry = make(map[string]*zap.Logger)
	mu       sync.RWMutex
	logDir   = "./log"
)

func InitLogDir(dir string) {
	logDir = dir
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

	filePath := filepath.Join(logDir, serviceName+".log")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return newZapLogger(os.Stdout, serviceName)
	}

	eConf := zap.NewProductionEncoderConfig()
	eConf.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(eConf),
		zapcore.AddSync(file),
		zapcore.InfoLevel,
	)

	return zap.New(core)
}

func newZapLogger(w *os.File, serviceName string) *zap.Logger {
	eConf := zap.NewProductionEncoderConfig()
	eConf.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(eConf),
		zapcore.AddSync(w),
		zapcore.InfoLevel,
	)
	return zap.New(core)
}
