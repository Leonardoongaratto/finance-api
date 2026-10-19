// Package domain holds the pure finance rules: no I/O, no clocks, no persistence.
package domain

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	dateLayout  = "2006-01-02"
	monthLayout = "2006-01"
	minYear     = 1900
	maxYear     = 2199
)

var (
	ErrInvalidDate  = errors.New("date must be YYYY-MM-DD with a year from 1900 to 2199")
	ErrInvalidMonth = errors.New("month must be YYYY-MM with a year from 1900 to 2199")
)

type Date string

type Month string

func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil || !supportedYear(t) {
		return "", ErrInvalidDate
	}
	return DateOf(t), nil
}

func supportedYear(t time.Time) bool {
	return t.Year() >= minYear && t.Year() <= maxYear
}

func DateOf(t time.Time) Date {
	return Date(t.Format(dateLayout))
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return ErrInvalidDate
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d Date) Time() time.Time {
	t, _ := time.Parse(dateLayout, string(d))
	return t
}

func (d Date) Month() Month {
	return Month(d[:7])
}

func (d Date) AddMonths(n int) Date {
	t := d.Time()
	target := Month(t.Format(monthLayout)).Add(n)
	lastDay := target.EndDate().Time().Day()
	day := min(t.Day(), lastDay)
	return DateOf(time.Date(target.Time().Year(), target.Time().Month(), day, 0, 0, 0, 0, time.UTC))
}

func ParseMonth(s string) (Month, error) {
	t, err := time.Parse(monthLayout, s)
	if err != nil || !supportedYear(t) {
		return "", ErrInvalidMonth
	}
	return Month(t.Format(monthLayout)), nil
}

func (m Month) Time() time.Time {
	t, _ := time.Parse(monthLayout, string(m))
	return t
}

func (m Month) Add(n int) Month {
	return Month(m.Time().AddDate(0, n, 0).Format(monthLayout))
}

func (m Month) EndDate() Date {
	return DateOf(m.Time().AddDate(0, 1, -1))
}

func MonthsBetween(from, to Month) int {
	a, b := from.Time(), to.Time()
	return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
}

func MonthRange(from, to Month) []Month {
	var months []Month
	for m := from; m <= to; m = m.Add(1) {
		months = append(months, m)
	}
	return months
}
