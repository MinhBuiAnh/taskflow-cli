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

func setupServiceTest(t *testing.T) (context.Context, *sql.DB, *domain.Project, *repository.TaskRepository, *repository.DependencyRepository, *TaskService) {
	t.Helper()
	ctx := context.Background()
	db, err := repository.OpenSQLite(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repository.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{Name: "Service test"}
	if err := repository.NewProjectRepository(db).Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	return ctx, db, project, repository.NewTaskRepository(db), repository.NewDependencyRepository(db), NewTaskService(db)
}

func createServiceTask(t *testing.T, ctx context.Context, repo *repository.TaskRepository, projectID int64, title string) *domain.Task {
	t.Helper()
	task := &domain.Task{ProjectID: projectID, Title: title}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestTaskTransitionsRespectDependencies(t *testing.T) {
	ctx, _, project, tasks, deps, svc := setupServiceTest(t)
	prerequisite := createServiceTask(t, ctx, tasks, project.ID, "Prerequisite")
	dependent := createServiceTask(t, ctx, tasks, project.ID, "Dependent")
	if err := deps.Add(ctx, dependent.ID, prerequisite.ID); err != nil {
		t.Fatal(err)
	}

	state, err := svc.GetState(ctx, dependent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != domain.TaskBlocked || state.Task.Status != domain.TaskTodo {
		t.Fatalf("expected effective blocked while stored status remains todo, got effective=%s stored=%s", state.Status, state.Task.Status)
	}
	if len(state.BlockingTaskIDs) != 1 || state.BlockingTaskIDs[0] != prerequisite.ID {
		t.Fatalf("unexpected blocking task IDs: %v", state.BlockingTaskIDs)
	}
	if err := svc.Start(ctx, dependent.ID); !errors.Is(err, ErrTaskBlocked) {
		t.Fatalf("expected start to be rejected as blocked, got %v", err)
	}
	if err := svc.Complete(ctx, dependent.ID); !errors.Is(err, ErrTaskBlocked) {
		t.Fatalf("expected completion to be rejected as blocked, got %v", err)
	}

	// A task with no prerequisites may be completed directly from todo.
	if err := svc.Complete(ctx, prerequisite.ID); err != nil {
		t.Fatalf("complete prerequisite directly: %v", err)
	}
	completed, err := tasks.Get(ctx, prerequisite.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domain.TaskDone || completed.CompletedAt == nil {
		t.Fatalf("completion timestamp/status not persisted: %+v", completed)
	}
	state, err = svc.GetState(ctx, dependent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != domain.TaskTodo || len(state.BlockingTaskIDs) != 0 {
		t.Fatalf("dependent should become ready after prerequisite completion: %+v", state)
	}
	if err := svc.Start(ctx, dependent.ID); err != nil {
		t.Fatalf("start ready task: %v", err)
	}
	if err := svc.Complete(ctx, dependent.ID); err != nil {
		t.Fatalf("complete in-progress task: %v", err)
	}
}

func TestCancelBlockedTaskAndRejectInvalidTransitions(t *testing.T) {
	ctx, _, project, tasks, deps, svc := setupServiceTest(t)
	prerequisite := createServiceTask(t, ctx, tasks, project.ID, "Prerequisite")
	dependent := createServiceTask(t, ctx, tasks, project.ID, "Dependent")
	if err := deps.Add(ctx, dependent.ID, prerequisite.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Cancel(ctx, dependent.ID); err != nil {
		t.Fatalf("cancel blocked task should be allowed: %v", err)
	}
	if err := svc.Start(ctx, dependent.ID); !errors.Is(err, ErrTaskNotStartable) {
		t.Fatalf("expected cancelled task not to be startable, got %v", err)
	}
	if err := svc.Complete(ctx, dependent.ID); !errors.Is(err, ErrTaskNotCompletable) {
		t.Fatalf("expected cancelled task not to be completable, got %v", err)
	}
	cancelled, err := tasks.Get(ctx, dependent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != domain.TaskCancelled || cancelled.CompletedAt != nil {
		t.Fatalf("unexpected cancelled task state: %+v", cancelled)
	}
}

func TestTaskTransitionsRequireActiveProject(t *testing.T) {
	ctx, db, project, tasks, _, svc := setupServiceTest(t)
	task := createServiceTask(t, ctx, tasks, project.ID, "Task")
	project.Status = domain.ProjectClosed
	if err := repository.NewProjectRepository(db).Update(ctx, project); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(ctx, task.ID); !errors.Is(err, ErrProjectNotActive) {
		t.Fatalf("expected closed project to reject transition, got %v", err)
	}
}
