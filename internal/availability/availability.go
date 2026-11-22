// Package availability works out when a member of staff can start a
// treatment: their working hours (Timely's "Normal working hours"), minus
// the busy times their calendar shows, on Timely's online booking grid. It
// is pure: it reads no clock and calls nothing.
package availability

import (
	"fmt"
	"slices"
	"time"
)

// Clock is a time of day in minutes after midnight, 00:00 to 24:00.
type Clock int

const endOfDay Clock = 24 * 60

// ParseClock reads "HH:MM", 00:00 to 24:00.
func ParseClock(s string) (Clock, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	h, okH := twoDigits(s[0:2])
	m, okM := twoDigits(s[3:5])
	c := Clock(h*60 + m)
	if !okH || !okM || m > 59 || c > endOfDay {
		return 0, false
	}
	return c, true
}

func twoDigits(s string) (int, bool) {
	if s[0] < '0' || s[0] > '9' || s[1] < '0' || s[1] > '9' {
		return 0, false
	}
	return int(s[0]-'0')*10 + int(s[1]-'0'), true
}

func (c Clock) String() string { return fmt.Sprintf("%02d:%02d", c/60, c%60) }

// Shift is one stretch of working time on a day, From up to To.
type Shift struct{ From, To Clock }

// Week is working hours by weekday, indexed by time.Weekday (Sunday is 0).
type Week [7][]Shift

// Date is a calendar day at the salon.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf is the day t falls on in loc.
func DateOf(t time.Time, loc *time.Location) Date {
	y, m, d := t.In(loc).Date()
	return Date{y, m, d}
}

// ParseDate reads "YYYY-MM-DD" and rejects impossible days.
func ParseDate(s string) (Date, bool) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, false
	}
	return Date{t.Year(), t.Month(), t.Day()}, true
}

func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

// AddDays is the date n days later.
func (d Date) AddDays(n int) Date {
	t := time.Date(d.Year, d.Month, d.Day+n, 12, 0, 0, 0, time.UTC)
	return Date{t.Year(), t.Month(), t.Day()}
}

// Weekday is the day of the week of d.
func (d Date) Weekday() time.Weekday {
	return time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.UTC).Weekday()
}

// at is c on d in loc. Go resolves the local time across daylight-saving
// changes, which happen at night, outside working hours.
func (d Date) at(c Clock, loc *time.Location) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, int(c), 0, 0, loc)
}

// Schedule is one member of staff's bookable hours.
type Schedule struct {
	Week Week
	// Step is Timely's "Can be booked online every": starts fall on this
	// grid, which runs from the day's first shift across any breaks
	// ("Reset the online booking time after a break" is off).
	Step    time.Duration
	DaysOff []Date
}

// Valid reports whether s can be used: a whole-minute step, at least one
// shift, and each day's shifts in order, non-empty and not overlapping.
func (s Schedule) Valid() bool {
	if s.Step <= 0 || s.Step%time.Minute != 0 {
		return false
	}
	shifts := 0
	for _, day := range s.Week {
		end := Clock(-1)
		for _, sh := range day {
			if sh.From < 0 || sh.From >= sh.To || sh.To > endOfDay || sh.From < end {
				return false
			}
			end = sh.To
			shifts++
		}
	}
	return shifts > 0
}

// Interval is a busy time, Start up to End.
type Interval struct{ Start, End time.Time }

// Starts is every time on day, in order, when a treatment lasting duration
// fits inside one of s's shifts without overlapping busy, and not before
// earliest. An invalid schedule, a day off and a non-positive duration
// offer nothing.
func (s Schedule) Starts(day Date, loc *time.Location, duration time.Duration, busy []Interval, earliest time.Time) []time.Time {
	shifts := s.Week[day.Weekday()]
	if duration <= 0 || !s.Valid() || len(shifts) == 0 || slices.Contains(s.DaysOff, day) {
		return nil
	}
	step, length := Clock(s.Step/time.Minute), Clock((duration+time.Minute-1)/time.Minute)
	var out []time.Time
	for c := shifts[0].From; c+length <= shifts[len(shifts)-1].To; c += step {
		if !slices.ContainsFunc(shifts, func(sh Shift) bool { return sh.From <= c && c+length <= sh.To }) {
			continue
		}
		start := day.at(c, loc)
		end := start.Add(duration)
		if start.Before(earliest) || slices.ContainsFunc(busy, func(b Interval) bool { return start.Before(b.End) && b.Start.Before(end) }) {
			continue
		}
		out = append(out, start)
	}
	return out
}

// Before reports whether d is earlier than o.
func (d Date) Before(o Date) bool { return d.String() < o.String() }
