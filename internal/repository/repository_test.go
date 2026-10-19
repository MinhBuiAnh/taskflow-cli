package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
)

// Test database initialization
func testDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migration should be idempotent: %v", err)
	}
	return db, ctx
}

func TestProjectCRUD(t *testing.T) {
	db, ctx := testDB(t)
	repo := NewProjectRepository(db)
	p := &domain.Project{Name: "Demo", Description: "test"}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if p.ID == 0 {
		t.Fatal("expected generated ID")
	}
	got, err := repo.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Demo" || got.Status != domain.ProjectActive {
		t.Fatalf("unexpected project: %+v", got)
	}
	got.Name = "Updated"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Updated" {
		t.Fatalf("unexpected project list: %+v", list)
	}
	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTaskCRUDAndForeignKeys(t *testing.T) {
	db, ctx := testDB(t)
	projects := NewProjectRepository(db)
	tasks := NewTaskRepository(db)
	p := &domain.Project{Name: "Demo"}
	if err := projects.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{ProjectID: p.ID, Title: "First task"}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if task.Status != domain.TaskTodo || task.Priority != domain.PriorityMedium {
		t.Fatalf("defaults not applied: %+v", task)
	}
	got, err := tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.Title = "Updated task"
	if err := tasks.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := tasks.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Title != "Updated task" {
		t.Fatalf("unexpected tasks: %+v", list)
	}
	if err := projects.Delete(ctx, p.ID); err == nil {
		t.Fatal("expected deleting project with tasks to fail")
	}
	if err := tasks.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := projects.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSubtaskMustBelongToSameProject(t *testing.T) {
	db, ctx := testDB(t)
	projects := NewProjectRepository(db)
	tasks := NewTaskRepository(db)
	p1 := &domain.Project{Name: "One"}
	p2 := &domain.Project{Name: "Two"}
	if err := projects.Create(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := projects.Create(ctx, p2); err != nil {
		t.Fatal(err)
	}
	parent := &domain.Task{ProjectID: p1.ID, Title: "Parent"}
	if err := tasks.Create(ctx, parent); err != nil {
		t.Fatal(err)
	}
	parentID := parent.ID
	child := &domain.Task{ProjectID: p2.ID, ParentID: &parentID, Title: "Invalid child"}
	if err := tasks.Create(ctx, child); err == nil {
		t.Fatal("expected cross-project parent to be rejected")
	}
}