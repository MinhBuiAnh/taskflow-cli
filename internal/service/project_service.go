package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
)

var ErrProjectRequiredTasksIncomplete = errors.New("project has required tasks that are not complete")

// ProjectService applies business rules before persisting project status changes.
type ProjectService struct {
	projects *repository.ProjectRepository
	planner  *Planner
}

func NewProjectService(db *sql.DB) *ProjectService {
	return &ProjectService{projects: repository.NewProjectRepository(db), planner: NewPlanner(db)}
}

func (s *ProjectService) SetStatus(ctx context.Context, projectID int64, status domain.ProjectStatus) error {
	if projectID <= 0 {
		return fmt.Errorf("project ID must be positive")
	}
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return err
	}
	p.Status = status
	if err := p.Validate(); err != nil {
		return err
	}
	if status == domain.ProjectCompleted {
		report, err := s.planner.Report(ctx, projectID, time.Now())
		if err != nil {
			return err
		}
		if !report.CanComplete {
			return fmt.Errorf("%w (%d/%d)", ErrProjectRequiredTasksIncomplete, report.RequiredDone, report.RequiredTasks)
		}
	}
	if status == domain.ProjectArchived {
		now := time.Now().UTC()
		p.ArchivedAt = &now
	} else {
		p.ArchivedAt = nil
	}
	return s.projects.Update(ctx, p)
}