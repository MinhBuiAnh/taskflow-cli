package repository

import (
	"database/sql"
	"time"
)

// Converts time type variable into a string representation
func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// This is used for nullable time columns
func nullableStamp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return stamp(*t)
}

// Converts back from string representation to the actual time variable
func parseStamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// SQLite doesn't support boolean type, so we have to convert bool to int before inserting
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Will be used later
var _ sql.Scanner = (*sql.NullString)(nil)