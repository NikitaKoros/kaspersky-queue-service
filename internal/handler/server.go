package handler

import (
	"context"
	"log"
	"net/http"

	"github.com/NikitaKoros/queue-service/internal/controller"
)

type Server struct {
	httpServer *http.Server
	ctrl       controller.ControllerProvider
}

func NewServer(addr string, ctrl controller.ControllerProvider) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/enqueue", HandleEnqueue(ctrl))
	mux.HandleFunc("/healthz", HandleHealth(ctrl))

	httpServer := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return &Server{
		httpServer: &httpServer,
		ctrl:       ctrl,
	}
}

func (s *Server) Start() error {
	log.Printf("Starting http server on %s", s.httpServer.Addr)

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) {
	log.Println("Shutting down HTTP server...")

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	} else {
		log.Println("HTTP server shut up successfuly")
	}
}
