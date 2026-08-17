// Package logger provides the centralized Zap logger.
// Development: human-readable console. Production: JSON.
// Injected via constructors — never a package-level global in business code.
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a *zap.Logger for the given environment.
func New(env string) (*zap.Logger, error) {
	if env == "production" {
		cfg := zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		return cfg.Build(zap.AddCaller())
	}
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return cfg.Build()
}
