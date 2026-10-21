package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/graph"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
)

var ErrProjectHasNoTasks = errors.New("project has no tasks")

// PlannedTask contains the task's effective status and any unfinished prerequisites.
type PlannedTask struct {
	TaskState
	Ready   bool `json:"ready"`
	Overdue bool `json:"overdue"`
}

// ProjectReport summarizes completion and blockers for a project.
type ProjectReport struct {
	ProjectID       int64   `json:"project_id"`
	ProjectName     string  `json:"project_name"`
	ProjectStatus   string  `json:"project_status"`
	TotalTasks      int     `json:"total_tasks"`
	TodoTasks       int     `json:"todo_tasks"`
	InProgressTasks int     `json:"in_progress_tasks"`
	BlockedTasks    int     `json:"blocked_tasks"`
	DoneTasks       int     `json:"done_tasks"`
	CancelledTasks  int     `json:"cancelled_tasks"`
	OverdueTasks    int     `json:"overdue_tasks"`
	RequiredTasks   int     `json:"required_tasks"`
	RequiredDone    int     `json:"required_done"`
	CompletionPct   float64 `json:"completion_percent"`
	CanComplete     bool    `json:"can_complete"`
}

type Planner struct {
	db       *sql.DB
	tasks    *repository.TaskRepository
	projects *repository.ProjectRepository
	deps     *repository.DependencyRepository
	states   *TaskService
}

func NewPlanner(db *sql.DB) *Planner {
	return &Planner{
		db:       db,
		tasks:    repository.NewTaskRepository(db),
		projects: repository.NewProjectRepository(db),
		deps:     repository.NewDependencyRepository(db),
		states:   NewTaskService(db),
	}
}

// Returns todo tasks whose prerequisites are all done, in a project that is active.
func (p *Planner) Ready(ctx context.Context, projectID int64) ([]PlannedTask, error) {
	project, err := p.projects.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if project.Status != domain.ProjectActive {
		return []PlannedTask{}, nil
	}
	all, err := p.listProjectTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedTask, 0)
	for _, item := range all {
		if item.Status == domain.TaskTodo && item.Ready {
			out = append(out, item)
		}
	}
	sortPlanned(out)
	return out, nil
}

// Returns tasks currently waiting on unfinished prerequisites.
func (p *Planner) Blocked(ctx context.Context, projectID int64) ([]PlannedTask, error) {
	all, err := p.listProjectTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedTask, 0)
	for _, item := range all {
		if item.Status == domain.TaskBlocked {
			out = append(out, item)
		}
	}
	sortPlanned(out)
	return out, nil
}

// Returns unfinished, non-cancelled tasks whose deadline is before the supplied date.
// Supplying now makes the method deterministic for tests and callers; zero uses current time.
func (p *Planner) Overdue(ctx context.Context, projectID int64, now time.Time) ([]PlannedTask, error) {
	if now.IsZero() {
		now = time.Now()
	}
	all, err := p.listProjectTasksAt(ctx, projectID, now)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedTask, 0)
	for _, item := range all {
		if item.Overdue {
			out = append(out, item)
		}
	}
	sortPlanned(out)
	return out, nil
}

// Returns tasks in dependency-safe execution order, prerequisites first.
// Completed and cancelled tasks remain in the plan so the full dependency chain is visible.
func (p *Planner) Plan(ctx context.Context, projectID int64) ([]PlannedTask, error) {
	all, err := p.listProjectTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	g := graph.NewDependencyGraph()
	byID := make(map[int64]PlannedTask, len(all))
	for _, item := range all {
		if err := g.AddTask(item.Task.ID); err != nil {
			return nil, err
		}
		byID[item.Task.ID] = item
	}
	dependencies, err := p.deps.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, d := range dependencies {
		if err := g.AddDependency(d.TaskID, d.DependsOnID); err != nil {
			return nil, fmt.Errorf("build project plan: %w", err)
		}
	}
	order, err := g.TopologicalOrder()
	if err != nil {
		return nil, fmt.Errorf("build project plan: %w", err)
	}
	out := make([]PlannedTask, 0, len(order))
	for _, id := range order {
		if item, ok := byID[id]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

// Returns project information and all of its tasks
func (p *Planner) Report(ctx context.Context, projectID int64, now time.Time) (*ProjectReport, error) {
	project, err := p.projects.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	all, err := p.listProjectTasksAt(ctx, projectID, now)
	if err != nil {
		return nil, err
	}
	r := &ProjectReport{ProjectID: project.ID, ProjectName: project.Name, ProjectStatus: string(project.Status), TotalTasks: len(all), CanComplete: true}
	for _, item := range all {
		switch item.Status {
		case domain.TaskTodo:
			r.TodoTasks++
		case domain.TaskInProgress:
			r.InProgressTasks++
		case domain.TaskBlocked:
			r.BlockedTasks++
		case domain.TaskDone:
			r.DoneTasks++
		case domain.TaskCancelled:
			r.CancelledTasks++
		}
		if item.Overdue {
			r.OverdueTasks++
		}
		if item.Task.IsRequired {
			r.RequiredTasks++
			if item.Status == domain.TaskDone {
				r.RequiredDone++
			} else {
				r.CanComplete = false
			}
		}
	}
	if r.TotalTasks > 0 {
		r.CompletionPct = float64(r.DoneTasks) * 100 / float64(r.TotalTasks)
	}
	return r, nil
}

func (p *Planner) listProjectTasks(ctx context.Context, projectID int64) ([]PlannedTask, error) {
	return p.listProjectTasksAt(ctx, projectID, time.Now())
}

func (p *Planner) listProjectTasksAt(ctx context.Context, projectID int64, now time.Time) ([]PlannedTask, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("project ID must be positive")
	}
	tasks, err := p.tasks.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedTask, 0, len(tasks))
	for _, task := range tasks {
		state, err := p.states.GetState(ctx, task.ID)
		if err != nil {
			return nil, err
		}
		ready := state.Status == domain.TaskTodo && len(state.BlockingTaskIDs) == 0
		overdue := task.Deadline != nil && task.Deadline.Before(now) && state.Status != domain.TaskDone && state.Status != domain.TaskCancelled
		out = append(out, PlannedTask{TaskState: *state, Ready: ready, Overdue: overdue})
	}
	return out, nil
}

func sortPlanned(items []PlannedTask) {
	priorityRank := map[domain.TaskPriority]int{domain.PriorityUrgent: 0, domain.PriorityHigh: 1, domain.PriorityMedium: 2, domain.PriorityLow: 3}
	sort.Slice(items, func(i, j int) bool {
		if priorityRank[items[i].Task.Priority] != priorityRank[items[j].Task.Priority] {
			return priorityRank[items[i].Task.Priority] < priorityRank[items[j].Task.Priority]
		}
		a, b := items[i].Task.Deadline, items[j].Task.Deadline
		if a != nil && b == nil {
			return true
		}
		if a == nil && b != nil {
			return false
		}
		if a != nil && b != nil && !a.Equal(*b) {
			return a.Before(*b)
		}
		return items[i].Task.ID < items[j].Task.ID
	})
}