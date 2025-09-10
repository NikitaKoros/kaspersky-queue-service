package shutdown

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type CoordinatorProvider interface {
	IsAccepting() bool
	ShutdownWithTimeout(time.Duration, ...func(context.Context))
}

type Coordinator struct {
	accepting    atomic.Bool
	shutDownOnce sync.Once
	signals      chan os.Signal
	done         chan struct{}
}

func NewCoordinator() *Coordinator {
	c := &Coordinator{
		signals: make(chan os.Signal, 1),
		done:    make(chan struct{}),
	}

	c.accepting.Store(true)

	signal.Notify(c.signals, syscall.SIGINT, syscall.SIGTERM)

	return c
}

func (c *Coordinator) IsAccepting() bool {
	return c.accepting.Load()
}

func (c *Coordinator) initiateShutdown() {
	c.shutDownOnce.Do(func() {
		log.Println("Initiating graceful shutdown...")

		c.accepting.Store(false)
		log.Println("Stopped accepting new tasks...")

		close(c.done)
	})
}

func (c *Coordinator) waitForSignal() {
	go func() {
		sig := <-c.signals
		log.Printf("Recieved shutdown signal: %v", sig)
		c.initiateShutdown()
	}()
}

func (c *Coordinator) ShutdownWithTimeout(timeout time.Duration, shutdownFuncs ...func(context.Context)) {
	c.waitForSignal()

	<-c.done

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Starting graceful shutdown with %v timeout", timeout)

	var wg sync.WaitGroup
	for _, fn := range shutdownFuncs {
		wg.Add(1)
		go func(shutdownFunc func(context.Context)) {
			defer wg.Done()
			shutdownFunc(ctx)
		}(fn)
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Graceful shutdown completed successfuly")
	case <-ctx.Done():
		log.Println("Graceful shutdown timeout reached, forcing exit")
	}

}
