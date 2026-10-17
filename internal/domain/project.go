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

var (
	ErrInvalidProjectName = errors.New("project name cannot be empty")
	ErrInvalidProjectStatus = errors.New("invalid project status")
)

type Project struct {
	ID int64
	Name string
	Description string
	Status ProjectStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	ArchivedAt *time.Time
}

func (p *Project) Validate() error {
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