package distributedid

import (
	"fmt"
	"sync"
	"time"

	"github.com/sony/sonyflake/v2"
)

var (
	flake   *sonyflake.Sonyflake
	once    sync.Once
	initErr error
)

func Init() error {
	once.Do(func() {
		var settings sonyflake.Settings
		settings.StartTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		var err error
		flake, err = sonyflake.New(settings)
		if err != nil {
			initErr = fmt.Errorf("failed to start sonyflake: %w", err)
		}
	})
	return initErr
}

func NextID() (int64, error) {
	if err := Init(); err != nil {
		return 0, err
	}
	if flake == nil {
		return 0, fmt.Errorf("id generator not initialized")
	}
	return flake.NextID()
}
