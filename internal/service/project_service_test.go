package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
)

func openProjectServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := repository.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "project-service.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestProjectServiceRequiresRequiredTasksDone(t *testing.T) {
	ctx := context.Background()
	db := openProjectServiceTestDB(t)
	projects := repository.NewProjectRepository(db)
	p := &domain.Project{Name: "Project policy"}
	if err := projects.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{ProjectID: p.ID, Title: "Required task", IsRequired: true}
	if err := repository.NewTaskRepository(db).Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	if err := NewProjectService(db).SetStatus(ctx, p.ID, domain.ProjectCompleted); !errors.Is(err, ErrProjectRequiredTasksIncomplete) {
		t.Fatalf("expected required-task policy error, got %v", err)
	}
	stored, err := projects.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.ProjectActive {
		t.Fatalf("failed completion changed project status to %s", stored.Status)
	}

	if err := NewTaskService(db).Complete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := NewProjectService(db).SetStatus(ctx, p.ID, domain.ProjectCompleted); err != nil {
		t.Fatalf("complete project after required task is done: %v", err)
	}
	stored, err = projects.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.ProjectCompleted {
		t.Fatalf("expected completed status, got %s", stored.Status)
	}
}