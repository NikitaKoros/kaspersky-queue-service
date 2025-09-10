package store

import (
	"sync"
	"testing"
)

func TestMemStore_CreateTask(t *testing.T) {
	store := NewMemStore()

	err := store.CreateTask("task1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	err = store.CreateTask("task1")
	if err != ErrTaskExists {
		t.Errorf("Expected ErrTaskExists, got %v", err)
	}
}

func TestMemStore_DeleteTask(t *testing.T) {
	store := NewMemStore()

	store.CreateTask("task1")

	err := store.DeleteTask("task1")
	if err != nil {
		t.Errorf("Expected no error when deleting existing task, got %v", err)
	}

	err = store.DeleteTask("task1")
	if err == nil {
		t.Error("Expected error when deleting non-existent task, got nil")
	}
	if err != nil && err.Error() != "task not found: id = task1" {
		t.Errorf("Expected 'task not found: id = task1', got %q", err.Error())
	}
}

func TestMemStore_UpdateTaskState(t *testing.T) {
	store := NewMemStore()

	store.CreateTask("task1")

	err := store.UpdateTaskState("task1", TaskStateRunning, "processing")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	stats := store.GetStats()
	if stats[TaskStateRunning] != 1 {
		t.Errorf("Expected 1 task in 'running', got %d", stats[TaskStateRunning])
	}

	err = store.UpdateTaskState("task1", TaskStateDone, "success")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	stats = store.GetStats()
	if stats[TaskStateDone] != 1 {
		t.Errorf("Expected 1 task in 'done', got %d", stats[TaskStateDone])
	}

	err = store.UpdateTaskState("nonexistent", TaskStateFailed, "error")
	if err == nil {
		t.Error("Expected error for non-existent task, got nil")
	}
	if err != nil && err.Error() != "task not found: id = nonexistent" {
		t.Errorf("Expected 'task not found: id = nonexistent', got %q", err.Error())
	}
}

func TestMemStore_GetStats(t *testing.T) {
	store := NewMemStore()

	stats := store.GetStats()
	if len(stats) != 0 {
		t.Errorf("Expected empty stats for empty store, got %v", stats)
	}

	store.CreateTask("task1")
	store.CreateTask("task2")
	store.UpdateTaskState("task1", TaskStateRunning, "")
	store.UpdateTaskState("task2", TaskStateDone, "")

	stats = store.GetStats()

	expected := map[TaskState]int{
		TaskStateQueued:  0,
		TaskStateRunning: 1,
		TaskStateDone:    1,
		TaskStateFailed:  0,
	}

	for state, exp := range expected {
		if got := stats[state]; got != exp {
			t.Errorf("For state %s: expected %d, got %d", state, exp, got)
		}
	}
}

func TestMemStore_ConcurrentAccess(t *testing.T) {
	store := NewMemStore()
	var wg sync.WaitGroup
	const n = 100

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "task" + string(rune('A'+i%26))
			store.CreateTask(id)
			store.UpdateTaskState(id, TaskStateRunning, "concurrent")
		}(i)
	}

	wg.Wait()

	stats := store.GetStats()
	if stats[TaskStateRunning] > 26 || stats[TaskStateRunning] == 0 {
		t.Errorf("Unexpected number of running tasks: %d", stats[TaskStateRunning])
	}
	if stats[TaskStateQueued] != 0 {
		t.Errorf("Expected 0 queued tasks after updates, got %d", stats[TaskStateQueued])
	}
}
