package store

import (
	"fmt"
	"sync"
)

type TaskState string

var (
	TaskStateQueued  TaskState = "queued"
	TaskStateRunning TaskState = "running"
	TaskStateDone    TaskState = "done"
	TaskStateFailed  TaskState = "failed"
)

var (
	ErrTaskExists   = fmt.Errorf("task already exists")
	ErrTaskNotFound = fmt.Errorf("task not found")
)

type StoreProvider interface {
	CreateTask(string) error
	DeleteTask(string) error
	UpdateTaskState(string, TaskState, string) error
	GetStats() map[TaskState]int
}

type TaskRecord struct {
	State  TaskState
	Reason string
}

type MemStore struct {
	states map[string]TaskRecord
	mu     sync.RWMutex
}

func NewMemStore() *MemStore {
	return &MemStore{
		states: make(map[string]TaskRecord),
		mu:     sync.RWMutex{},
	}
}

func (s *MemStore) CreateTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.states[id]; exists {
		return ErrTaskExists
	}

	s.states[id] = TaskRecord{
		State:  TaskStateQueued,
		Reason: "",
	}
	return nil
}

func (s *MemStore) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.states[id]; !exists {
		return fmt.Errorf("%w: id = %s", ErrTaskNotFound, id)
	}

	delete(s.states, id)
	return nil
}

func (s *MemStore) UpdateTaskState(id string, newState TaskState, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.states[id]; !exists {
		return fmt.Errorf("%w: id = %s", ErrTaskNotFound, id)
	}

	s.states[id] = TaskRecord{
		State:  newState,
		Reason: reason,
	}
	return nil
}

func (s *MemStore) GetStats() map[TaskState]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[TaskState]int)
	for _, record := range s.states {
		stats[record.State]++
	}

	return stats
}
