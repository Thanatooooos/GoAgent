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
