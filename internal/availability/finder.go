package availability

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// Source reads busy times from calendars. A calendar it cannot read is
// missing from its answer; the Finder never treats a missing calendar as
// free.
type Source interface {
	Busy(ctx context.Context, calendars []string, from, to time.Time) (map[string][]Interval, error)
}

var (
	// ErrUnavailable means the calendars could not be read in time.
	ErrUnavailable = errors.New("availability unavailable")
	// ErrRange is a date range that is backwards or longer than MaxDays.
	ErrRange = errors.New("date range out of bounds")
)

// MaxDays is the longest range Find looks at, in days.
const MaxDays = 14

// readTimeout bounds one calendar read.
const readTimeout = 3 * time.Second

// ParseCalendars reads "<calendarId>|<staffId>;…": the calendar Timely
// syncs each member of staff to. Empty is no calendars. Errors name a pair
// by its position, never its contents: they are logged, and calendar IDs
// stay out of logs.
func ParseCalendars(config string) (map[int64]string, error) {
	out := map[int64]string{}
	seen := map[string]bool{}
	n := 0
	for pair := range strings.SplitSeq(config, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		n++
		cal, staff, ok := strings.Cut(pair, "|")
		cal, staff = strings.TrimSpace(cal), strings.TrimSpace(staff)
		id, err := strconv.ParseInt(staff, 10, 64)
		if !ok || cal == "" || err != nil {
			return nil, fmt.Errorf("availability calendars: pair %d is not <calendarId>|<staffId>", n)
		}
		if seen[cal] || out[id] != "" {
			return nil, fmt.Errorf("availability calendars: pair %d repeats a calendar or a member of staff", n)
		}
		seen[cal], out[id] = true, cal
	}
	return out, nil
}

// Person is a member of staff who may do a treatment.
type Person struct {
	ID       int64
	Name     string
	Schedule *Schedule
}

// Slot is a start time with one member of staff.
type Slot struct {
	StaffID int64
	Name    string
	Start   time.Time
}

// Result is what Find found: slots in time order, and the names of staff
// it could not check (no hours, no calendar, or an unreadable calendar).
type Result struct {
	Slots       []Slot
	Unavailable []string
}

// FinderOptions configure a Finder.
type FinderOptions struct {
	Source Source
	// Calendars maps staff IDs to their calendar IDs (ParseCalendars).
	Calendars map[int64]string
	// EveryStaff gives each member of staff a calendar named after their ID
	// when Calendars has none: for DevSource, which answers any name.
	EveryStaff bool
	// DevHours gives staff without hours of their own made-up ones (Monday
	// to Saturday, 10:00 to 18:30): for local work and the browser tests,
	// whose content snapshot has no hours.
	DevHours bool
	Location *time.Location
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Lead is how far ahead the first offered start must be.
	Lead time.Duration
}

// Finder offers booking times: working hours minus calendar busy times.
type Finder struct{ opts FinderOptions }

// NewFinder returns a Finder over opts.
func NewFinder(opts FinderOptions) *Finder {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Finder{opts: opts}
}

// Enabled reports whether the Finder can read any calendar.
func (f *Finder) Enabled() bool {
	return f != nil && f.opts.Source != nil && (len(f.opts.Calendars) > 0 || f.opts.EveryStaff)
}

// calendar is staff member id's calendar, or "" when there is none.
func (f *Finder) calendar(id int64) string {
	if c := f.opts.Calendars[id]; c != "" {
		return c
	}
	if f.opts.EveryStaff {
		return "staff-" + strconv.FormatInt(id, 10)
	}
	return ""
}

// Find returns up to limit starts for a treatment lasting duration, from
// the first to the last day (inclusive, at most MaxDays), with any of staff.
// It reads all their calendars in one request; a failed request is
// ErrUnavailable, never an empty answer.
func (f *Finder) Find(ctx context.Context, staff []Person, duration time.Duration, first, last Date, limit int) (Result, error) {
	loc := f.opts.Location
	start, end := first.at(0, loc), last.AddDays(1).at(0, loc)
	if !start.Before(end) || end.Sub(start) > MaxDays*24*time.Hour+time.Hour {
		return Result{}, ErrRange
	}
	var res Result
	var asked []Person
	var calendars []string
	for _, p := range staff {
		cal := f.calendar(p.ID)
		if p.Schedule == nil && f.opts.DevHours {
			p.Schedule = &devHours
		}
		if p.Schedule == nil || cal == "" {
			res.Unavailable = append(res.Unavailable, p.Name)
			continue
		}
		asked = append(asked, p)
		calendars = append(calendars, cal)
	}
	if len(asked) > 0 {
		readCtx, cancel := context.WithTimeout(ctx, readTimeout)
		defer cancel()
		busy, err := f.opts.Source.Busy(readCtx, calendars, start, end)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		earliest := f.opts.Now().Add(f.opts.Lead)
		for _, p := range asked {
			b, ok := busy[f.calendar(p.ID)]
			if !ok {
				res.Unavailable = append(res.Unavailable, p.Name)
				continue
			}
			for day := first; day != last.AddDays(1); day = day.AddDays(1) {
				for _, t := range p.Schedule.Starts(day, loc, duration, b, earliest) {
					res.Slots = append(res.Slots, Slot{StaffID: p.ID, Name: p.Name, Start: t})
				}
			}
		}
	}
	slices.SortStableFunc(res.Slots, func(a, b Slot) int { return cmp.Or(a.Start.Compare(b.Start), cmp.Compare(a.Name, b.Name)) })
	if len(res.Slots) > limit {
		res.Slots = res.Slots[:limit]
	}
	slices.Sort(res.Unavailable)
	return res, nil
}

// Google reads busy times with the Calendar API's freeBusy query. Each
// staff calendar is shared with the runtime service account at "See only
// free/busy", so no event details ever reach the site.
type Google struct{ svc *calendar.Service }

// NewGoogle connects with Application Default Credentials; opts are for
// tests.
func NewGoogle(ctx context.Context, opts ...option.ClientOption) (*Google, error) {
	opts = append([]option.ClientOption{option.WithScopes(calendar.CalendarFreebusyScope)}, opts...)
	svc, err := calendar.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("calendar client: %w", err)
	}
	return &Google{svc: svc}, nil
}

// Busy implements Source with one freeBusy query for every calendar. A
// calendar the answer marks with errors (a lost share, a deleted calendar)
// is left out.
func (g *Google) Busy(ctx context.Context, calendars []string, from, to time.Time) (map[string][]Interval, error) {
	req := &calendar.FreeBusyRequest{TimeMin: from.Format(time.RFC3339), TimeMax: to.Format(time.RFC3339)}
	for _, c := range calendars {
		req.Items = append(req.Items, &calendar.FreeBusyRequestItem{Id: c})
	}
	res, err := g.svc.Freebusy.Query(req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	out := map[string][]Interval{}
	for _, c := range calendars {
		cal, ok := res.Calendars[c]
		if !ok || len(cal.Errors) > 0 {
			continue
		}
		intervals := []Interval{}
		for _, p := range cal.Busy {
			start, errStart := time.Parse(time.RFC3339, p.Start)
			end, errEnd := time.Parse(time.RFC3339, p.End)
			if errStart != nil || errEnd != nil {
				return nil, fmt.Errorf("a calendar returned an unreadable busy time %q–%q", p.Start, p.End)
			}
			intervals = append(intervals, Interval{Start: start, End: end})
		}
		out[c] = intervals
	}
	return out, nil
}

// Warm makes one cheap call so the credentials and the connection exist
// before the first visitor asks for times.
func (g *Google) Warm(ctx context.Context) error {
	now := time.Now()
	_, err := g.svc.Freebusy.Query(&calendar.FreeBusyRequest{
		TimeMin: now.Format(time.RFC3339), TimeMax: now.Add(time.Minute).Format(time.RFC3339),
	}).Context(ctx).Do()
	return err
}

// devHours are FinderOptions.DevHours' made-up hours.
var devHours = func() Schedule {
	var w Week
	for d := time.Monday; d <= time.Saturday; d++ {
		w[d] = []Shift{{From: 10 * 60, To: 18*60 + 30}}
	}
	return Schedule{Week: w, Step: 15 * time.Minute}
}()

// DevSource is a made-up calendar for local development: each calendar is
// busy for two stretches a day, the same on every run.
type DevSource struct{ Location *time.Location }

// Busy implements Source.
func (s DevSource) Busy(_ context.Context, calendars []string, from, to time.Time) (map[string][]Interval, error) {
	out := map[string][]Interval{}
	for _, c := range calendars {
		for day := DateOf(from, s.Location); day.at(0, s.Location).Before(to); day = day.AddDays(1) {
			h := fnv.New32a()
			_, _ = h.Write([]byte(c + day.String()))
			n := Clock(h.Sum32() % 16)
			out[c] = append(out[c],
				Interval{Start: day.at(10*60+n*15, s.Location), End: day.at(11*60+30+n*15, s.Location)},
				Interval{Start: day.at(14*60+n%8*15, s.Location), End: day.at(15*60+n%8*15, s.Location)},
			)
		}
	}
	return out, nil
}
