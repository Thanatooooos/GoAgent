package rocketmq

import (
	"sync"

	"github.com/apache/rocketmq-client-go/v2/rlog"
)

var configureLoggerOnce sync.Once

func configureClientLogger() {
	configureLoggerOnce.Do(func() {
		rlog.SetLogLevel("warn")
	})
}
