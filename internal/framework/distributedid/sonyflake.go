package distributedid

import (
	"fmt"
	"time"

	"github.com/sony/sonyflake/v2"
)

var (
	flake *sonyflake.Sonyflake
)

func Init() error {

	var settings sonyflake.Settings

	settings.StartTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var err error

	flake, err = sonyflake.New(settings)

	if err != nil {
		return fmt.Errorf("failed to start sonyflake")
	}

	return nil
}

func NextID() (int64, error) {
	if flake == nil {
		return 0, fmt.Errorf("id generator not initialized")
	}
	return flake.NextID()
}
