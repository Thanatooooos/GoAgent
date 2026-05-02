package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const cronSearchHorizon = 5 * 366 * 24 * time.Hour

type cronSchedule struct {
	seconds   cronField
	minutes   cronField
	hours     cronField
	monthDays cronField
	months    cronField
	weekDays  cronField
}

type cronField struct {
	allowed    []bool
	noSpecific bool
}

func NextRunTime(cron string, from time.Time) (*time.Time, error) {
	if strings.TrimSpace(cron) == "" || from.IsZero() {
		return nil, nil
	}

	schedule, err := parseCronSchedule(cron)
	if err != nil {
		return nil, err
	}
	return schedule.nextAfter(from), nil
}

func IsIntervalLessThan(cron string, from time.Time, minSeconds int64) (bool, error) {
	if strings.TrimSpace(cron) == "" || from.IsZero() {
		return true, nil
	}

	schedule, err := parseCronSchedule(cron)
	if err != nil {
		return true, err
	}

	first := schedule.nextAfter(from)
	if first == nil {
		return true, nil
	}
	second := schedule.nextAfter(*first)
	if second == nil {
		return true, nil
	}

	return int64(second.Sub(*first).Seconds()) < minSeconds, nil
}

func parseCronSchedule(expression string) (cronSchedule, error) {
	normalized := normalizeCronMacro(strings.TrimSpace(expression))
	fields := strings.Fields(normalized)
	if len(fields) == 5 {
		fields = append([]string{"0"}, fields...)
	}
	if len(fields) != 6 {
		return cronSchedule{}, fmt.Errorf("cron expression must contain 5 or 6 fields")
	}

	seconds, err := parseCronField(fields[0], 0, 59, nil, false, false)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("parse cron seconds: %w", err)
	}
	minutes, err := parseCronField(fields[1], 0, 59, nil, false, false)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("parse cron minutes: %w", err)
	}
	hours, err := parseCronField(fields[2], 0, 23, nil, false, false)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("parse cron hours: %w", err)
	}
	monthDays, err := parseCronField(fields[3], 1, 31, nil, false, true)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("parse cron month days: %w", err)
	}
	months, err := parseCronField(fields[4], 1, 12, cronMonthAliases(), false, false)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("parse cron months: %w", err)
	}
	weekDays, err := parseCronField(fields[5], 0, 6, cronWeekdayAliases(), true, true)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("parse cron week days: %w", err)
	}

	return cronSchedule{
		seconds:   seconds,
		minutes:   minutes,
		hours:     hours,
		monthDays: monthDays,
		months:    months,
		weekDays:  weekDays,
	}, nil
}

func (s cronSchedule) nextAfter(from time.Time) *time.Time {
	candidate := from.Truncate(time.Second).Add(time.Second)
	deadline := candidate.Add(cronSearchHorizon)

	for !candidate.After(deadline) {
		month := int(candidate.Month())
		if !s.months.matches(month) {
			nextMonth, ok := s.months.nextAllowed(month)
			if ok {
				candidate = time.Date(candidate.Year(), time.Month(nextMonth), 1, 0, 0, 0, 0, candidate.Location())
			} else {
				candidate = time.Date(candidate.Year()+1, time.January, 1, 0, 0, 0, 0, candidate.Location())
			}
			continue
		}

		if !s.matchesDay(candidate) {
			candidate = startOfNextDay(candidate)
			continue
		}

		hour := candidate.Hour()
		nextHour, ok := s.hours.nextAllowed(hour)
		if !ok {
			candidate = startOfNextDay(candidate)
			continue
		}
		if nextHour != hour {
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), nextHour, 0, 0, 0, candidate.Location())
			continue
		}

		minute := candidate.Minute()
		nextMinute, ok := s.minutes.nextAllowed(minute)
		if !ok {
			candidate = candidate.Add(time.Hour).Truncate(time.Hour)
			continue
		}
		if nextMinute != minute {
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), candidate.Hour(), nextMinute, 0, 0, candidate.Location())
			continue
		}

		second := candidate.Second()
		nextSecond, ok := s.seconds.nextAllowed(second)
		if !ok {
			candidate = candidate.Add(time.Minute).Truncate(time.Minute)
			continue
		}
		if nextSecond != second {
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), candidate.Hour(), candidate.Minute(), nextSecond, 0, candidate.Location())
			continue
		}

		return &candidate
	}

	return nil
}

func (s cronSchedule) matchesDay(t time.Time) bool {
	monthDayMatches := s.monthDays.matches(t.Day())
	weekDayMatches := s.weekDays.matches(int(t.Weekday()))

	if s.monthDays.noSpecific && s.weekDays.noSpecific {
		return true
	}
	if s.monthDays.noSpecific {
		return weekDayMatches
	}
	if s.weekDays.noSpecific {
		return monthDayMatches
	}
	return monthDayMatches && weekDayMatches
}

func parseCronField(raw string, minValue int, maxValue int, aliases map[string]int, weekDay bool, allowQuestion bool) (cronField, error) {
	field := strings.TrimSpace(raw)
	if field == "" {
		return cronField{}, fmt.Errorf("empty field")
	}
	if strings.ContainsAny(field, "LW#") {
		return cronField{}, fmt.Errorf("unsupported cron field modifier %q", field)
	}

	allowed := make([]bool, maxValue+1)
	noSpecific := false
	for _, item := range strings.Split(field, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return cronField{}, fmt.Errorf("empty list item")
		}
		if item == "?" {
			if !allowQuestion {
				return cronField{}, fmt.Errorf("question mark is not allowed")
			}
			noSpecific = true
			fillCronRange(allowed, minValue, maxValue, 1)
			continue
		}

		start, end, step, err := parseCronFieldItem(item, minValue, maxValue, aliases, weekDay, allowQuestion)
		if err != nil {
			return cronField{}, err
		}
		fillCronRange(allowed, start, end, step)
	}

	return cronField{allowed: allowed, noSpecific: noSpecific}, nil
}

func parseCronFieldItem(item string, minValue int, maxValue int, aliases map[string]int, weekDay bool, allowQuestion bool) (int, int, int, error) {
	base := item
	step := 1
	if strings.Contains(item, "/") {
		parts := strings.Split(item, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return 0, 0, 0, fmt.Errorf("invalid step expression %q", item)
		}
		parsedStep, err := strconv.Atoi(parts[1])
		if err != nil || parsedStep <= 0 {
			return 0, 0, 0, fmt.Errorf("invalid step value %q", parts[1])
		}
		base = strings.TrimSpace(parts[0])
		step = parsedStep
	}

	if base == "*" || base == "?" {
		if base == "?" && !allowQuestion {
			return 0, 0, 0, fmt.Errorf("question mark is not allowed")
		}
		return minValue, maxValue, step, nil
	}

	if strings.Contains(base, "-") {
		parts := strings.Split(base, "-")
		if len(parts) != 2 {
			return 0, 0, 0, fmt.Errorf("invalid range expression %q", item)
		}
		start, err := parseCronValue(parts[0], minValue, maxValue, aliases, weekDay)
		if err != nil {
			return 0, 0, 0, err
		}
		end, err := parseCronValue(parts[1], minValue, maxValue, aliases, weekDay)
		if err != nil {
			return 0, 0, 0, err
		}
		if start > end {
			return 0, 0, 0, fmt.Errorf("range start is greater than end in %q", item)
		}
		return start, end, step, nil
	}

	start, err := parseCronValue(base, minValue, maxValue, aliases, weekDay)
	if err != nil {
		return 0, 0, 0, err
	}
	if step > 1 {
		return start, maxValue, step, nil
	}
	return start, start, step, nil
}

func parseCronValue(raw string, minValue int, maxValue int, aliases map[string]int, weekDay bool) (int, error) {
	valueText := strings.ToUpper(strings.TrimSpace(raw))
	if valueText == "" {
		return 0, fmt.Errorf("empty value")
	}

	if aliases != nil {
		if value, ok := aliases[valueText]; ok {
			return value, nil
		}
		if len(valueText) > 3 {
			if value, ok := aliases[valueText[:3]]; ok {
				return value, nil
			}
		}
	}

	value, err := strconv.Atoi(valueText)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", raw)
	}
	if weekDay && value == 7 {
		return 0, nil
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("value %d outside range %d-%d", value, minValue, maxValue)
	}
	return value, nil
}

func (f cronField) matches(value int) bool {
	return value >= 0 && value < len(f.allowed) && f.allowed[value]
}

func (f cronField) nextAllowed(current int) (int, bool) {
	if current < 0 {
		current = 0
	}
	for value := current; value < len(f.allowed); value++ {
		if f.allowed[value] {
			return value, true
		}
	}
	return 0, false
}

func fillCronRange(allowed []bool, start int, end int, step int) {
	for value := start; value <= end && value < len(allowed); value += step {
		if value >= 0 {
			allowed[value] = true
		}
	}
}

func startOfNextDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
}

func normalizeCronMacro(expression string) string {
	switch strings.ToLower(expression) {
	case "@yearly", "@annually":
		return "0 0 0 1 1 *"
	case "@monthly":
		return "0 0 0 1 * *"
	case "@weekly":
		return "0 0 0 * * 0"
	case "@daily", "@midnight":
		return "0 0 0 * * *"
	case "@hourly":
		return "0 0 * * * *"
	default:
		return expression
	}
}

func cronMonthAliases() map[string]int {
	return map[string]int{
		"JAN": 1,
		"FEB": 2,
		"MAR": 3,
		"APR": 4,
		"MAY": 5,
		"JUN": 6,
		"JUL": 7,
		"AUG": 8,
		"SEP": 9,
		"OCT": 10,
		"NOV": 11,
		"DEC": 12,
	}
}

func cronWeekdayAliases() map[string]int {
	return map[string]int{
		"SUN": 0,
		"MON": 1,
		"TUE": 2,
		"WED": 3,
		"THU": 4,
		"FRI": 5,
		"SAT": 6,
	}
}
