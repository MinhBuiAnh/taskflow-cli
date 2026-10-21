package domain

import "time"

type Dependency struct {
	TaskID		int64
	DependsOnID int64
	CreatedAt   time.Time
}