package main

import (
	"context"
	"log"
	"time"

	"github.com/NikitaKoros/queue-service/internal/store"
	"github.com/NikitaKoros/queue-service/internal/config"
	"github.com/NikitaKoros/queue-service/internal/controller"
	"github.com/NikitaKoros/queue-service/internal/handler"
	"github.com/NikitaKoros/queue-service/internal/queue"
	"github.com/NikitaKoros/queue-service/internal/shutdown"
)

func main() {
	cfg := config.LoadConfig()
	log.Printf("Configuration: Workers=%d, QueueSize=%d, BaseBackoff=%v, MaxBackoff=%v",
		cfg.Workers, cfg.QueueSize, cfg.BaseBackoff, cfg.MaxBackoff)
	
	memStore := store.NewMemStore()
	taskQueue := queue.NewQueue(cfg.QueueSize)
	workerPool := queue.NewWorkerPool(taskQueue, memStore, cfg.Workers, cfg.BaseBackoff, cfg.MaxBackoff)
	shutdownCoordinator := shutdown.NewCoordinator()
	
	ctrl := controller.NewController(taskQueue, memStore, shutdownCoordinator)
	server := handler.NewServer(cfg.ServerAddr, ctrl)
	
	workerPool.Start()
	
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	
	log.Println("Queue service started successfully")
	
	shutdownCoordinator.ShutdownWithTimeout(30*time.Second,
		func(ctx context.Context) {
			server.Shutdown(ctx)
		},
		func(ctx context.Context) {
			timeout := 25 * time.Second
			if deadline, ok := ctx.Deadline(); ok {
				if remaining := time.Until(deadline); remaining < timeout {
					timeout = remaining
				}
			}
			workerPool.Stop(timeout)
		},
	)
	
	log.Println("Queue service stopped")
}