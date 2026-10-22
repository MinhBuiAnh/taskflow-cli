package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
)

type TaskRepository struct{ db *sql.DB }

func NewTaskRepository(db *sql.DB) *TaskRepository { return &TaskRepository{db: db} }

func (r *TaskRepository) Create(ctx context.Context, t *domain.Task) error {
	if t.Status == "" {
		t.Status = domain.TaskTodo
	}
	if t.Priority == "" {
		t.Priority = domain.PriorityMedium
	}
	if err := t.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	result, err := r.db.ExecContext(ctx, `INSERT INTO tasks(project_id, parent_id, title, description, status, priority, deadline, is_required, created_at, updated_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, t.ProjectID, t.ParentID, t.Title, t.Description, t.Status, t.Priority, nullableStamp(t.Deadline), boolInt(t.IsRequired), stamp(t.CreatedAt), stamp(t.UpdatedAt), nullableStamp(t.CompletedAt))
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	t.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get task ID: %w", err)
	}
	return nil
}

func (r *TaskRepository) Get(ctx context.Context, id int64) (*domain.Task, error) {
	t := new(domain.Task)
	var deadline, completed sql.NullString
	var parentID sql.NullInt64
	var created, updated string
	var required int
	err := r.db.QueryRowContext(ctx, `SELECT id, project_id, parent_id, title, description, status, priority, deadline, is_required, created_at, updated_at, completed_at FROM tasks WHERE id=?`, id).Scan(&t.ID, &t.ProjectID, &parentID, &t.Title, &t.Description, &t.Status, &t.Priority, &deadline, &required, &created, &updated, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if parentID.Valid {
		v := parentID.Int64
		t.ParentID = &v
	}
	if deadline.Valid {
		v, e := parseStamp(deadline.String)
		if e != nil {
			return nil, e
		}
		t.Deadline = &v
	}
	if completed.Valid {
		v, e := parseStamp(completed.String)
		if e != nil {
			return nil, e
		}
		t.CompletedAt = &v
	}
	t.IsRequired = required == 1
	t.CreatedAt, err = parseStamp(created)
	if err != nil {
		return nil, err
	}
	t.UpdatedAt, err = parseStamp(updated)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *TaskRepository) ListByProject(ctx context.Context, projectID int64) ([]domain.Task, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM tasks WHERE project_id=? ORDER BY id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(ids))
	for _, id := range ids {
		t, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

func (r *TaskRepository) Update(ctx context.Context, t *domain.Task) error {
	if err := t.Validate(); err != nil {
		return err
	}
	t.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE tasks SET project_id=?, parent_id=?, title=?, description=?, status=?, priority=?, deadline=?, is_required=?, updated_at=?, completed_at=? WHERE id=?`, t.ProjectID, t.ParentID, t.Title, t.Description, t.Status, t.Priority, nullableStamp(t.Deadline), boolInt(t.IsRequired), stamp(t.UpdatedAt), nullableStamp(t.CompletedAt), t.ID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TaskRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}