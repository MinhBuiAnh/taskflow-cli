package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
	"github.com/MinhBuiAnh/taskflow-cli/internal/service"
)

func TestCobraCLIEndToEndDependencyWorkflow(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cli.db")
	db, err := repository.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{Name: "CLI integration"}
	if err := repository.NewProjectRepository(db).Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	tasks := repository.NewTaskRepository(db)
	prerequisite := &domain.Task{ProjectID: project.ID, Title: "Prerequisite"}
	dependent := &domain.Task{ProjectID: project.ID, Title: "Dependent"}
	if err := tasks.Create(ctx, prerequisite); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Create(ctx, dependent); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewDependencyRepository(db).Add(ctx, dependent.ID, prerequisite.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (string, error) {
		t.Helper()
		root := NewRootCommand()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append([]string{"--db", dbPath}, args...))
		err := root.Execute()
		return stdout.String(), err
	}

	if _, err := run("task", "start", "not-an-id"); err == nil || !strings.Contains(err.Error(), "số nguyên dương") {
		t.Fatalf("expected readable validation error, got %v", err)
	}
	if _, err := run("task", "start", stringID(dependent.ID)); !errors.Is(err, service.ErrTaskBlocked) {
		t.Fatalf("expected blocked task error, got %v", err)
	}

	// A failed command must not persist a state transition.
	db, err = repository.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.NewTaskService(db).GetState(ctx, dependent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Task.Status != domain.TaskTodo {
		t.Fatalf("failed start mutated stored status: %s", state.Task.Status)
	}
	if err := service.NewTaskService(db).Complete(ctx, prerequisite.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if out, err := run("task", "start", stringID(dependent.ID)); err != nil || !strings.Contains(out, "updated successfully") {
		t.Fatalf("expected successful start, output=%q err=%v", out, err)
	}
	if out, err := run("task", "status", stringID(dependent.ID)); err != nil || !strings.Contains(out, "Status: in_progress") {
		t.Fatalf("expected in_progress status, output=%q err=%v", out, err)
	}
	if out, err := run("report", stringID(project.ID)); err != nil || !strings.Contains(out, "In progress: 1") {
		t.Fatalf("expected report to reflect state transition, output=%q err=%v", out, err)
	}
}

func stringID(id int64) string { return strconv.FormatInt(id, 10) }

func TestCobraCLIProjectTaskAndDependencyCreation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "create.db")
	run := func(args ...string) (string, error) {
		t.Helper()
		root := NewRootCommand()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append([]string{"--db", dbPath}, args...))
		err := root.Execute()
		return stdout.String(), err
	}

	out, err := run("project", "create", "Demo project", "--description", "CLI integration test")
	if err != nil || !strings.Contains(out, "Đã tạo project #1") {
		t.Fatalf("create project: output=%q err=%v", out, err)
	}
	out, err = run("task", "create", "1", "Prepare", "--priority", "high", "--deadline", "2027-01-31", "--required=true")
	if err != nil || !strings.Contains(out, "Đã tạo task #1") {
		t.Fatalf("create prerequisite: output=%q err=%v", out, err)
	}
	out, err = run("task", "create", "1", "Build", "--required=false")
	if err != nil || !strings.Contains(out, "Đã tạo task #2") {
		t.Fatalf("create dependent: output=%q err=%v", out, err)
	}
	if out, err = run("dependency", "add", "2", "1"); err != nil || !strings.Contains(out, "phụ thuộc task #1") {
		t.Fatalf("add dependency: output=%q err=%v", out, err)
	}
	if out, err = run("task", "status", "2"); err != nil || !strings.Contains(out, "Status: blocked") {
		t.Fatalf("dependent task should be blocked: output=%q err=%v", out, err)
	}
	if out, err = run("dependency", "list", "1"); err != nil || !strings.Contains(out, "2\t1") {
		t.Fatalf("list dependencies: output=%q err=%v", out, err)
	}
}

func TestCobraCLIRejectsCompletingProjectWithRequiredTasksOutstanding(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-status.db")
	ctx := context.Background()
	db, err := repository.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	p := &domain.Project{Name: "Incomplete project"}
	if err := repository.NewProjectRepository(db).Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{ProjectID: p.ID, Title: "Required work", IsRequired: true}
	if err := repository.NewTaskRepository(db).Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"--db", dbPath, "project", "set-status", stringID(p.ID), "completed"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); !errors.Is(err, service.ErrProjectRequiredTasksIncomplete) {
		t.Fatalf("expected completion policy error, got %v", err)
	}
}