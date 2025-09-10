package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NikitaKoros/queue-service/internal/controller"
	"github.com/NikitaKoros/queue-service/internal/store"
)

// --- Моки ---

type MockController struct {
	enqueueErr    error
	isAccepting   bool
	taskStats     map[store.TaskState]int
	queueStats    controller.QueueStats
	enqueueCalled bool
	lastReq       controller.EnqueueRequest
}

func (mc *MockController) EnqueueTask(req controller.EnqueueRequest) (string, error) {
	mc.enqueueCalled = true
	mc.lastReq = req
	if mc.enqueueErr != nil {
		return "", mc.enqueueErr
	}
	return req.ID, nil
}

func (mc *MockController) GetHealthStats() (map[store.TaskState]int, controller.QueueStats) {
	return mc.taskStats, mc.queueStats
}

func (mc *MockController) IsAccepting() bool {
	return mc.isAccepting
}

// --- Тесты Server ---

func TestServer_StartAndShutdown(t *testing.T) {
	ctrl := &MockController{}
	srv := NewServer(":0", ctrl) // :0 — случайный порт

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		srv.Shutdown(ctx)
	}()

	err := srv.Start()
	if err != nil && err != http.ErrServerClosed {
		t.Errorf("Expected server to shut down cleanly, got %v", err)
	}
}

// --- Тесты хендлеров ---

func TestHandleEnqueue_Success(t *testing.T) {
	ctrl := &MockController{isAccepting: true}
	handler := HandleEnqueue(ctrl)

	reqBody := `{"id": "task1", "payload": "test", "max_retries": 3}`
	req := httptest.NewRequest(http.MethodPost, "/enqueue", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", resp.StatusCode)
	}

	var res controller.EnqueueResponse
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal("Failed to decode response JSON")
	}

	if res.TaskID != "task1" || res.Message != "Task enqueued successfully" {
		t.Errorf("Unexpected response: %+v", res)
	}

	if !ctrl.enqueueCalled {
		t.Error("Expected EnqueueTask to be called")
	}

	if ctrl.lastReq.ID != "task1" || ctrl.lastReq.Payload != "test" || ctrl.lastReq.MaxRetries != 3 {
		t.Errorf("Unexpected request passed to controller: %+v", ctrl.lastReq)
	}
}

func TestHandleEnqueue_ServiceNotAccepting(t *testing.T) {
	ctrl := &MockController{isAccepting: false}
	handler := HandleEnqueue(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/enqueue", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", resp.StatusCode)
	}

	if !strings.Contains(string(body), "Service is shutting down") {
		t.Errorf("Expected shutdown message, got %s", string(body))
	}
}

func TestHandleEnqueue_QueueFull(t *testing.T) {
	ctrl := &MockController{
		isAccepting: true,
		enqueueErr:  controller.ErrQueueFull,
	}
	handler := HandleEnqueue(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/enqueue", strings.NewReader(`{"id": "task1"}`))
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", resp.StatusCode)
	}

	if !strings.Contains(string(body), "Queue is full") {
		t.Errorf("Expected 'Queue is full', got %s", string(body))
	}
}

func TestHandleEnqueue_TaskExists(t *testing.T) {
	ctrl := &MockController{
		isAccepting: true,
		enqueueErr:  controller.ErrTaskExists,
	}
	handler := HandleEnqueue(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/enqueue", strings.NewReader(`{"id": "task1"}`))
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected 409, got %d", resp.StatusCode)
	}

	if !strings.Contains(string(body), "Task with this ID already exists") {
		t.Errorf("Expected conflict message, got %s", string(body))
	}
}

func TestHandleEnqueue_InvalidJSON(t *testing.T) {
	ctrl := &MockController{isAccepting: true}
	handler := HandleEnqueue(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/enqueue", strings.NewReader(`{invalid json`))
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}

	if !strings.Contains(string(body), "Invalid JSON") {
		t.Errorf("Expected 'Invalid JSON', got %s", string(body))
	}
}

func TestHandleEnqueue_Validation(t *testing.T) {
	ctrl := &MockController{isAccepting: true}
	handler := HandleEnqueue(ctrl)

	tests := []struct {
		name     string
		body     string
		expected int
		message  string
	}{
		{"Missing ID", `{"payload": "test"}`, http.StatusBadRequest, "ID is required"},
		{"Negative MaxRetries", `{"id": "task1", "max_retries": -1}`, http.StatusBadRequest, "max_retries must be >= 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/enqueue", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, resp.StatusCode)
			}

			if !strings.Contains(string(body), tt.message) {
				t.Errorf("Expected message %q, got %s", tt.message, string(body))
			}
		})
	}
}

func TestHandleHealth_Success(t *testing.T) {
	ctrl := &MockController{
		taskStats: map[store.TaskState]int{
			store.TaskStateQueued:  5,
			store.TaskStateRunning: 2,
		},
		queueStats: controller.QueueStats{
			Size:     7,
			Capacity: 100,
		},
	}
	handler := HandleHealth(ctrl)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("Expected 202, got %d", resp.StatusCode)
	}

	var res controller.HealthResponse
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal("Failed to decode response JSON")
	}

	if res.Status != "ok" {
		t.Errorf("Expected status 'ok', got %s", res.Status)
	}

	if res.Stats[store.TaskStateQueued] != 5 {
		t.Errorf("Expected 5 queued tasks, got %d", res.Stats[store.TaskStateQueued])
	}

	if res.Queue.Size != 7 || res.Queue.Capacity != 100 {
		t.Errorf("Unexpected queue stats: %+v", res.Queue)
	}
}

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	ctrl := &MockController{}
	handler := HandleHealth(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}

	if !strings.Contains(string(body), "Method not allowed") {
		t.Errorf("Expected method not allowed message, got %s", string(body))
	}
}
