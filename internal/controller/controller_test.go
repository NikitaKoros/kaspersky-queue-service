package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NikitaKoros/queue-service/internal/queue"
	"github.com/NikitaKoros/queue-service/internal/store"
)

type MockQueue struct {
	enqueueErr error
	size       int
	capacity   int
	lastTask   *queue.Task
}

func (mq *MockQueue) Enqueue(task *queue.Task) error {
	mq.lastTask = task
	return mq.enqueueErr
}

func (mq *MockQueue) Dequeue(ctx context.Context) (*queue.Task, bool) {
	return nil, false
}

func (mq *MockQueue) Size() int {
	return mq.size
}

func (mq *MockQueue) Capacity() int {
	return mq.capacity
}

type MockStore struct {
	createErr error
	deleteErr error
	stats     map[store.TaskState]int
	tasks     map[string]struct{}
}

func NewMockStore() *MockStore {
	return &MockStore{
		stats: make(map[store.TaskState]int),
		tasks: make(map[string]struct{}),
	}
}

func (ms *MockStore) CreateTask(id string) error {
	if ms.createErr != nil {
		return ms.createErr
	}
	ms.tasks[id] = struct{}{}
	ms.stats[store.TaskStateQueued]++
	return nil
}

func (ms *MockStore) DeleteTask(id string) error {
	if ms.deleteErr != nil {
		return ms.deleteErr
	}
	if _, exists := ms.tasks[id]; !exists {
		return store.ErrTaskNotFound
	}
	delete(ms.tasks, id)
	ms.stats[store.TaskStateQueued]--
	return nil
}

func (ms *MockStore) UpdateTaskState(id string, state store.TaskState, reason string) error {
	return nil
}

func (ms *MockStore) GetStats() map[store.TaskState]int {
	return ms.stats
}

type MockShutdown struct {
	accepting bool
}

func (ms *MockShutdown) IsAccepting() bool {
	return ms.accepting
}

func (ms *MockShutdown) ShutdownWithTimeout(timeout time.Duration, fns ...func(context.Context)) {
}

func TestController_EnqueueTask_Success(t *testing.T) {
	mockQueue := &MockQueue{capacity: 100}
	mockStore := NewMockStore()
	shutdown := &MockShutdown{accepting: true}

	ctrl := NewController(mockQueue, mockStore, shutdown)

	req := EnqueueRequest{
		ID:         "task1",
		Payload:    "test payload",
		MaxRetries: 3,
	}

	taskID, err := ctrl.EnqueueTask(req)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if taskID != "task1" {
		t.Errorf("Expected task ID 'task1', got %s", taskID)
	}

	if _, exists := mockStore.tasks["task1"]; !exists {
		t.Error("Task not found in store after enqueue")
	}

	if mockQueue.lastTask == nil {
		t.Fatal("Expected task to be enqueued, but it was not")
	}

	if mockQueue.lastTask.ID != "task1" || mockQueue.lastTask.Payload != "test payload" || mockQueue.lastTask.MaxRetries != 3 {
		t.Errorf("Enqueued task has wrong data: %+v", mockQueue.lastTask)
	}
}

func TestController_EnqueueTask_TaskExists(t *testing.T) {
	mockQueue := &MockQueue{}
	mockStore := &MockStore{createErr: store.ErrTaskExists}
	shutdown := &MockShutdown{accepting: true}

	ctrl := NewController(mockQueue, mockStore, shutdown)

	req := EnqueueRequest{ID: "task1"}

	_, err := ctrl.EnqueueTask(req)

	if !errors.Is(err, store.ErrTaskExists) {
		t.Errorf("Expected ErrTaskExists, got %v", err)
	}

	if mockQueue.lastTask != nil {
		t.Errorf("Expected no task enqueued, but got %+v", mockQueue.lastTask)
	}
}

func TestController_EnqueueTask_QueueFull_Rollback(t *testing.T) {
	mockQueue := &MockQueue{enqueueErr: queue.ErrQueueFull}
	mockStore := NewMockStore()
	shutdown := &MockShutdown{accepting: true}

	ctrl := NewController(mockQueue, mockStore, shutdown)

	req := EnqueueRequest{ID: "task1"}

	_, err := ctrl.EnqueueTask(req)

	if !errors.Is(err, queue.ErrQueueFull) {
		t.Errorf("Expected ErrQueueFull, got %v", err)
	}

	if _, exists := mockStore.tasks["task1"]; exists {
		t.Error("Task should have been deleted from store after queue error")
	}
}

func TestController_EnqueueTask_QueueFull_RollbackFails(t *testing.T) {
	mockQueue := &MockQueue{enqueueErr: queue.ErrQueueFull}
	mockStore := NewMockStore()
	mockStore.deleteErr = errors.New("delete failed")
	shutdown := &MockShutdown{accepting: true}

	ctrl := NewController(mockQueue, mockStore, shutdown)

	req := EnqueueRequest{ID: "task1"}

	_, err := ctrl.EnqueueTask(req)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !errors.Is(err, queue.ErrQueueFull) {
		t.Errorf("Expected wrapped ErrQueueFull, got %v", err)
	}

	if !strings.Contains(err.Error(), "failed to delete task from store") {
		t.Errorf("Expected error to mention delete failure, got %q", err.Error())
	}
}

func TestController_GetHealthStats(t *testing.T) {
	mockQueue := &MockQueue{size: 5, capacity: 10}
	mockStore := NewMockStore()
	mockStore.stats = map[store.TaskState]int{
		store.TaskStateQueued:  3,
		store.TaskStateRunning: 2,
		store.TaskStateDone:    5,
	}
	shutdown := &MockShutdown{accepting: true}

	ctrl := NewController(mockQueue, mockStore, shutdown)

	taskStats, qStats := ctrl.GetHealthStats()

	if len(taskStats) != 3 {
		t.Errorf("Expected 3 task states, got %d", len(taskStats))
	}

	if taskStats[store.TaskStateQueued] != 3 {
		t.Errorf("Expected 3 queued tasks, got %d", taskStats[store.TaskStateQueued])
	}

	if qStats.Size != 5 {
		t.Errorf("Expected queue size 5, got %d", qStats.Size)
	}

	if qStats.Capacity != 10 {
		t.Errorf("Expected queue capacity 10, got %d", qStats.Capacity)
	}
}

func TestController_IsAccepting(t *testing.T) {
	shutdownAccepting := &MockShutdown{accepting: true}
	shutdownNotAccepting := &MockShutdown{accepting: false}

	ctrl1 := NewController(&MockQueue{}, NewMockStore(), shutdownAccepting)
	ctrl2 := NewController(&MockQueue{}, NewMockStore(), shutdownNotAccepting)

	if !ctrl1.IsAccepting() {
		t.Error("Expected IsAccepting() to return true")
	}

	if ctrl2.IsAccepting() {
		t.Error("Expected IsAccepting() to return false")
	}
}
