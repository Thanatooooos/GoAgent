package log

import (
	"fmt"

	"go.uber.org/zap"
)

var sugar *zap.SugaredLogger

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
