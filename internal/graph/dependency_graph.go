package graph

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrInvalidTaskID  = errors.New("task IDs must be positive")
	ErrSelfDependency = errors.New("a task cannot depend on itself")
	ErrCycle          = errors.New("dependency would create a cycle")
)

// Represents a task that depends on another task.
// TaskID cannot proceed until DependsOnID has completed.
type Dependency struct {
	TaskID      int64
	DependsOnID int64
}

// Stores edges from a task to its prerequisites.
type DependencyGraph struct {
	prerequisites map[int64]map[int64]struct{}
}

func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{prerequisites: make(map[int64]map[int64]struct{})}
}

// Registers a task even if it currently has no dependencies.
func (g *DependencyGraph) AddTask(taskID int64) error {
	if taskID <= 0 {
		return ErrInvalidTaskID
	}
	if g.prerequisites[taskID] == nil {
		g.prerequisites[taskID] = make(map[int64]struct{})
	}
	return nil
}

// Adds taskID -> dependsOnID after verifying that it does not introduce a cycle.
// A duplicate edge is treated as a no-op, no errors returned.
func (g *DependencyGraph) AddDependency(taskID, dependsOnID int64) error {
	if taskID <= 0 || dependsOnID <= 0 {
		return ErrInvalidTaskID
	}
	if taskID == dependsOnID {
		return ErrSelfDependency
	}
	if _, exists := g.prerequisites[taskID][dependsOnID]; exists {
		return nil
	}

	// Adding taskID -> dependsOnID forms a cycle exactly when dependsOnID
	// can already reach taskID by following prerequisite edges.
	if g.reachable(dependsOnID, taskID) {
		return fmt.Errorf("%w: task %d depends on task %d", ErrCycle, taskID, dependsOnID)
	}
	if g.prerequisites[taskID] == nil {
		g.prerequisites[taskID] = make(map[int64]struct{})
	}
	if g.prerequisites[dependsOnID] == nil {
		g.prerequisites[dependsOnID] = make(map[int64]struct{})
	}
	g.prerequisites[taskID][dependsOnID] = struct{}{}
	return nil
}

func (g *DependencyGraph) RemoveDependency(taskID, dependsOnID int64) {
	if deps := g.prerequisites[taskID]; deps != nil {
		delete(deps, dependsOnID)
	}
}

// Uses DFS to determine whether the graph has a cycle
func (g *DependencyGraph) HasCycle() bool {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[int64]int, len(g.prerequisites))
	var visit func(int64) bool
	visit = func(node int64) bool {
		state[node] = visiting
		for next := range g.prerequisites[node] {
			switch state[next] {
			case visiting:
				return true
			case unvisited:
				if visit(next) {
					return true
				}
			}
		}
		state[node] = visited
		return false
	}
	for node := range g.prerequisites {
		if state[node] == unvisited && visit(node) {
			return true
		}
	}
	return false
}

func (g *DependencyGraph) reachable(from, target int64) bool {
	seen := make(map[int64]bool)
	stack := []int64{from}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == target {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		for next := range g.prerequisites[n] {
			if !seen[next] {
				stack = append(stack, next)
			}
		}
	}
	return false
}

// Rreturns tasks in execution order: prerequisites first.
// It returns ErrCycle if the graph contains a cycle.
func (g *DependencyGraph) TopologicalOrder() ([]int64, error) {
	if g.HasCycle() {
		return nil, ErrCycle
	}
	// Reverse the stored prerequisite edges so Kahn's algorithm can start
	// with tasks that have no prerequisites.
	dependents := make(map[int64][]int64, len(g.prerequisites))
	indegree := make(map[int64]int, len(g.prerequisites))
	for task, deps := range g.prerequisites {
		if _, ok := indegree[task]; !ok {
			indegree[task] = 0
		}
		for dep := range deps {
			dependents[dep] = append(dependents[dep], task)
			indegree[task]++
			if _, ok := indegree[dep]; !ok {
				indegree[dep] = 0
			}
		}
	}
	ready := make([]int64, 0)
	for task, degree := range indegree {
		if degree == 0 {
			ready = append(ready, task)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	order := make([]int64, 0, len(indegree))
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)
		for _, dependent := range dependents[n] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
			}
		}
	}
	if len(order) != len(indegree) {
		return nil, ErrCycle
	}
	return order, nil
}

// Returns a stable, sorted snapshot of all edges.
func (g *DependencyGraph) Dependencies() []Dependency {
	out := make([]Dependency, 0)
	for taskID, deps := range g.prerequisites {
		for depID := range deps {
			out = append(out, Dependency{TaskID: taskID, DependsOnID: depID})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TaskID == out[j].TaskID {
			return out[i].DependsOnID < out[j].DependsOnID
		}
		return out[i].TaskID < out[j].TaskID
	})
	return out
}