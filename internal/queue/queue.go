package queue

import (
	"context"
	"fmt"
	"time"
)

var ErrQueueFull = fmt.Errorf("queue is full")

type Task struct {
	ID         string    `json:"id"`
	Payload    string    `json:"payload"`
	MaxRetries int       `json:"max_retries"`
	Attemps    int       `json:"-"`
	CreatedAt  time.Time `json:"-"`
}

type QueueProvider interface {
	Enqueue(task *Task) error
	Dequeue(ctx context.Context) (*Task, bool)
	Size() int
	Capacity() int
}

type Queue struct {
	ch   chan *Task
	size int
}

func NewQueue(size int) *Queue {
	return &Queue{
		ch:   make(chan *Task, size),
		size: size,
	}
}

func (q *Queue) Enqueue(task *Task) error {
	select {
	case q.ch <- task:
		return nil
	default:
		return ErrQueueFull
	}
}

func (q *Queue) Dequeue(ctx context.Context) (*Task, bool) {
	select {
	case task := <-q.ch:
		return task, true
	case <-ctx.Done():
		return nil, false
	}
}

func (q *Queue) Size() int {
	return len(q.ch)
}

func (q *Queue) Capacity() int {
	return q.size
}
