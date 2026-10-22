package domain

import "time"

// Dependency means TaskID cannot proceed until DependsOnTaskID is done.
type Dependency struct {
	TaskID      int64     `json:"task_id"`
	DependsOnID int64     `json:"depends_on_id"`
	CreatedAt   time.Time `json:"created_at"`
}