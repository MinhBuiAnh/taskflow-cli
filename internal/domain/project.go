package domain

import (
	"errors"
	"strings"
	"time"
)

type ProjectStatus string

const (
	ProjectActive    ProjectStatus = "active"
	ProjectCompleted ProjectStatus = "completed"
	ProjectClosed    ProjectStatus = "closed"
	ProjectArchived  ProjectStatus = "archived"
)

var ErrInvalidProjectName = errors.New("project name cannot be empty")
var ErrInvalidProjectStatus = errors.New("invalid project status")

type Project struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Status      ProjectStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	ArchivedAt  *time.Time    `json:"archived_at,omitempty"`
}

func (p Project) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return ErrInvalidProjectName
	}
	switch p.Status {
	case ProjectActive, ProjectCompleted, ProjectClosed, ProjectArchived:
		return nil
	default:
		return ErrInvalidProjectStatus
	}
}