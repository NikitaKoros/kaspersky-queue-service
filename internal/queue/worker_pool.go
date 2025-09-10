package queue

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/NikitaKoros/queue-service/internal/store"
)

type WorkerPool struct {
	queue       QueueProvider
	store       store.StoreProvider
	workers     int
	baseBackoff time.Duration
	maxBackoff  time.Duration
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewWorkerPool(queue QueueProvider, store store.StoreProvider, workers int, baseBackoff, maxBackoff time.Duration) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		queue:       queue,
		store:       store,
		workers:     workers,
		baseBackoff: baseBackoff,
		maxBackoff:  maxBackoff,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (wp *WorkerPool) Start() {
	log.Printf("Starting worker pool with %d workers", wp.workers)

	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

func (wp *WorkerPool) Stop(timeout time.Duration) {
	log.Println("Stopping worker pool...")

	wp.cancel()

	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All workers stopped gracefully")
	case <-time.After(timeout):
		log.Println("Worker pool shutdown timeout reached")
	}
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	log.Printf("Worker %d started", id)

	for {
		select {
		case <-wp.ctx.Done():
			log.Printf("Worker %d stopping due to context cancellation", id)
			return
		default:
			task, ok := wp.queue.Dequeue(wp.ctx)
			if !ok {
				log.Printf("Worker %d stopping - no more tasks", id)
				return
			}

			wp.processTask(task, id)
		}
	}
}

var randFloat32 = rand.Float32

func (wp *WorkerPool) processTask(task *Task, workerID int) {
	wp.wg.Add(1)
	defer wp.wg.Done()

	log.Printf("Worker %d processing task %s (attempt %d)", workerID, task.ID, task.Attemps+1)

	wp.store.UpdateTaskState(task.ID, store.TaskStateRunning, "")

	processingTime := time.Duration(100+rand.Intn(400)) * time.Millisecond

	select {
	case <-time.After(processingTime):
	case <-wp.ctx.Done():
		wp.store.UpdateTaskState(task.ID, store.TaskStateFailed, "cancelled during processing")
		return
	}

	if randFloat32() < 0.2 {
		task.Attemps++
		errorMsg := fmt.Sprintf("simulated error on attempt %d", task.Attemps)
		log.Printf("Task %s failed: %s", task.ID, errorMsg)

		if task.Attemps < task.MaxRetries {
			wp.scheduleRetry(task, errorMsg, workerID)
		} else {
			wp.store.UpdateTaskState(task.ID, store.TaskStateFailed, errorMsg)
			log.Printf("Task %s failed permanently after %d attempts", task.ID, task.Attemps)
		}
	} else {
		wp.store.UpdateTaskState(task.ID, store.TaskStateDone, "")
		log.Printf("Worker %d completed task %s successfully", workerID, task.ID)
	}
}

func (wp *WorkerPool) scheduleRetry(task *Task, errorMsg string, workerID int) {
	wp.store.UpdateTaskState(task.ID, store.TaskStateQueued, errorMsg)

	backoff := wp.calculateBackoff(task.Attemps)

	jitteredBackoff := time.Duration(rand.Int63n(int64(backoff)))

	log.Printf("Worker %d scheduling retry for task %s after %v", workerID, task.ID, jitteredBackoff)

	go func() {
		select {
		case <-time.After(jitteredBackoff):
			select {
			case <-wp.ctx.Done():
				wp.store.UpdateTaskState(task.ID, store.TaskStateFailed, "cancelled during retry wait")
				return
			default:
			}

			if err := wp.queue.Enqueue(task); err != nil {
				log.Printf("Failed to re-enqueue task %s: %v", task.ID, err)
				wp.store.UpdateTaskState(task.ID, store.TaskStateFailed, fmt.Sprintf("retry failed: %v", err))
			}

		case <-wp.ctx.Done():
			wp.store.UpdateTaskState(task.ID, store.TaskStateFailed, "cancelled during retry wait")
		}
	}()
}

func (wp *WorkerPool) calculateBackoff(attempts int) time.Duration {
	backoff := wp.baseBackoff
	for i := 1; i < attempts; i++ {
		backoff *= 2
		if backoff > wp.maxBackoff {
			backoff = wp.maxBackoff
			break
		}
	}
	return backoff
}
