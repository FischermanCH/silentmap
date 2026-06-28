// Package timeutil provides shared helpers for parsing SQLite datetime strings.
package timeutil

import (
	"strings"
	"time"
)

// ParseSQLiteTime parses SQLite datetime strings stored in multiple formats
// and returns the result in local time. Returns zero time on parse failure.
func ParseSQLiteTime(s string) time.Time {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	s = strings.TrimSuffix(s, "Z")
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}
