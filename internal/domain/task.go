package domain

import (
	"errors"
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskTodo	   TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone	   TaskStatus = "done"
	TaskBlocked	   TaskStatus = "blocked"
	TaskCancelled   TaskStatus = "cancelled"
)

type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityMedium TaskPriority = "medium"
	PriorityHigh   TaskPriority = "high"
	PriorityUrgent TaskPriority = "urgent"
)

var (
	ErrInvalidTaskTitle = errors.New("task title cannot be empty")
	ErrInvalidTaskStatus = errors.New("invalid task status")
	ErrInvalidTaskPriority = errors.New("invalid task priority")
)

type Task struct {
	ID		    int64
	ProjectID   int64
	ParentID    *int64
	Title       string
	Description string
	Status      TaskStatus
	Priority    TaskPriority
	Deadline    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	IsRequired  bool
}

func (t *Task) Validate() error {
	if strings.TrimSpace(t.Title) == "" {
		return ErrInvalidTaskTitle
	}

	switch t.Status {
	case TaskTodo, TaskInProgress, TaskDone, TaskBlocked, TaskCancelled:
		// Valid status
	default:
		return ErrInvalidTaskStatus
	}

	switch t.Priority {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
		// Valid priority
	default:
		return ErrInvalidTaskPriority
	}

	if t.ProjectID <= 0 {
		return errors.New("project_id must be positive")
	}
	if t.ParentID != nil && *t.ParentID <= 0 {
		return errors.New("parent_id must be positive")
	}

	return nil
}