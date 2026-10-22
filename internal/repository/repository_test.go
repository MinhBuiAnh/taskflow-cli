package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/graph"
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

func TestDependencyRepositoryRejectsCyclesAndCrossProjectEdges(t *testing.T) {
	db, ctx := testDB(t)
	projects := NewProjectRepository(db)
	tasks := NewTaskRepository(db)
	deps := NewDependencyRepository(db)
	p1 := &domain.Project{Name: "Project One"}
	p2 := &domain.Project{Name: "Project Two"}
	if err := projects.Create(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := projects.Create(ctx, p2); err != nil {
		t.Fatal(err)
	}
	createTask := func(projectID int64, title string) *domain.Task {
		t.Helper()
		task := &domain.Task{ProjectID: projectID, Title: title}
		if err := tasks.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
		return task
	}
	a := createTask(p1.ID, "A")
	b := createTask(p1.ID, "B")
	c := createTask(p1.ID, "C")
	other := createTask(p2.ID, "Other project")

	if err := deps.Add(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := deps.Add(ctx, c.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := deps.Add(ctx, a.ID, c.ID); !errors.Is(err, graph.ErrCycle) {
		t.Fatalf("expected cycle error, got %v", err)
	}
	if err := deps.Add(ctx, a.ID, a.ID); !errors.Is(err, graph.ErrSelfDependency) {
		t.Fatalf("expected self-dependency error, got %v", err)
	}
	if err := deps.Add(ctx, a.ID, other.ID); !errors.Is(err, ErrTasksDifferentProjects) {
		t.Fatalf("expected cross-project error, got %v", err)
	}
	if err := deps.Add(ctx, a.ID, 99999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not-found error, got %v", err)
	}
	list, err := deps.ListByProject(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 persisted dependencies, got %d", len(list))
	}
	g, err := deps.GraphByProject(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if g.HasCycle() {
		t.Fatal("stored graph should remain acyclic after rejected edge")
	}
}

func TestDependencyRepositoryDuplicateAndRemove(t *testing.T) {
	db, ctx := testDB(t)
	projects := NewProjectRepository(db)
	tasks := NewTaskRepository(db)
	deps := NewDependencyRepository(db)
	p := &domain.Project{Name: "Demo"}
	if err := projects.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	a := &domain.Task{ProjectID: p.ID, Title: "A"}
	b := &domain.Task{ProjectID: p.ID, Title: "B"}
	if err := tasks.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := deps.Add(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := deps.Add(ctx, b.ID, a.ID); !errors.Is(err, ErrDependencyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if err := deps.Remove(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := deps.Remove(ctx, b.ID, a.ID); !errors.Is(err, ErrDependencyNotFound) {
		t.Fatalf("expected dependency not found, got %v", err)
	}
}

func TestDeleteTaskReferencedByDependencyIsRejectedWithoutDataLoss(t *testing.T) {
	db, ctx := testDB(t)
	project := &domain.Project{Name: "Dependency delete test"}
	if err := NewProjectRepository(db).Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(db)
	deps := NewDependencyRepository(db)
	prerequisite := &domain.Task{ProjectID: project.ID, Title: "Prerequisite"}
	dependent := &domain.Task{ProjectID: project.ID, Title: "Dependent"}
	if err := tasks.Create(ctx, prerequisite); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Create(ctx, dependent); err != nil {
		t.Fatal(err)
	}
	if err := deps.Add(ctx, dependent.ID, prerequisite.ID); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Delete(ctx, prerequisite.ID); err == nil {
		t.Fatal("expected referenced task deletion to fail")
	}
	if _, err := tasks.Get(ctx, prerequisite.ID); err != nil {
		t.Fatalf("failed deletion must preserve task: %v", err)
	}
	items, err := deps.ListByProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("failed deletion must preserve dependency, got %+v", items)
	}
}

func TestRepositoriesRejectInvalidDomainInputAndMissingRecords(t *testing.T) {
	db, ctx := testDB(t)
	projects := NewProjectRepository(db)
	if err := projects.Create(ctx, &domain.Project{Name: "   "}); !errors.Is(err, domain.ErrInvalidProjectName) {
		t.Fatalf("expected invalid project name, got %v", err)
	}
	if _, err := projects.Get(ctx, 123456); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing project, got %v", err)
	}
	if err := NewTaskRepository(db).Create(ctx, &domain.Task{ProjectID: 1, Title: "   "}); !errors.Is(err, domain.ErrInvalidTaskTitle) {
		t.Fatalf("expected invalid task title, got %v", err)
	}
}