package domain

import (
	"errors"
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	// TaskBlocked is an effective status derived from unfinished prerequisites; it is not persisted.
	TaskBlocked   TaskStatus = "blocked"
	TaskDone      TaskStatus = "done"
	TaskCancelled TaskStatus = "cancelled"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

var ErrInvalidTaskTitle = errors.New("task title cannot be empty")
var ErrInvalidTaskStatus = errors.New("invalid task status")
var ErrInvalidPriority = errors.New("invalid task priority")

type Task struct {
	ID          int64      `json:"id"`
	ProjectID   int64      `json:"project_id"`
	ParentID    *int64     `json:"parent_id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	Priority    Priority   `json:"priority"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	IsRequired  bool       `json:"is_required"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Title) == "" {
		return ErrInvalidTaskTitle
	}
	switch t.Status {
	case TaskTodo, TaskInProgress, TaskDone, TaskCancelled:
	default:
		return ErrInvalidTaskStatus
	}
	switch t.Priority {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
	default:
		return ErrInvalidPriority
	}
	if t.ProjectID <= 0 {
		return errors.New("project_id must be positive")
	}
	if t.ParentID != nil && *t.ParentID <= 0 {
		return errors.New("parent_id must be positive")
	}
	return nil
}