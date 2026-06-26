package app

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const EnvLogLevel = "PROXYGATEWAY_LOG_LEVEL"

var noopLogger = zap.NewNop()

func ParseLogLevel(value string) (zapcore.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return zapcore.InfoLevel, true
	case "debug":
		return zapcore.DebugLevel, true
	case "info":
		return zapcore.InfoLevel, true
	case "warn", "warning":
		return zapcore.WarnLevel, true
	case "error":
		return zapcore.ErrorLevel, true
	default:
		return zapcore.InfoLevel, false
	}
}

func NewProcessLoggerFromEnv() (*zap.Logger, error) {
	rawLevel := os.Getenv(EnvLogLevel)
	level, ok := ParseLogLevel(rawLevel)
	logger, err := NewConsoleLogger(level)
	if err != nil {
		return nil, err
	}
	if !ok {
		logger.Warn("invalid log level; falling back to info",
			zap.String("env", EnvLogLevel),
			zap.String("value", strings.TrimSpace(rawLevel)),
			zap.String("default", "info"),
		)
	}
	return logger, nil
}

func NewConsoleLogger(level zapcore.Level) (*zap.Logger, error) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "ts"
	encoderConfig.LevelKey = "level"
	encoderConfig.NameKey = "logger"
	encoderConfig.CallerKey = "caller"
	encoderConfig.MessageKey = "msg"
	encoderConfig.StacktraceKey = "stacktrace"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	config := zap.Config{
		Level:             zap.NewAtomicLevelAt(level),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: true,
		Encoding:          "console",
		EncoderConfig:     encoderConfig,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	return config.Build()
}

func ensureLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return noopLogger
	}
	return logger
}

func (g *Gateway) log() *zap.Logger {
	if g == nil {
		return noopLogger
	}
	return ensureLogger(g.logger)
}
