package domain

import (
	"testing"
	"time"
)

func TestCalendarScheduleUsesSavedTimezone(t *testing.T) {
	s := Schedule{Kind: ScheduleDaily, Timezone: "Asia/Shanghai", LocalTime: "08:00"}
	from := time.Date(2026, 9, 30, 0, 30, 0, 0, time.UTC)
	next, err := s.Next(from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestWeeklyAndMonthlySchedules(t *testing.T) {
	from := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		schedule Schedule
		want     time.Time
	}{
		{Schedule{Kind: ScheduleWeekly, Timezone: "UTC", LocalTime: "09:00", Weekday: 5}, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)},
		{Schedule{Kind: ScheduleMonthly, Timezone: "UTC", LocalTime: "09:00", MonthDay: 1}, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		next, err := tc.schedule.Next(from)
		if err != nil || !next.Equal(tc.want) {
			t.Errorf("%s next = %s, %v; want %s", tc.schedule.Kind, next, err, tc.want)
		}
	}
}

func TestScheduleDSTMissingAndRepeatedLocalTime(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	s := Schedule{Kind: ScheduleDaily, Timezone: loc.String(), LocalTime: "02:30"}
	missing, err := s.Next(time.Date(2026, 3, 8, 0, 0, 0, 0, loc))
	if err != nil || missing.In(loc).Format("2006-01-02 15:04") != "2026-03-08 03:00" {
		t.Fatalf("missing next = %s, %v", missing, err)
	}
	s.LocalTime = "01:30"
	repeated, err := s.Next(time.Date(2026, 11, 1, 0, 0, 0, 0, loc))
	if err != nil || repeated.UTC() != (time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Fatalf("repeated next = %s, %v", repeated, err)
	}
}

func TestIntervalScheduleIsAnchoredAndOnceExpires(t *testing.T) {
	anchor := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	s := Schedule{Kind: ScheduleInterval, Timezone: "UTC", At: anchor, EverySeconds: 3600}
	next, err := s.Next(anchor.Add(2*time.Hour + 10*time.Minute))
	if err != nil || !next.Equal(anchor.Add(3*time.Hour)) {
		t.Fatalf("next = %s, %v", next, err)
	}
	s = Schedule{Kind: ScheduleOnce, Timezone: "UTC", At: anchor}
	next, err = s.Next(anchor)
	if err != nil || !next.IsZero() {
		t.Fatalf("expired once = %s, %v", next, err)
	}
}
