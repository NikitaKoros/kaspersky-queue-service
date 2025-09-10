package controller

import (
	"fmt"
	"time"

	"github.com/NikitaKoros/queue-service/internal/queue"
	"github.com/NikitaKoros/queue-service/internal/shutdown"
	"github.com/NikitaKoros/queue-service/internal/store"
)

type ControllerProvider interface {
	EnqueueTask(EnqueueRequest) (string, error)
	GetHealthStats() (map[store.TaskState]int, QueueStats)
	IsAccepting() bool
}

var (
	ErrTaskExists = store.ErrTaskExists
	ErrQueueFull  = queue.ErrQueueFull
)

type Controller struct {
	taskQueue   queue.QueueProvider
	statesStore store.StoreProvider
	shutdown    shutdown.CoordinatorProvider
}

type EnqueueRequest struct {
	ID         string `json:"id"`
	Payload    string `json:"payload"`
	MaxRetries int    `json:"max_retries"`
}

type EnqueueResponse struct {
	Message string `json:"message"`
	TaskID  string `json:"task_id"`
}

type QueueStats struct {
	Size     int `json:"size"`
	Capacity int `json:"capacity"`
}

type HealthResponse struct {
	Status string                  `json:"status"`
	Stats  map[store.TaskState]int `json:"stats"`
	Queue  QueueStats              `json:"queue"`
}

func NewController(
	q queue.QueueProvider,
	s store.StoreProvider,
	sc shutdown.CoordinatorProvider,
) *Controller {
	return &Controller{
		taskQueue:   q,
		statesStore: s,
		shutdown:    sc,
	}
}

func (c *Controller) EnqueueTask(req EnqueueRequest) (string, error) {
	if err := c.statesStore.CreateTask(req.ID); err != nil {
		return "", err
	}

	task := &queue.Task{
		ID:         req.ID,
		Payload:    req.Payload,
		MaxRetries: req.MaxRetries,
		Attemps:    0,
		CreatedAt:  time.Now(),
	}

	if err := c.taskQueue.Enqueue(task); err != nil {
		storeErr := c.statesStore.DeleteTask(req.ID)
		if storeErr != nil {
			return "", fmt.Errorf("queue error: %w; failed to delete task from store: %v", err, storeErr)
		}
		return "", err
	}

	return req.ID, nil
}

func (c *Controller) GetHealthStats() (map[store.TaskState]int, QueueStats) {
	taskStats := c.statesStore.GetStats()
	queueStats := QueueStats{
		Size:     c.taskQueue.Size(),
		Capacity: c.taskQueue.Capacity(),
	}

	return taskStats, queueStats
}

func (c *Controller) IsAccepting() bool {
	return c.shutdown.IsAccepting()
}
