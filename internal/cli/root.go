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
		Long:          "TaskFlow is a CLI tool for project management with dependencies, planning and progress report.",
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
	root.PersistentFlags().StringVar(&dbPath, "db", "taskflow.db", "filepath for SQLite file")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return err })
	cobra.OnFinalize(func() {
		if db != nil {
			_ = db.Close()
		}
	})

	projectCmd := &cobra.Command{Use: "project", Short: "Manage projects"}
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

	taskCmd := &cobra.Command{Use: "task", Short: "Lookup and transition task status"}
	taskCmd.AddCommand(newTaskListCommand(func() *sql.DB { return db }))
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
	taskCmd.AddCommand(newTaskTransitionCommand("start", "Start eligible task", func(ctx context.Context, s *service.TaskService, id int64) error { return s.Start(ctx, id) }, func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskTransitionCommand("complete", "Complete eligible task", func(ctx context.Context, s *service.TaskService, id int64) error { return s.Complete(ctx, id) }, func() *sql.DB { return db }))
	taskCmd.AddCommand(newTaskTransitionCommand("cancel", "Cancel task", func(ctx context.Context, s *service.TaskService, id int64) error { return s.Cancel(ctx, id) }, func() *sql.DB { return db }))
	root.AddCommand(taskCmd)

	root.AddCommand(newProjectPlanCommand("plan", "Plan task order sorted by dependencies", false, func() *sql.DB { return db }))
	root.AddCommand(newProjectPlanCommand("report", "Report project progress", true, func() *sql.DB { return db }))
	return root
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
		Use: "status <task-id>", Short: "List current status of a task", Args: cobra.ExactArgs(1),
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
		return 0, fmt.Errorf("%s must be a positive integer", label)
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
		fmt.Fprintln(out, "No tasks yet.")
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