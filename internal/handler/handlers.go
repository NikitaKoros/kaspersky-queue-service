package handler

import (
	"encoding/json"
	"net/http"

	"github.com/NikitaKoros/queue-service/internal/controller"
)

func HandleEnqueue(ctrl controller.ControllerProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ctrl.IsAccepting() {
			writeErrorResponce(w, http.StatusServiceUnavailable, "Service is shutting down")
			return
		}

		if r.Method != http.MethodPost {
			writeErrorResponce(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		var req controller.EnqueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErrorResponce(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.ID == "" {
			writeErrorResponce(w, http.StatusBadRequest, "ID is required")
			return
		}

		if req.MaxRetries < 0 {
			writeErrorResponce(w, http.StatusBadRequest, "max_retries must be >= 0")
			return
		}

		taskID, err := ctrl.EnqueueTask(req)
		if err != nil {
			switch err {
			case controller.ErrQueueFull:
				writeErrorResponce(w, http.StatusServiceUnavailable, "Queue is full")
				return
			case controller.ErrTaskExists:
				writeErrorResponce(w, http.StatusConflict, "Task with this ID already exists and is active")
				return
			default:
				writeErrorResponce(w, http.StatusInternalServerError, "Failed to enqueue task")
				return
			}
		}

		response := controller.EnqueueResponse{
			Message: "Task enqueued successfully",
			TaskID:  taskID,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(response)
	}
}

func HandleHealth(ctrl controller.ControllerProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorResponce(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		taskStats, queueStats := ctrl.GetHealthStats()

		response := controller.HealthResponse{
			Status: "ok",
			Stats:  taskStats,
			Queue:  queueStats,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(response)
	}
}

func writeErrorResponce(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]string{
		"error": message,
	}

	enc := json.NewEncoder(w) 
	enc.SetEscapeHTML(false)
	enc.Encode(errorResponse)
}
