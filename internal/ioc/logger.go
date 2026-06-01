package ioc

import (
	"go.uber.org/zap"

	"github.com/raiki02/EG/pkg/logger"
)

// LoggerSet contains all per-service loggers for wire injection
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

// NewLoggerSet creates a new LoggerSet with all service loggers
func NewLoggerSet() *LoggerSet {
	return &LoggerSet{
		Activity:    logger.GetLogger("activity"),
		User:        logger.GetLogger("user"),
		Post:        logger.GetLogger("post"),
		Feed:        logger.GetLogger("feed"),
		Comment:     logger.GetLogger("comment"),
		Auditor:     logger.GetLogger("auditor"),
		Interaction: logger.GetLogger("interaction"),
		Bff:         logger.GetLogger("bff"),
	}
}