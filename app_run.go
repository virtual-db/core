package core

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AnqorDX/vdb-core/internal/framework"
	"github.com/AnqorDX/vdb-core/internal/points"
)

// Run executes the full startup sequence and then blocks until Stop is called
// or the server terminates. It may only be called once per *App.
//
// Startup sequence:
//  1. vdb.context.create  — build and seal the global context.
//  2. ConnectAll          — discover, launch, and wire all plugins.
//  3. vdb.server.start    — configure and launch the server goroutine.
//  4. Idle                — block until Stop() signals shutdown or the server exits.
func (a *App) Run() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("core: Run already called")
	}
	a.running = true
	a.mu.Unlock()

	// Step 1: vdb.context.create — build and seal the global context.
	if _, err := a.pipe.Process(points.PipelineContextCreate, nil, framework.HandlerContext{}); err != nil {
		return fmt.Errorf("core: vdb.context.create: %w", err)
	}

	// Step 2: Connect all plugins.
	a.plugins.ConnectAll(a.bus, a.pipe)

	// Step 3: vdb.server.start — configure and launch the server goroutine.
	if _, err := a.global.Pipeline().Process(points.PipelineServerStart, nil); err != nil {
		return fmt.Errorf("core: vdb.server.start: %w", err)
	}

	// Step 4: OS signal handling and idle loop.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case err := <-a.serverErrCh:
		a.Stop()
		if err != nil {
			return fmt.Errorf("core: server terminated: %w", err)
		}
		return nil
	case <-sigCh:
		log.Printf("core: signal received; initiating graceful shutdown")
		a.Stop()
		return nil
	case <-a.shutdown:
		return nil
	}
}

// Stop executes the graceful shutdown sequence and unblocks Run. Idempotent.
func (a *App) Stop() {
	a.mu.Lock()
	if !a.running || a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	a.mu.Unlock()

	if _, err := a.global.Pipeline().Process(points.PipelineServerStop, nil); err != nil {
		log.Printf("core: vdb.server.stop: %v", err)
	}

	close(a.shutdown)
}
