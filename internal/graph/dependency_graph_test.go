package graph

import (
	"errors"
	"reflect"
	"testing"
)

func TestAddDependencyAndDetectCycle(t *testing.T) {
	g := NewDependencyGraph()
	for _, id := range []int64{1, 2, 3, 4} {
		if err := g.AddTask(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.AddDependency(2, 1); err != nil {
		t.Fatal(err)
	}
	if err := g.AddDependency(3, 2); err != nil {
		t.Fatal(err)
	}
	if g.HasCycle() {
		t.Fatal("acyclic graph reported a cycle")
	}
	if err := g.AddDependency(1, 3); !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
	if g.HasCycle() {
		t.Fatal("rejected edge must not mutate graph")
	}
}

func TestSelfDependencyAndInvalidIDs(t *testing.T) {
	g := NewDependencyGraph()
	if err := g.AddDependency(1, 1); !errors.Is(err, ErrSelfDependency) {
		t.Fatalf("expected ErrSelfDependency, got %v", err)
	}
	if err := g.AddTask(0); !errors.Is(err, ErrInvalidTaskID) {
		t.Fatalf("expected ErrInvalidTaskID, got %v", err)
	}
}

func TestTopologicalOrderPrerequisitesFirst(t *testing.T) {
	g := NewDependencyGraph()
	for _, id := range []int64{1, 2, 3, 4} {
		_ = g.AddTask(id)
	}
	_ = g.AddDependency(3, 1)
	_ = g.AddDependency(3, 2)
	_ = g.AddDependency(4, 3)
	order, err := g.TopologicalOrder()
	if err != nil {
		t.Fatal(err)
	}
	positions := make(map[int64]int)
	for i, id := range order {
		positions[id] = i
	}
	constraints := [][2]int64{{3, 1}, {3, 2}, {4, 3}}
	for _, pair := range constraints {
		task, prerequisite := pair[0], pair[1]
		if positions[prerequisite] >= positions[task] {
			t.Fatalf("prerequisite %d should precede task %d in %v", prerequisite, task, order)
		}
	}
	if len(order) != 4 {
		t.Fatalf("expected all tasks in order, got %v", order)
	}
}

func TestDependenciesSorted(t *testing.T) {
	g := NewDependencyGraph()
	_ = g.AddDependency(3, 2)
	_ = g.AddDependency(2, 1)
	want := []Dependency{{TaskID: 2, DependsOnID: 1}, {TaskID: 3, DependsOnID: 2}}
	if got := g.Dependencies(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Dependencies() = %#v, want %#v", got, want)
	}
}