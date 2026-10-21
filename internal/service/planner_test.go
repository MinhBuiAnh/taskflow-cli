package service

import (
	"testing"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
)

func TestSortPlannedUsesPriorityThenDeadline(t *testing.T) {
	late := time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)
	early := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	items := []PlannedTask{
		{TaskState: TaskState{Task: domain.Task{ID: 1, Priority: domain.PriorityMedium, Deadline: &early}}},
		{TaskState: TaskState{Task: domain.Task{ID: 2, Priority: domain.PriorityUrgent, Deadline: &late}}},
		{TaskState: TaskState{Task: domain.Task{ID: 3, Priority: domain.PriorityHigh, Deadline: &late}}},
		{TaskState: TaskState{Task: domain.Task{ID: 4, Priority: domain.PriorityHigh, Deadline: &early}}},
	}
	sortPlanned(items)
	want := []int64{2, 4, 3, 1}
	for i, id := range want {
		if items[i].Task.ID != id {
			t.Fatalf("position %d: got task %d, want %d", i, items[i].Task.ID, id)
		}
	}
}

func TestSortPlannedNoDeadlineAfterDeadline(t *testing.T) {
	items := []PlannedTask{
		{TaskState: TaskState{Task: domain.Task{ID: 1, Priority: domain.PriorityLow}}},
		{TaskState: TaskState{Task: domain.Task{ID: 2, Priority: domain.PriorityLow, Deadline: timePtr(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))}}},
	}
	sortPlanned(items)
	if items[0].Task.ID != 2 || items[1].Task.ID != 1 {
		t.Fatalf("unexpected order: %d, %d", items[0].Task.ID, items[1].Task.ID)
	}
}

func timePtr(t time.Time) *time.Time { return &t }