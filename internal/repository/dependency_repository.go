package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/graph"
)

var (
	ErrDependencyNotFound     = errors.New("dependency not found")
	ErrTasksDifferentProjects = errors.New("dependency tasks must belong to the same project")
	ErrDependencyExists       = errors.New("dependency already exists")
)

type DependencyRepository struct{
	db *sql.DB
}

func NewDependencyRepository(db *sql.DB) *DependencyRepository {
	return &DependencyRepository{db: db}
}

// Adds new dependency, validates task existence, same-project membership, and cycle safety in one transaction before persisting the edge.
func (r *DependencyRepository) Add(ctx context.Context, taskID, dependsOnID int64) error {
	// Validate dependency ID
	if taskID <= 0 || dependsOnID <= 0 {
		return graph.ErrInvalidTaskID
	}
	if taskID == dependsOnID {
		return graph.ErrSelfDependency
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin add dependency: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get project ID from both tasks and ensure both are in the same project
	var taskProject, prerequisiteProject int64
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM tasks WHERE id = ?`, taskID).Scan(&taskProject); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get dependent task: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM tasks WHERE id = ?`, dependsOnID).Scan(&prerequisiteProject); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get prerequisite task: %w", err)
	}
	if taskProject != prerequisiteProject {
		return ErrTasksDifferentProjects
	}

	// Check if dependency exists
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM dependencies WHERE task_id = ? AND depends_on_id = ?`, taskID, dependsOnID).Scan(&exists); err != nil {
		return fmt.Errorf("check existing dependency: %w", err)
	}
	if exists > 0 {
		return ErrDependencyExists
	}

	// Get the project's dependencies and build the dependency graph
	// And add the new dependency into the graph
	rows, err := tx.QueryContext(ctx, `SELECT task_id, depends_on_id FROM dependencies WHERE project_id = ?`, taskProject)
	if err != nil {
		return fmt.Errorf("load project dependencies: %w", err)
	}
	g := graph.NewDependencyGraph()
	for rows.Next() {
		var currentTask, currentPrerequisite int64
		if err := rows.Scan(&currentTask, &currentPrerequisite); err != nil {
			rows.Close()
			return fmt.Errorf("scan dependency: %w", err)
		}
		if err := g.AddDependency(currentTask, currentPrerequisite); err != nil {
			rows.Close()
			return fmt.Errorf("existing dependency graph is invalid: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate dependencies: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close dependency rows: %w", err)
	}
	if err := g.AddDependency(taskID, dependsOnID); err != nil {
		return err
	}

	// Insert new dependency into the database
	_, err = tx.ExecContext(ctx, `INSERT INTO dependencies(task_id, depends_on_id, project_id, created_at) VALUES (?, ?, ?, ?)`, taskID, dependsOnID, taskProject, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert dependency: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dependency: %w", err)
	}

	return nil
}


// Removes the dependency by the task ID and the task ID it depends on
func (r *DependencyRepository) Remove(ctx context.Context, taskID, dependsOnID int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM dependencies WHERE task_id = ? AND depends_on_id = ?`, taskID, dependsOnID)
	if err != nil {
		return fmt.Errorf("remove dependency: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count removed dependencies: %w", err)
	}
	if n == 0 {
		return ErrDependencyNotFound
	}
	return nil
}

// Lists all dependencies by project ID
func (r *DependencyRepository) ListByProject(ctx context.Context, projectID int64) ([]domain.Dependency, error) {
	// Gets all dependencies from the database
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, depends_on_id, created_at FROM dependencies WHERE project_id = ? ORDER BY task_id, depends_on_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project dependencies: %w", err)
	}
	defer rows.Close()

	// Map to a list of Dependencies
	out := make([]domain.Dependency, 0)
	for rows.Next() {
		var d domain.Dependency
		var createdAt string
		if err := rows.Scan(&d.TaskID, &d.DependsOnID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		d.CreatedAt, err = parseStamp(createdAt)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project dependencies: %w", err)
	}

	return out, nil
}

// List all dependencies by task ID
func (r *DependencyRepository) ListForTask(ctx context.Context, taskID int64) ([]domain.Dependency, error) {
	// Gets all dependencies from the database
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, depends_on_id, created_at FROM dependencies WHERE task_id = ? ORDER BY depends_on_id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task dependencies: %w", err)
	}
	defer rows.Close()

	// Map to a list of Dependencies
	out := make([]domain.Dependency, 0)
	for rows.Next() {
		var d domain.Dependency
		var createdAt string
		if err := rows.Scan(&d.TaskID, &d.DependsOnID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		d.CreatedAt, err = parseStamp(createdAt)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task dependencies: %w", err)
	}

	return out, nil
}

// Builds an in-memory graph snapshot for a project.
func (r *DependencyRepository) GraphByProject(ctx context.Context, projectID int64) (*graph.DependencyGraph, error) {
	g := graph.NewDependencyGraph()
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM tasks WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project tasks: %w", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan task ID: %w", err)
		}
		if err := g.AddTask(id); err != nil {
			rows.Close()
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate project tasks: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close task rows: %w", err)
	}
	dependencies, err := r.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, d := range dependencies {
		if err := g.AddDependency(d.TaskID, d.DependsOnID); err != nil {
			return nil, fmt.Errorf("build dependency graph: %w", err)
		}
	}
	return g, nil
}