package domain

import (
	"fmt"
	"strings"
	"time"
)

type ScheduleKind string

const (
	ScheduleOnce     ScheduleKind = "once"
	ScheduleInterval ScheduleKind = "interval"
	ScheduleDaily    ScheduleKind = "daily"
	ScheduleWeekly   ScheduleKind = "weekly"
	ScheduleMonthly  ScheduleKind = "monthly"
)

// Schedule stores a browser-selected IANA timezone, rather than following the
// timezone of whichever browser last opened the task.
type Schedule struct {
	Kind         ScheduleKind `json:"kind"`
	Timezone     string       `json:"timezone"`
	At           time.Time    `json:"at,omitempty"`
	EverySeconds int64        `json:"everySeconds,omitempty"`
	LocalTime    string       `json:"localTime,omitempty"`
	Weekday      int          `json:"weekday,omitempty"`  // Sunday = 0.
	MonthDay     int          `json:"monthDay,omitempty"` // 1–31; missing dates are skipped.
}

func (s Schedule) Validate() error {
	if _, err := time.LoadLocation(strings.TrimSpace(s.Timezone)); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", s.Timezone, err)
	}
	switch s.Kind {
	case ScheduleOnce:
		if s.At.IsZero() {
			return fmt.Errorf("once schedule requires at")
		}
	case ScheduleInterval:
		if s.At.IsZero() || s.EverySeconds < 60 {
			return fmt.Errorf("interval schedule requires at and interval of at least one minute")
		}
	case ScheduleDaily, ScheduleWeekly, ScheduleMonthly:
		if _, err := time.Parse("15:04", s.LocalTime); err != nil {
			return fmt.Errorf("invalid local time %q: %w", s.LocalTime, err)
		}
		if s.Kind == ScheduleWeekly && (s.Weekday < 0 || s.Weekday > 6) {
			return fmt.Errorf("weekday must be 0..6")
		}
		if s.Kind == ScheduleMonthly && (s.MonthDay < 1 || s.MonthDay > 31) {
			return fmt.Errorf("month day must be 1..31")
		}
	default:
		return fmt.Errorf("unsupported schedule kind %q", s.Kind)
	}
	return nil
}

// Next returns the first scheduled instant strictly after from. A repeated
// local time fires at its first occurrence; a missing local time fires at the
// first valid minute after the clock jump on that calendar day.
func (s Schedule) Next(from time.Time) (time.Time, error) {
	if err := s.Validate(); err != nil {
		return time.Time{}, err
	}
	switch s.Kind {
	case ScheduleOnce:
		if s.At.After(from) {
			return s.At, nil
		}
		return time.Time{}, nil
	case ScheduleInterval:
		if s.At.After(from) {
			return s.At, nil
		}
		seconds := int64(from.Sub(s.At)/time.Second)/s.EverySeconds + 1
		return s.At.Add(time.Duration(seconds*s.EverySeconds) * time.Second), nil
	default:
		loc, _ := time.LoadLocation(s.Timezone)
		clock, _ := time.Parse("15:04", s.LocalTime)
		local := from.In(loc)
		for day := 0; day < 370; day++ {
			date := time.Date(local.Year(), local.Month(), local.Day()+day, 12, 0, 0, 0, loc)
			if s.Kind == ScheduleWeekly && int(date.Weekday()) != s.Weekday {
				continue
			}
			if s.Kind == ScheduleMonthly && date.Day() != s.MonthDay {
				continue
			}
			candidate := firstLocalMinute(date, clock.Hour()*60+clock.Minute(), loc)
			if candidate.After(from) {
				return candidate, nil
			}
		}
		return time.Time{}, fmt.Errorf("schedule has no occurrence within one year")
	}
}

func firstLocalMinute(date time.Time, requestedMinute int, loc *time.Location) time.Time {
	// Begin before local midnight so an offset change at midnight is covered.
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc).Add(-3 * time.Hour)
	for instant := start; instant.Before(start.Add(30 * time.Hour)); instant = instant.Add(time.Minute) {
		local := instant.In(loc)
		if local.Year() != date.Year() || local.Month() != date.Month() || local.Day() != date.Day() {
			continue
		}
		if local.Hour()*60+local.Minute() >= requestedMinute {
			return instant
		}
	}
	return time.Time{}
}
