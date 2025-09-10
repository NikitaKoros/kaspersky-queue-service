package shutdown

import (
	"context"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewCoordinator_IsAcceptingInitially(t *testing.T) {
	coord := NewCoordinator()
	if !coord.IsAccepting() {
		t.Error("Expected coordinator to be accepting initially, but it is not")
	}
}

func TestCoordinator_ShutdownSetsAcceptingToFalse(t *testing.T) {
	coord := NewCoordinator()

	coord.initiateShutdown()

	if coord.IsAccepting() {
		t.Error("Expected coordinator to stop accepting after shutdown, but it is still accepting")
	}
}

func TestCoordinator_ShutdownOnlyOnce(t *testing.T) {
	coord := NewCoordinator()
	var shutdownCount int

	oldLog := log.Writer()
	defer log.SetOutput(oldLog)

	var logBuf sync.Mutex
	var logs []string
	log.SetOutput(&testLogWriter{mu: &logBuf, logs: &logs})

	coord.initiateShutdown()
	coord.initiateShutdown()
	coord.initiateShutdown()

	for _, l := range logs {
		if strings.Contains(l, "Initiating graceful shutdown...") {
			shutdownCount++
		}
	}

	if shutdownCount != 1 {
		t.Errorf("Expected initiateShutdown to run only once, but ran %d times", shutdownCount)
	}

	if !coord.accepting.Load() {
	} else {
		t.Error("Coordinator should not be accepting after shutdown")
	}
}

func TestCoordinator_ShutdownWithTimeout_CompletesWithinTimeout(t *testing.T) {
	coord := NewCoordinator()

	oldLog := log.Writer()
	defer log.SetOutput(oldLog)
	var logs []string
	var logBuf sync.Mutex
	log.SetOutput(&testLogWriter{mu: &logBuf, logs: &logs})

	called := make(chan bool, 1)
	shutdownFunc := func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
		called <- true
	}

	go func() {
		time.Sleep(1 * time.Millisecond)
		coord.initiateShutdown()
	}()

	coord.ShutdownWithTimeout(100*time.Millisecond, shutdownFunc)

	select {
	case <-called:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Shutdown function was not called within expected time")
	}

	needLog := "Graceful shutdown completed successfuly"
	found := false
	for _, l := range logs {
		if strings.Contains(l, needLog) {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected log message 'Graceful shutdown completed successfuly' not found")
	}
}

func TestCoordinator_ShutdownWithTimeout_TimesOut(t *testing.T) {
	coord := NewCoordinator()

	oldLog := log.Writer()
	defer log.SetOutput(oldLog)
	var logs []string
	var logBuf sync.Mutex
	log.SetOutput(&testLogWriter{mu: &logBuf, logs: &logs})

	shutdownFunc := func(ctx context.Context) {
		<-ctx.Done()
	}

	go func() {
		time.Sleep(1 * time.Millisecond)
		coord.initiateShutdown()
	}()

	start := time.Now()
	coord.ShutdownWithTimeout(50*time.Millisecond, shutdownFunc)
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond {
		t.Error("Shutdown finished too quickly — timeout logic may be broken")
	}

	needLog := "Graceful shutdown timeout reached, forcing exit"
	found := false
	for _, l := range logs {
		if strings.Contains(l, needLog) {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected timeout log message not found")
	}
}

type testLogWriter struct {
	mu   *sync.Mutex
	logs *[]string
}

func (w *testLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	*w.logs = append(*w.logs, string(p[:len(p)-1]))
	return len(p), nil
}
