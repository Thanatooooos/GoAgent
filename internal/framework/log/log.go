package log

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

type contextKey struct{}

var (
	sugar         *zap.SugaredLogger
	fallbackSugar = zap.NewNop().Sugar()
)

// Init 初始化全局 logger，若失败回退到 fmt
func Init() error {
	l, err := zap.NewProduction()
	if err != nil {
		// fallback: leave sugar nil and rely on fmt
		return err
	}
	sugar = l.Sugar()
	return nil
}

func Warnf(format string, args ...interface{}) {
	if sugar != nil {
		sugar.Warnf(format, args...)
		return
	}
	fmt.Printf(format+"\n", args...)
}

func Infof(format string, args ...interface{}) {
	if sugar != nil {
		sugar.Infof(format, args...)
		return
	}
	fmt.Printf(format+"\n", args...)
}

func Errorf(format string, args ...interface{}) {
	if sugar != nil {
		sugar.Errorf(format, args...)
		return
	}
	fmt.Printf(format+"\n", args...)
}

// Infow 输出结构化 info 级别日志。
func Infow(msg string, keysAndValues ...interface{}) {
	if sugar != nil {
		sugar.Infow(msg, keysAndValues...)
		return
	}
	fmt.Printf("%s %v\n", msg, keysAndValues)
}

// Warnw 输出结构化 warn 级别日志。
func Warnw(msg string, keysAndValues ...interface{}) {
	if sugar != nil {
		sugar.Warnw(msg, keysAndValues...)
		return
	}
	fmt.Printf("%s %v\n", msg, keysAndValues)
}

// Errorw 输出结构化 error 级别日志。
func Errorw(msg string, keysAndValues ...interface{}) {
	if sugar != nil {
		sugar.Errorw(msg, keysAndValues...)
		return
	}
	fmt.Printf("%s %v\n", msg, keysAndValues)
}

// FromContext returns a logger bound to ctx, or the global logger when none is set.
func FromContext(ctx context.Context) *zap.SugaredLogger {
	if ctx != nil {
		if logger, ok := ctx.Value(contextKey{}).(*zap.SugaredLogger); ok && logger != nil {
			return logger
		}
	}
	if sugar != nil {
		return sugar
	}
	return fallbackSugar
}

// BindLogger stores the provided logger on ctx.
func BindLogger(ctx context.Context, logger *zap.SugaredLogger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, logger)
}

// NewContext stores a child logger with extra fields on ctx.
func NewContext(ctx context.Context, fields ...interface{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, FromContext(ctx).With(fields...))
}

// WithFields returns a logger with extra fields using the global fallback chain.
func WithFields(fields ...interface{}) *zap.SugaredLogger {
	return FromContext(nil).With(fields...)
}
