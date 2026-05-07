package convention

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// TimePointer 返回时间指针，零值时间返回 nil。
func TimePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

// ParsePositiveInt 解析查询参数为正整数，无效时返回默认值。
func ParsePositiveInt(value string, defaultValue int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return defaultValue
	}
	return n
}

// ParseBool 解析查询参数为布尔值，无效时返回默认值。
func ParseBool(value string, defaultValue bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return defaultValue
	}
}

// CalcPages 根据 total 和 pageSize 计算总页数。
func CalcPages(total int, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
