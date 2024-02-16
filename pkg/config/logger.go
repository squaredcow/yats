package config

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger Creates a new instance of Uber Zap
func NewLogger(zc zapcore.Core) *zap.Logger {
	logger := zap.New(zc, zap.AddCaller())
	return logger
}

// CloseLogger Flushes any pending logs
func CloseLogger(logger *zap.Logger) {
	_ = logger.Sync()
}
