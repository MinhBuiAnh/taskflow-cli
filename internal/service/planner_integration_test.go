package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
)

func setupPlannerIntegration(t *testing.T) (context.Context, *sql.DB, *domain.Project, *repository.TaskRepository, *repository.DependencyRepository, *Planner) {
	t.Helper()
	ctx := context.Background()
	db, err := repository.OpenSQLite(ctx, filepath.Join(t.TempDir(), "planner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repository.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{Name: "Planner integration"}
	if err := repository.NewProjectRepository(db).Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	return ctx, db, project, repository.NewTaskRepository(db), repository.NewDependencyRepository(db), NewPlanner(db)
}

func TestPlannerReadyBlockedOverdueAndReportIntegration(t *testing.T) {
	ctx, db, project, tasks, deps, planner := setupPlannerIntegration(t)
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	prerequisite := &domain.Task{ProjectID: project.ID, Title: "Prerequisite", IsRequired: true}
	dependent := &domain.Task{ProjectID: project.ID, Title: "Dependent", Deadline: &past, IsRequired: true}
	optional := &domain.Task{ProjectID: project.ID, Title: "Optional", Deadline: &past, Priority: domain.PriorityHigh}
	for _, task := range []*domain.Task{prerequisite, dependent, optional} {
		if err := tasks.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	if err := deps.Add(ctx, dependent.ID, prerequisite.ID); err != nil {
		t.Fatal(err)
	}

	ready, err := planner.Ready(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 2 {
		t.Fatalf("expected prerequisite and optional task ready, got %d", len(ready))
	}
	blocked, err := planner.Blocked(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0].Task.ID != dependent.ID {
		t.Fatalf("unexpected blocked tasks: %+v", blocked)
	}
	overdue, err := planner.Overdue(ctx, project.ID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(overdue) != 2 {
		t.Fatalf("expected blocked and optional unfinished tasks overdue, got %+v", overdue)
	}

	report, err := planner.Report(ctx, project.ID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalTasks != 3 || report.BlockedTasks != 1 || report.OverdueTasks != 2 || report.RequiredTasks != 2 || report.CanComplete {
		t.Fatalf("unexpected project report: %+v", report)
	}
	if err := NewTaskService(db).Complete(ctx, prerequisite.ID); err != nil {
		t.Fatal(err)
	}
	if err := NewTaskService(db).Complete(ctx, dependent.ID); err != nil {
		t.Fatal(err)
	}
	report, err = planner.Report(ctx, project.ID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.RequiredDone != 2 || !report.CanComplete {
		t.Fatalf("required tasks should allow project completion: %+v", report)
	}
}

// Test the public planner behavior for missing project IDs without relying on CLI parsing.
func TestPlannerMissingProjectAndInvalidID(t *testing.T) {
	ctx, _, _, _, _, planner := setupPlannerIntegration(t)
	if _, err := planner.Ready(ctx, 99999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := planner.Plan(ctx, 0); err == nil {
		t.Fatal("expected invalid project ID to fail")
	}
	if _, err := planner.Blocked(ctx, 99999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected missing project error for blocked query, got %v", err)
	}
	if _, err := planner.Overdue(ctx, 99999, time.Now()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected missing project error for overdue query, got %v", err)
	}
}