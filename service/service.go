package service

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	shutdownContextTimeout  = 10 * time.Second
	waitForWaitGroupTimeout = 5 * time.Second
)

// Starter provides the expected interface for a service to be started.
// It is required that the service that is started runs `wg.Done()` or
// else it will timeout in 5 seconds. Right now, timeout isn't configurable.
type Starter interface {
	Start(ctx context.Context, wg *sync.WaitGroup) error
}

// Stopper provides the expected interfaces for a service to be stopped.
type Stopper interface {
	Stop(ctx context.Context) error
}

// Start takes a context, errorgroup and Starter interface.
// Will create a waitgroup and run wg.Add(1) and expect
// the service to run wg.Done() within the timeout.
// Passes Starter.Start(ctx, wg) to the errorgroup.
func Start(ctx context.Context, g *errgroup.Group, s Starter) {
	start(ctx, g, s, waitForWaitGroupTimeout)
}

func start(ctx context.Context, g *errgroup.Group, s Starter, timeout time.Duration) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	g.Go(func() error {
		return s.Start(ctx, wg)
	})

	err := waitForWaitGroupWithTimeout(wg, timeout)
	if err != nil {
		g.Go(func() error {
			return fmt.Errorf("timed out waiting for service to start: %T", s)
		})
	}
}

// Stop takes a context, errorgroup and Stopper interface.
// Passes Stopper.Stop(ctx) to the errorgroup.
// Timeout should be handled outside of the function using
// the context with timeout.
func Stop(ctx context.Context, g *errgroup.Group, s Stopper) {
	g.Go(func() error {
		return s.Stop(ctx)
	})
}

// NewErrGroupAndContext creates a new errorgroup and context, returns
// a pointer to the errorgroup.Group, context.Context and context.CancelFunc.
// Helper function for Start() and Stop().
func NewErrGroupAndContext() (*errgroup.Group, context.Context, context.CancelFunc) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)
	return g, ctx, cancel
}

// WaitForErrGroup takes a errgroup and waits for it,
// returning the error if any.
// Helper function for Start() and Stop().
func WaitForErrGroup(g *errgroup.Group) error {
	err := g.Wait()
	if err != nil {
		return fmt.Errorf("error groups error: %w", err)
	}

	return nil
}

// NewShutdownTimeoutContext creates acontext with
// a default timeout.
// Helper function for Start() and Stop().
func NewShutdownTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), shutdownContextTimeout)
}

// WaitForStop takes a channel and context and waits for
// one of them to close and returns a string with the reason.
// Helper function for Start() and Stop().
func WaitForStop(stopChan chan os.Signal, ctx context.Context) string {
	select {
	case sig := <-stopChan:
		return fmt.Sprintf("os.Signal (%s)", sig)
	case <-ctx.Done():
		return "context"
	}
}

// NewStopChannel creates a new channel that will be signalled by
// different os signals.
// Helper function for Start() and Stop().
func NewStopChannel() chan os.Signal {
	stopChan := make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	return stopChan
}

func waitForWaitGroupWithTimeout(wg *sync.WaitGroup, timeout time.Duration) error {
	c := make(chan struct{})

	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timed out after: %s", timeout)
	}
}
