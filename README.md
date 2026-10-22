# TaskFlow CLI

**TaskFlow** is a command-line tool written in Go for projects management, tasks, subtasks and dependencies. Unlike traditional todo list, TaskFlow checks business condition before allowing task beginning or completion, detect dependency cycle and supports planning with appropiate working order.

## Functions

- Project Management: create, view, list, update information and change status.
- Task/Subtask Management: create, list, update, delete non-referenced task.
- Task Details: description, priority (`low`, `medium`, `high`, `urgent`), deadline and required flag.
- Dependency: add, list, remove dependency; reject non-existant task, duplicate dependencies, different projects, self-reference or create cycle.
- Validity status: `blocked` is inferred from uncomplete dependency, does not save as a separate status in the task table.
- Transtion status through business services: no blocked task beginning/completion; no task status transition in inactive project.
- Planning by topological order, ready/blocked/overdue task lookup and progress report.
- SQLite with migrations embedded in binary; data is resided on user's device.
- CLI is built with [Cobra](https://github.com/spf13/cobra).

## Prequisites

- Go 1.23 or newer.
- Internet in the first build if Go modules are not in cache.

Main dependencies: `github.com/spf13/cobra` và `modernc.org/sqlite` (Go-pure SQLite).

## Installation and Run

```bash
git clone <URL-repository>
cd taskflow
go mod tidy
go run . --help
```

TaskFlow by default create/use `taskflow.db` file at the current directory. Use `--db`to select a different database:

```bash
go run . --db ./data/demo.db project list
```

Every data-related command opens SQLite themselves and runs unapplied migrations. Help command does not need to initialize database.

## Quick Start: example workflow

The following examples run independently with `demo.db` database. ID is generated automatically; if the database already has data, use IDs returned from the query.

### 1. Create projects and tasks

```bash
go run . --db demo.db project create "Website launch" --description "Phát hành website phiên bản đầu"
go run . --db demo.db project list
go run . --db demo.db project show 1

go run . --db demo.db task create 1 "Finalize requirements" --priority high --deadline 2027-01-10
go run . --db demo.db task create 1 "Implement API" --priority urgent --deadline 2027-01-20
go run . --db demo.db task create 1 "Write documentation" --required=false
```

Create subtask by adding `--parent <task-id>` behind `task create`. By default new task is required to complete and `medium` priority.

### 2. Dependency declaration

```bash
go run . --db demo.db dependency add 2 1
go run . --db demo.db dependency list 1
go run . --db demo.db task blocked 1
```

Syntax `dependency add <task-id> <prerequisite-task-id>` means the first task depends the second task. The above example means `Implement API` is only eligible to start after `Finalize requirements` completed. If adding dependency creates cycle, TaskFlow will reject the operation.

### 3. Working with tasks

```bash
go run . --db demo.db task ready 1
go run . --db demo.db task start 1
go run . --db demo.db task complete 1
go run . --db demo.db task start 2
go run . --db demo.db task status 2
go run . --db demo.db task complete 2
```

If the task is blocked, `start` or `complete` returns error and does not record failed status transition into the database. Complete prerequisite tasks first then retry.

### 4. Planning and Report

```bash
go run . --db demo.db plan 1
go run . --db demo.db task overdue 1
go run . --db demo.db report 1
```

Report consists of total number of tasks, completion ratio, number of tasks by status, number of overdue tasks and required tasks completion ratio.

### 5. Update or archive

```bash
go run . --db demo.db project update 1 --name "Website v1 launch"
go run . --db demo.db task update 3 --priority low --required=false
go run . --db demo.db dependency remove 2 1
go run . --db demo.db project set-status 1 archived
```

To delete a task, use `task delete <task-id>`. SQLite will reject deleting task being referenced by dependency or subtask; remove all relations/references first. Set project status to `completed` if only every required tasks is completed according to business report.

## List of Commands

| Command | Description |
|---|---|
| `project create <name>` | Create project; supports `--description` |
| `project list` | List projects |
| `project show <project-id>` | View project information |
| `project update <project-id>` | Update `--name` and/or `--description` |
| `project set-status <project-id> <status>` | Change project status: `active`, `completed`, `closed`, `archived` |
| `task create <project-id> <title>` | Create task; supports `--description`, `--priority`, `--deadline`, `--parent`, `--required` |
| `task list <project-id>` | List tasks and validity status |
| `task update <task-id>` | Update titile, description, priority, deadline, required task flag |
| `task delete <task-id>` | Delete non-referenced task |
| `task ready <project-id>` | List eligible tasks |
| `task blocked <project-id>` | List blocked tasks and their uncompleted prerequisites |
| `task overdue <project-id>` | List overdue, uncompleted and uncancelled tasks |
| `task status <task-id>` | View task status and blocking tasks |
| `task start <task-id>` | Start task if eligible |
| `task complete <task-id>` | Complete task if eligible |
| `task cancel <task-id>` | Cancel task |
| `dependency add <task-id> <prerequisite-id>` | Add dependency |
| `dependency remove <task-id> <prerequisite-id>` | Remove dependency |
| `dependency list <project-id>` | List dependencies of a project |
| `plan <project-id>` | Plan a project by its dependency |
| `report <project-id>` | Project report |

You can look up for help in every command, e.g. `go run . task --help` or `go run . dependency add --help`.

## Business Logic

1. **Dependency:** if B depends A, B is eligible to start only if A has status `done`.
2. **No dependency cycle:** dependency graph is validated before adding a new edge; dependencies that create cycle are rejected.
3. **Status `blocked`:** a status inferred from uncompleted prerequisites. Task status is still `todo`.
4. **Status transitioning:** `Start`, `Complete` và `Cancel` go through business services; not only through CLI validation.
5. **Project completion:** project can only be changed to `completed` when all required tasks are completed. Non-required tasks can be left uncompleted.
6. **Task deletion:** foreign key prevents deleting task when it's referenced by other tasks; this operation does not automatically delete related tasks.

## Testing and Code Formatting

```bash
go test ./...
go test -race ./...
gofmt -w main.go $(find internal -name '*.go')
```

Or using Makefile:

```bash
make test
make fmt
make tidy
```

The tests consist of dependency graph, repository/migration with temporary SQLite, status transitioning service, planner/report and CLI Workflow with Cobra. With environments that don't have dependencies yet, run `go mod tidy` with internet connection before testing. DO NOT consider successful build as a proof of successful testing.

## Project Structure

```text
taskflow/
├── .github/workflows/test.yml
├── internal/
│   ├── cli/          # Cobra commands and CLI integration tests
│   ├── domain/       # Project, Task, Dependency and validation
│   ├── graph/        # Directed graph, cycle detection, topological order
│   ├── repository/   # SQLite, migration runner, CRUD and dependency persistence
│   │   └── migrations/ # Migrations embedded in binary when building
│   └── service/      # Project/task state rules, planner and reports
├── .gitignore
├── LICENSE
├── Makefile
├── README.md
├── go.mod
└── main.go
```

## Licenses

This project is published with the MIT License; see [LICENSE](LICENSE).