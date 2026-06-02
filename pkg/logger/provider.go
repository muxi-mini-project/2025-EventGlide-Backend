package logger

import "github.com/google/wire"

var Provider = wire.NewSet(
	NewLoggerSet,
)
