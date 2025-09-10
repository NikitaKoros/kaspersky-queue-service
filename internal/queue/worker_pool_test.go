package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/NikitaKoros/queue-service/internal/store"
)

type MockQueue struct {
	tasks    chan *Task
	capacity int
	mu       sync.Mutex
}

func NewMockQueue(capacity int) *MockQueue {
	return &MockQueue{
		tasks:    make(chan *Task, capacity),
		capacity: capacity,
	}
}

func (mq *MockQueue) Enqueue(task *Task) error {
	select {
	case mq.tasks <- task:
		return nil
	default:
		return ErrQueueFull
	}
}

func (mq *MockQueue) Dequeue(ctx context.Context) (*Task, bool) {
	select {
	case task := <-mq.tasks:
		return task, true
	case <-ctx.Done():
		return nil, false
	}
}

func (mq *MockQueue) Size() int {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return len(mq.tasks)
}

func (mq *MockQueue) Capacity() int {
	return mq.capacity
}

type MockStore struct {
	tasks map[string]store.TaskRecord
	mu    sync.RWMutex
}

func NewMockStore() *MockStore {
	return &MockStore{
		tasks: make(map[string]store.TaskRecord),
	}
}

func (ms *MockStore) CreateTask(id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.tasks[id]; exists {
		return store.ErrTaskExists
	}
	ms.tasks[id] = store.TaskRecord{State: store.TaskStateQueued}
	return nil
}

func (ms *MockStore) DeleteTask(id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.tasks[id]; !exists {
		return fmt.Errorf("%w: id = %s", store.ErrTaskNotFound, id)
	}
	delete(ms.tasks, id)
	return nil
}

func (ms *MockStore) UpdateTaskState(id string, state store.TaskState, reason string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.tasks[id]; !exists {
		return fmt.Errorf("%w: id = %s", store.ErrTaskNotFound, id)
	}
	ms.tasks[id] = store.TaskRecord{State: state, Reason: reason}
	return nil
}

func (ms *MockStore) GetStats() map[store.TaskState]int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	stats := make(map[store.TaskState]int)
	for _, rec := range ms.tasks {
		stats[rec.State]++
	}
	return stats
}

type testLogWriter struct {
	mu   *sync.Mutex
	logs *[]string
}

func (w *testLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	*w.logs = append(*w.logs, msg)
	return len(p), nil
}

func TestWorkerPool_StartStop(t *testing.T) {
	var logs []string
	var logMu sync.Mutex
	log.SetOutput(&testLogWriter{mu: &logMu, logs: &logs})
	defer func() {
		log.SetOutput(log.Writer())
	}()

	queue := NewMockQueue(10)
	store := NewMockStore()

	wp := NewWorkerPool(queue, store, 2, 10*time.Millisecond, 100*time.Millisecond)

	wp.Start()
	time.Sleep(50 * time.Millisecond)
	wp.Stop(200 * time.Millisecond)

	select {
	case <-wp.ctx.Done():
	default:
		t.Error("Expected context to be cancelled after Stop()")
	}
}

func TestWorkerPool_ProcessesTaskSuccessfully(t *testing.T) {
	var logs []string
	var logMu sync.Mutex
	log.SetOutput(&testLogWriter{mu: &logMu, logs: &logs})
	defer func() {
		log.SetOutput(log.Writer())
	}()

	mockQueue := NewMockQueue(10)
	mockStore := NewMockStore()

	wp := NewWorkerPool(mockQueue, mockStore, 1, 10*time.Millisecond, 100*time.Millisecond)

	task := &Task{
		ID:         "task1",
		Payload:    "test",
		MaxRetries: 3,
		Attemps:    0,
	}

	err := mockStore.CreateTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to create task in store: %v", err)
	}

	err = mockQueue.Enqueue(task)
	if err != nil {
		t.Fatalf("Failed to enqueue task: %v", err)
	}

	wp.Start()
	time.Sleep(600 * time.Millisecond)
	wp.Stop(200 * time.Millisecond)

	mockStore.mu.RLock()
	record, exists := mockStore.tasks["task1"]
	mockStore.mu.RUnlock()

	if !exists {
		t.Fatal("Task not found in store")
		t.Logf("Logs: %+v", logs)
	}

	if record.State != store.TaskStateDone {
		t.Errorf("Expected task state 'done', got %s", record.State)
		t.Logf("Logs: %+v", logs)
	}
}

func TestWorkerPool_RetriesFailedTask(t *testing.T) {
	var logs []string
	var logMu sync.Mutex
	log.SetOutput(&testLogWriter{mu: &logMu, logs: &logs})
	defer func() {
		log.SetOutput(log.Writer())
	}()

	oldRand := randFloat32
	randFloat32 = func() float32 { return 0.1 }
	defer func() { randFloat32 = oldRand }()

	mockQueue := NewMockQueue(10)
	mockStore := NewMockStore()

	wp := NewWorkerPool(mockQueue, mockStore, 1, 10*time.Millisecond, 100*time.Millisecond)

	task := &Task{
		ID:         "task1",
		Payload:    "test",
		MaxRetries: 1,
		Attemps:    0,
	}

	err := mockStore.CreateTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to create task in store: %v", err)
	}

	mockQueue.Enqueue(task)

	wp.Start()
	time.Sleep(800 * time.Millisecond)
	wp.Stop(500 * time.Millisecond)

	mockStore.mu.RLock()
	record, exists := mockStore.tasks["task1"]
	mockStore.mu.RUnlock()

	if !exists {
		t.Fatal("Task not found in store")
		t.Logf("Logs: %+v", logs)
	}

	if record.State != store.TaskStateFailed {
		t.Errorf("Expected task state 'failed', got %s", record.State)
		t.Logf("Logs: %+v", logs)
	}
}

func TestWorkerPool_RespectsContextCancellation(t *testing.T) {
	var logs []string
	var logMu sync.Mutex
	log.SetOutput(&testLogWriter{mu: &logMu, logs: &logs})
	defer func() {
		log.SetOutput(log.Writer())
	}()

	mockQueue := NewMockQueue(10)
	mockStore := NewMockStore()

	wp := NewWorkerPool(mockQueue, mockStore, 1, 10*time.Millisecond, 100*time.Millisecond)

	task := &Task{
		ID:         "longtask",
		Payload:    "slow",
		MaxRetries: 0,
		Attemps:    0,
	}

	err := mockStore.CreateTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to create task in store: %v", err)
	}

	mockQueue.Enqueue(task)

	wp.Start()
	time.Sleep(50 * time.Millisecond)

	wp.cancel()
	time.Sleep(100 * time.Millisecond)

	wp.Stop(200 * time.Millisecond)

	mockStore.mu.RLock()
	record, exists := mockStore.tasks["longtask"]
	mockStore.mu.RUnlock()

	if !exists {
		t.Fatal("Task not found in store")
		t.Logf("Logs: %+v", logs)
	}

	if record.State != store.TaskStateFailed {
		t.Errorf("Expected task state 'failed', got %s", record.State)
		t.Logf("Logs: %+v", logs)
	}

	if record.Reason != "cancelled during processing" {
		t.Errorf("Expected reason 'cancelled during processing', got %q", record.Reason)
		t.Logf("Logs: %+v", logs)
	}
}

func TestWorkerPool_CalculateBackoff(t *testing.T) {
	wp := &WorkerPool{
		baseBackoff: 10 * time.Millisecond,
		maxBackoff:  100 * time.Millisecond,
	}

	tests := []struct {
		attempts int
		expected time.Duration
	}{
		{1, 10 * time.Millisecond},
		{2, 20 * time.Millisecond},
		{3, 40 * time.Millisecond},
		{4, 80 * time.Millisecond},
		{5, 100 * time.Millisecond},
		{6, 100 * time.Millisecond},
	}

	for _, tt := range tests {
		got := wp.calculateBackoff(tt.attempts)
		if got != tt.expected {
			t.Errorf("calculateBackoff(%d) = %v; expected %v", tt.attempts, got, tt.expected)
		}
	}
}
