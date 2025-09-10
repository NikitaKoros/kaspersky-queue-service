package queue

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestQueue_EnqueueDequeue(t *testing.T) {
	q := NewQueue(2)

	task1 := &Task{ID: "1", Payload: "hello"}
	task2 := &Task{ID: "2", Payload: "world"}

	err := q.Enqueue(task1)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	err = q.Enqueue(task2)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	ctx := context.Background()
	got1, ok := q.Dequeue(ctx)
	if !ok || got1.ID != "1" {
		t.Errorf("Expected task ID '1', got %v, ok=%v", got1, ok)
	}

	got2, ok := q.Dequeue(ctx)
	if !ok || got2.ID != "2" {
		t.Errorf("Expected task ID '2', got %v, ok=%v", got2, ok)
	}
}

func TestQueue_QueueFull(t *testing.T) {
	q := NewQueue(1)

	task1 := &Task{ID: "1", Payload: "first"}
	task2 := &Task{ID: "2", Payload: "second"}

	err := q.Enqueue(task1)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	err = q.Enqueue(task2)
	if err != ErrQueueFull {
		t.Errorf("Expected ErrQueueFull, got %v", err)
	}
}

func TestQueue_DequeueWithTimeout(t *testing.T) {
	q := NewQueue(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	task, ok := q.Dequeue(ctx)
	if ok {
		t.Error("Expected no task (queue empty), but got one")
	}
	if task != nil {
		t.Errorf("Expected nil task, got %v", task)
	}
}

func TestQueue_SizeAndCapacity(t *testing.T) {
	q := NewQueue(3)

	if q.Capacity() != 3 {
		t.Errorf("Expected capacity 3, got %d", q.Capacity())
	}

	if q.Size() != 0 {
		t.Errorf("Expected size 0 for empty queue, got %d", q.Size())
	}

	task := &Task{ID: "1", Payload: "test"}
	q.Enqueue(task)

	if q.Size() != 1 {
		t.Errorf("Expected size 1 after enqueue, got %d", q.Size())
	}
}

func TestQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	q := NewQueue(100)
	const N = 50

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		for i := range N {
			task := &Task{ID: fmt.Sprintf("task-%d", i), Payload: "data"}
			for {
				err := q.Enqueue(task)
				if err == nil {
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	count := 0
	for count < N {
		task, ok := q.Dequeue(ctx)
		if !ok {
			if ctx.Err() != nil {
				t.Fatal("Dequeue context canceled before receiving all tasks")
			}
			continue
		}
		if task == nil {
			t.Fatal("Received nil task")
		}
		count++
	}

	if count != N {
		t.Errorf("Expected %d tasks processed, got %d", N, count)
	}
}
