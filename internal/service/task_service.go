package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
)

var (
	ErrInvalidTransition  = errors.New("invalid task status transition")
	ErrTaskBlocked        = errors.New("task is blocked by unfinished prerequisites")
	ErrTaskNotStartable   = errors.New("task is not in todo status")
	ErrTaskNotCompletable = errors.New("task cannot be completed from its current status")
	ErrTaskNotCancellable = errors.New("task cannot be cancelled from its current status")
	ErrProjectNotActive   = errors.New("tasks can only be changed in an active project")
)

// TaskState combines the persisted task with its effective, derived status.
// Status is Blocked when the stored status is todo and prerequisites remain.
type TaskState struct {
	Task            domain.Task       `json:"task"`
	Status          domain.TaskStatus `json:"status"`
	BlockingTaskIDs []int64           `json:"blocking_task_ids,omitempty"`
}

type TaskService struct{ db *sql.DB }

func NewTaskService(db *sql.DB) *TaskService { return &TaskService{db: db} }

// Start moves a todo task to in_progress only when every prerequisite is done.
func (s *TaskService) Start(ctx context.Context, taskID int64) error {
	return s.transition(ctx, taskID, domain.TaskInProgress)
}

// Complete moves a todo or in_progress task to done only when every
// prerequisite is done. A todo task may be completed directly if it is ready.
func (s *TaskService) Complete(ctx context.Context, taskID int64) error {
	return s.transition(ctx, taskID, domain.TaskDone)
}

// Cancel cancels a todo or in_progress task, including a blocked task.
func (s *TaskService) Cancel(ctx context.Context, taskID int64) error {
	return s.transition(ctx, taskID, domain.TaskCancelled)
}

func (s *TaskService) transition(ctx context.Context, taskID int64, target domain.TaskStatus) (retErr error) {
	if taskID <= 0 {
		return fmt.Errorf("task ID must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin task transition: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	var current domain.TaskStatus
	var projectStatus domain.ProjectStatus
	err = tx.QueryRowContext(ctx, `SELECT t.status, p.status FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=?`, taskID).Scan(&current, &projectStatus)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return repository.ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("load task state: %w", err)
	}
	if projectStatus != domain.ProjectActive {
		_ = tx.Rollback()
		return ErrProjectNotActive
	}

	switch target {
	case domain.TaskInProgress:
		if current != domain.TaskTodo {
			_ = tx.Rollback()
			return fmt.Errorf("%w: %s -> %s", ErrTaskNotStartable, current, target)
		}
	case domain.TaskDone:
		if current != domain.TaskTodo && current != domain.TaskInProgress {
			_ = tx.Rollback()
			return fmt.Errorf("%w: %s -> %s", ErrTaskNotCompletable, current, target)
		}
	case domain.TaskCancelled:
		if current != domain.TaskTodo && current != domain.TaskInProgress {
			_ = tx.Rollback()
			return fmt.Errorf("%w: %s -> %s", ErrTaskNotCancellable, current, target)
		}
	default:
		_ = tx.Rollback()
		return fmt.Errorf("%w: unsupported target status %q", ErrInvalidTransition, target)
	}

	if target == domain.TaskInProgress || target == domain.TaskDone {
		blocking, err := blockingTaskIDs(ctx, tx, taskID)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if len(blocking) > 0 {
			_ = tx.Rollback()
			return fmt.Errorf("%w: task %d depends on unfinished task IDs %v", ErrTaskBlocked, taskID, blocking)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var result sql.Result
	if target == domain.TaskDone {
		result, err = tx.ExecContext(ctx, `UPDATE tasks SET status=?, completed_at=?, updated_at=? WHERE id=?`, target, now, now, taskID)
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE tasks SET status=?, completed_at=NULL, updated_at=? WHERE id=?`, target, now, taskID)
	}
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("persist task transition: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("check task transition: %w", err)
	}
	if affected == 0 {
		_ = tx.Rollback()
		return repository.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task transition: %w", err)
	}
	return nil
}

// GetState returns a task with its effective status and IDs of unfinished
// prerequisites. The stored status remains todo when effective status is blocked.
func (s *TaskService) GetState(ctx context.Context, taskID int64) (*TaskState, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("task ID must be positive")
	}
	tasks := repository.NewTaskRepository(s.db)
	t, err := tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	blocking, err := blockingTaskIDsDB(ctx, s.db, taskID)
	if err != nil {
		return nil, err
	}
	state := &TaskState{Task: *t, Status: t.Status, BlockingTaskIDs: blocking}
	if t.Status == domain.TaskTodo && len(blocking) > 0 {
		state.Status = domain.TaskBlocked
	}
	return state, nil
}

func blockingTaskIDs(ctx context.Context, tx *sql.Tx, taskID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT d.depends_on_id FROM dependencies d JOIN tasks prerequisite ON prerequisite.id=d.depends_on_id WHERE d.task_id=? AND prerequisite.status<>? ORDER BY d.depends_on_id`, taskID, domain.TaskDone)
	if err != nil {
		return nil, fmt.Errorf("check task prerequisites: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read blocking task: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blocking tasks: %w", err)
	}
	return ids, nil
}

func blockingTaskIDsDB(ctx context.Context, db *sql.DB, taskID int64) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT d.depends_on_id FROM dependencies d JOIN tasks prerequisite ON prerequisite.id=d.depends_on_id WHERE d.task_id=? AND prerequisite.status<>? ORDER BY d.depends_on_id`, taskID, domain.TaskDone)
	if err != nil {
		return nil, fmt.Errorf("check task prerequisites: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read blocking task: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blocking tasks: %w", err)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}