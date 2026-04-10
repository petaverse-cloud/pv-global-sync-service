package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.SugaredLogger
type Logger struct {
	*zap.SugaredLogger
}

// New creates a new logger
func New(level, format string) (*Logger, error) {
	var config zap.Config

	if format == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	zapLogger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{SugaredLogger: zapLogger.Sugar()}, nil
}

// Field is an alias for zap.Field
type Field = zapcore.Field

// String creates a string field
func String(key, value string) Field {
	return zap.String(key, value)
}

// Int creates an int field
func Int(key string, value int) Field {
	return zap.Int(key, value)
}

// Int64 creates an int64 field
func Int64(key string, value int64) Field {
	return zap.Int64(key, value)
}

// Error creates an error field
func Error(err error) Field {
	return zap.Error(err)
}

// Bool creates a bool field
func Bool(key string, value bool) Field {
	return zap.Bool(key, value)
}

// Any creates an any field
func Any(key string, value interface{}) Field {
	return zap.Any(key, value)
}
