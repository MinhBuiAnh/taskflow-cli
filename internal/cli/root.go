/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
	"github.com/MinhBuiAnh/taskflow-cli/internal/service"
	"github.com/spf13/cobra"
)

// NewRootCommand builds the CLI. Database initialization is delayed until a real
// subcommand runs, so `taskflow --help` works without opening a database.
func NewRootCommand() *cobra.Command {
	var dbPath string
	var db *sql.DB

	root := &cobra.Command{
		Use:           "taskflow",
		Short:         "Manage projects, tasks and dependencies",
		Long:          "TaskFlow is a CLI tool for managing projects with dependencies, planning and progress.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			opened, err := repository.OpenSQLite(ctx, dbPath)
			if err != nil {
				return err
			}
			if err := repository.Migrate(ctx, opened); err != nil {
				_ = opened.Close()
				return err
			}
			db = opened
			return nil
		},
	}
	root.PersistentFlags().StringVar(&dbPath, "db", "taskflow.db", "path to SQLite file")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return err })
	cobra.OnFinalize(func() {
		if db != nil {
			_ = db.Close()
		}
	})

	projectCmd := &cobra.Command{Use: "project", Short: "Manage projects"}
	projectCmd.AddCommand(newProjectCreateCommand(func() *sql.DB { return db }))
	projectCmd.AddCommand(newProjectShowCommand(func() *sql.DB { return db }))
	projectCmd.AddCommand(newProjectUpdateCommand(func() *sql.DB { return db }))
	projectCmd.AddCommand(newProjectStatusCommand(func() *sql.DB { return db }))
	projectCmd.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List projects", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := repository.NewProjectRepository(db).List(cmd.Context())
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No projects yet.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ID\tSTATUS\tNAME")
			for _, p := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\n", p.ID, p.Status, p.Name)
			}
			return nil
		},
	})
	root.AddCommand(projectCmd)

	taskCmd := &cobra.Command{Use: "task", Short: "Lookup and change task status"}
	taskCmd.AddCommand(newTaskCreateCommand(func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskUpdateCommand(func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskDeleteCommand(func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskListCommand(func() *sql.DB { return db }))
	depCmd := &cobra.Command{Use: "dependency", Aliases: []string{"dep"}, Short: "Manage dependencies between tasks"}
	depCmd.AddCommand(newDependencyAddCommand(func() *sql.DB { return db }))
	depCmd.AddCommand(newDependencyRemoveCommand(func() *sql.DB { return db }))
	depCmd.AddCommand(newDependencyListCommand(func() *sql.DB { return db }))
	root.AddCommand(depCmd)
	taskCmd.AddCommand(newTaskStateListCommand("ready", "List tasks eligible to start", func(ctx context.Context, p *service.Planner, id int64) ([]service.PlannedTask, error) {
		return p.Ready(ctx, id)
	}, false, func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskStateListCommand("blocked", "List tasks blocked by dependencies", func(ctx context.Context, p *service.Planner, id int64) ([]service.PlannedTask, error) {
		return p.Blocked(ctx, id)
	}, true, func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskStateListCommand("overdue", "List overdue tasks", func(ctx context.Context, p *service.Planner, id int64) ([]service.PlannedTask, error) {
		return p.Overdue(ctx, id, time.Now())
	}, false, func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskStatusCommand(func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskTransitionCommand("start", "Start a task", func(ctx context.Context, s *service.TaskService, id int64) error { return s.Start(ctx, id) }, func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskTransitionCommand("complete", "Complete a task", func(ctx context.Context, s *service.TaskService, id int64) error { return s.Complete(ctx, id) }, func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskTransitionCommand("cancel", "Cancel a task", func(ctx context.Context, s *service.TaskService, id int64) error { return s.Cancel(ctx, id) }, func() *sql.DB { return db }))
	root.AddCommand(taskCmd)

	root.AddCommand(newProjectPlanCommand("plan", "Create a plan", false, func() *sql.DB { return db }))
	root.AddCommand(newProjectPlanCommand("report", "Project report", true, func() *sql.DB { return db }))
	return root
}

func newProjectCreateCommand(getDB dbGetter) *cobra.Command {
	var description string
	cmd := &cobra.Command{
		Use: "create <name>", Short: "Create new project", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := &domain.Project{Name: args[0], Description: description, Status: domain.ProjectActive}
			if err := repository.NewProjectRepository(getDB()).Create(cmd.Context(), p); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created project #%d: %s\n", p.ID, p.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "project description")
	return cmd
}

func newProjectShowCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{Use: "show <project-id>", Short: "View project information", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			p, err := repository.NewProjectRepository(getDB()).Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ID: %d\nName: %s\nDescription: %s\nStatus: %s\nCreated: %s\nUpdated: %s\n", p.ID, p.Name, p.Description, p.Status, p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339))
			if p.ArchivedAt != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Archived: %s\n", p.ArchivedAt.Format(time.RFC3339))
			}
			return nil
		},
	}
}

func newProjectUpdateCommand(getDB dbGetter) *cobra.Command {
	var name, description string
	cmd := &cobra.Command{Use: "update <project-id>", Short: "Update project name and/or description", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			repo := repository.NewProjectRepository(getDB())
			p, err := repo.Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("name") {
				p.Name = name
			}
			if cmd.Flags().Changed("description") {
				p.Description = description
			}
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("description") {
				return fmt.Errorf("need --name to --description")
			}
			if err := repo.Update(cmd.Context(), p); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated project #%d.\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project's new name")
	cmd.Flags().StringVar(&description, "description", "", "project's new description")
	return cmd
}

func newProjectStatusCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{
		Use: "set-status <project-id> <active|completed|closed|archived>", Short: "Update project status", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			status := domain.ProjectStatus(args[1])
			if err := service.NewProjectService(getDB()).SetStatus(cmd.Context(), id, status); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Project #%d changed to status %s.\n", id, status)
			return nil
		},
	}
}

func newTaskCreateCommand(getDB dbGetter) *cobra.Command {
	var description, priority, deadline string
	var parentID int64
	var required bool
	cmd := &cobra.Command{
		Use: "create <project-id> <title>", Short: "Create task or subtask", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			t := &domain.Task{ProjectID: projectID, Title: args[1], Description: description, Priority: domain.Priority(priority), IsRequired: required}
			if parentID > 0 {
				t.ParentID = &parentID
			} else if parentID < 0 {
				return fmt.Errorf("parent ID must be a positive integer")
			}
			if deadline != "" {
				d, e := time.Parse("2006-01-02", deadline)
				if e != nil {
					return fmt.Errorf("deadline must be in YYYY-MM-DD")
				}
				d = d.UTC()
				t.Deadline = &d
			}
			if err := repository.NewTaskRepository(getDB()).Create(cmd.Context(), t); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Creted task #%d in project #%d: %s\n", t.ID, t.ProjectID, t.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "task description")
	cmd.Flags().StringVar(&priority, "priority", "medium", "priority: low, medium, high, urgent")
	cmd.Flags().StringVar(&deadline, "deadline", "", "completion due date (YYYY-MM-DD)")
	cmd.Flags().Int64Var(&parentID, "parent", 0, "parent task ID for creating subtask")
	cmd.Flags().BoolVar(&required, "required", true, "mark task as required")
	return cmd
}

func newTaskUpdateCommand(getDB dbGetter) *cobra.Command {
	var title, description, priority, deadline string
	var required bool
	cmd := &cobra.Command{Use: "update <task-id>", Short: "Update task information", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "task ID")
			if err != nil {
				return err
			}
			repo := repository.NewTaskRepository(getDB())
			t, err := repo.Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			changed := false
			if cmd.Flags().Changed("title") {
				t.Title = title
				changed = true
			}
			if cmd.Flags().Changed("description") {
				t.Description = description
				changed = true
			}
			if cmd.Flags().Changed("priority") {
				t.Priority = domain.Priority(priority)
				changed = true
			}
			if cmd.Flags().Changed("deadline") {
				if deadline == "" {
					t.Deadline = nil
				} else {
					d, e := time.Parse("2006-01-02", deadline)
					if e != nil {
						return fmt.Errorf("deadline must be in YYYY-MM-DD")
					}
					d = d.UTC()
					t.Deadline = &d
				}
				changed = true
			}
			if cmd.Flags().Changed("required") {
				t.IsRequired = required
				changed = true
			}
			if !changed {
				return fmt.Errorf("need at least 1 update flag")
			}
			if err := repo.Update(cmd.Context(), t); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated task #%d.\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "task's new title")
	cmd.Flags().StringVar(&description, "description", "", "task's new description")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: low, medium, high, urgent")
	cmd.Flags().StringVar(&deadline, "deadline", "", "new due date (YYYY-MM-DD); empty for deadline removal")
	cmd.Flags().BoolVar(&required, "required", false, "task is required to complete or not")
	return cmd
}

func newTaskDeleteCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{Use: "delete <task-id>", Short: "Delete a task", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "task ID")
			if err != nil {
				return err
			}
			if err := repository.NewTaskRepository(getDB()).Delete(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Đã xóa task #%d.\n", id)
			return nil
		},
	}
}

func newDependencyAddCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{Use: "add <task-id> <prerequisite-task-id>", Short: "Add dependency (first task depends second task)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, err := positiveID(args[0], "task ID")
			if err != nil {
				return err
			}
			prereqID, err := positiveID(args[1], "prerequisite task ID")
			if err != nil {
				return err
			}
			if err := repository.NewDependencyRepository(getDB()).Add(cmd.Context(), taskID, prereqID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added dependency: task #%d depends task #%d.\n", taskID, prereqID)
			return nil
		},
	}
}

func newDependencyRemoveCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{Use: "remove <task-id> <prerequisite-task-id>", Short: "Delete dependency", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, err := positiveID(args[0], "task ID")
			if err != nil {
				return err
			}
			prereqID, err := positiveID(args[1], "prerequisite task ID")
			if err != nil {
				return err
			}
			if err := repository.NewDependencyRepository(getDB()).Remove(cmd.Context(), taskID, prereqID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted dependency between task #%d and task #%d.\n", taskID, prereqID)
			return nil
		},
	}
}

func newDependencyListCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{Use: "list <project-id>", Short: "List dependencies of a project", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			items, err := repository.NewDependencyRepository(getDB()).ListByProject(cmd.Context(), projectID)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Project has no dependencies.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "TASK_ID\tDEPENDS_ON_ID")
			for _, d := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%d\n", d.TaskID, d.DependsOnID)
			}
			return nil
		},
	}
}

type dbGetter func() *sql.DB

type plannedQuery func(context.Context, *service.Planner, int64) ([]service.PlannedTask, error)

func newTaskListCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{
		Use: "list <project-id>", Short: "List tasks of a project", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			ctx, db := cmd.Context(), getDB()
			items, err := repository.NewTaskRepository(db).ListByProject(ctx, id)
			if err != nil {
				return err
			}
			states := service.NewTaskService(db)
			fmt.Fprintln(cmd.OutOrStdout(), "ID\tSTATUS\tPRIORITY\tDEADLINE\tTITLE")
			for _, t := range items {
				state, err := states.GetState(ctx, t.ID)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%s\n", t.ID, state.Status, t.Priority, deadlineText(t.Deadline), t.Title)
			}
			return nil
		},
	}
}

func newTaskStateListCommand(use, short string, query plannedQuery, showBlockers bool, getDB dbGetter) *cobra.Command {
	return &cobra.Command{
		Use: use + " <project-id>", Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			items, err := query(cmd.Context(), service.NewPlanner(getDB()), id)
			if err != nil {
				return err
			}
			printPlanned(cmd.OutOrStdout(), items, showBlockers)
			return nil
		},
	}
}

func newProjectPlanCommand(use, short string, report bool, getDB dbGetter) *cobra.Command {
	return &cobra.Command{
		Use: use + " <project-id>", Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "project ID")
			if err != nil {
				return err
			}
			p := service.NewPlanner(getDB())
			if report {
				r, err := p.Report(cmd.Context(), id, time.Now())
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "Project: %s (#%d)\nStatus: %s\nTasks: %d\n", r.ProjectName, r.ProjectID, r.ProjectStatus, r.TotalTasks)
				fmt.Fprintf(out, "Done: %d (%.1f%%)\nTodo: %d\nIn progress: %d\nBlocked: %d\nCancelled: %d\nOverdue: %d\n", r.DoneTasks, r.CompletionPct, r.TodoTasks, r.InProgressTasks, r.BlockedTasks, r.CancelledTasks, r.OverdueTasks)
				fmt.Fprintf(out, "Required tasks completed: %d/%d\nCan complete project: %t\n", r.RequiredDone, r.RequiredTasks, r.CanComplete)
				return nil
			}
			items, err := p.Plan(cmd.Context(), id)
			if err != nil {
				return err
			}
			printPlanned(cmd.OutOrStdout(), items, false)
			return nil
		},
	}
}

func newTaskStatusCommand(getDB dbGetter) *cobra.Command {
	return &cobra.Command{
		Use: "status <task-id>", Short: "View task status", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "task ID")
			if err != nil {
				return err
			}
			state, err := service.NewTaskService(getDB()).GetState(cmd.Context(), id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Task #%d: %s\nStatus: %s\nPriority: %s\nBlocking task IDs: %v\n", id, state.Task.Title, state.Status, state.Task.Priority, state.BlockingTaskIDs)
			return nil
		},
	}
}

func newTaskTransitionCommand(use, short string, transition func(context.Context, *service.TaskService, int64) error, getDB dbGetter) *cobra.Command {
	return &cobra.Command{
		Use: use + " <task-id>", Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "task ID")
			if err != nil {
				return err
			}
			if err := transition(cmd.Context(), service.NewTaskService(getDB()), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Task #%d updated successfully.\n", id)
			return nil
		},
	}
}

func positiveID(value, label string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be apossitive integer", label)
	}
	return id, nil
}

func deadlineText(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02")
}

func printPlanned(out io.Writer, items []service.PlannedTask, showBlockers bool) {
	if len(items) == 0 {
		fmt.Fprintln(out, "No suitable tasks.")
		return
	}
	fmt.Fprintln(out, "ID\tSTATUS\tPRIORITY\tDEADLINE\tTITLE")
	for _, item := range items {
		fmt.Fprintf(out, "%d\t%s\t%s\t%s\t%s", item.Task.ID, item.Status, item.Task.Priority, deadlineText(item.Task.Deadline), item.Task.Title)
		if showBlockers {
			fmt.Fprintf(out, "\tblocked by: %v", item.BlockingTaskIDs)
		}
		if item.Overdue {
			fmt.Fprint(out, "\tOVERDUE")
		}
		fmt.Fprintln(out)
	}
}