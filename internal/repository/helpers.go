package repository

import (
	"database/sql"
	"time"
)

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func nullableStamp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return stamp(*t)
}
func parseStamp(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

var _ sql.Scanner = (*sql.NullString)(nil)