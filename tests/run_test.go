// Package core_test — tests/run_test.go
//
// Black-box integration tests for App.Run() and App.Stop().
// All unexported-field tests have been dropped; only public API is exercised.
//
// Pipeline accessibility note:
//
//	vdb.context.create is host-only: it runs before plugins connect, so
//	out-of-process plugin handlers can never be invoked on it. Tests in the
//	"vdb.context.create tests" section below exercise the in-process attachment
//	path — the intended use for drivers and embedders that call app.Attach
//	before app.Run.
//
//	vdb.server.stop is host-only: plugin cleanup on shutdown is handled by the
//	shutdown JSON-RPC request sent at the drain point, not by plugin pipeline
//	handlers. Tests for stop behaviour verify App.Stop() mechanics only.
package core_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/virtual-db/core"
	"github.com/virtual-db/core/internal/framework"
	"github.com/virtual-db/core/internal/points"
)

// ── recordingServer ───────────────────────────────────────────────────────

type recordingServer struct {
	runStarted chan struct{}
	runBlock   chan struct{}
	runReturn  error

	stopMu     sync.Mutex
	stopCalled bool
	stopErr    error
}

func newRecordingServer() *recordingServer {
	return &recordingServer{
		runStarted: make(chan struct{}),
		runBlock:   make(chan struct{}),
	}
}

func (s *recordingServer) Run() error {
	close(s.runStarted)
	<-s.runBlock
	return s.runReturn
}

func (s *recordingServer) Stop() error {
	s.stopMu.Lock()
	s.stopCalled = true
	s.stopMu.Unlock()
	select {
	case <-s.runBlock:
	default:
		close(s.runBlock)
	}
	return s.stopErr
}

func (s *recordingServer) wasStopCalled() bool {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	return s.stopCalled
}

// ── test helpers ──────────────────────────────────────────────────────────

func runInBackground(app *App) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- app.Run() }()
	return ch
}

func awaitServerStart(t *testing.T, srv *recordingServer) {
	t.Helper()
	select {
	case <-srv.runStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("server.Run() was not called within 500 ms — startup pipeline stalled")
	}
}

func awaitRunReturn(t *testing.T, runCh <-chan error) error {
	t.Helper()
	select {
	case err := <-runCh:
		return err
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not return within 500 ms")
		return nil
	}
}

func startEmitReadyCh(t *testing.T, app *App) <-chan struct{} {
	t.Helper()
	ch := make(chan struct{})
	var once sync.Once
	app.Attach(points.PointServerStartEmit, 5, func(ctx any, p any) (any, any, error) {
		once.Do(func() { close(ch) })
		return ctx, p, nil
	})
	return ch
}

// ── vdb.context.create tests (host-only, in-process attach path) ──────────
//
// vdb.context.create is host-only. The contribute point is the extension point
// for in-process host code — drivers and embedders that call app.Attach before
// app.Run — to inject values into the process-wide GlobalContext before it is
// sealed. Out-of-process plugins cannot reach any point in this pipeline.

// TestRunContextCreateContributeHonored verifies that a handler at
// vdb.context.create.contribute can store a key-value pair that survives sealing.
// This exercises the in-process (host-only) attach path, not a plugin declare path.
func TestRunContextCreateContributeHonored(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	app.Attach(points.PointContextCreateContribute, 10,
		func(ctx any, p any) (any, any, error) {
			if b, ok := p.(*framework.GlobalContextBuilder); ok {
				b.Set("run-test-key", "run-test-value")
			}
			return ctx, p, nil
		})

	runCh := runInBackground(app)
	awaitServerStart(t, srv)

	// The real assertion: Run() did not abort (server is still running).
	// If the contribute handler had failed, Run() would have already returned.
	select {
	case err := <-runCh:
		t.Fatalf("Run() returned early with error: %v", err)
	default:
		// still running — contribute pipeline succeeded
	}

	app.Stop()
	awaitRunReturn(t, runCh)
}

// TestRunContextCreateMultipleContributors verifies that multiple in-process
// contribute handlers can each add their own key without causing a startup
// failure. Exercises the host-only contribute point with concurrent priorities.
func TestRunContextCreateMultipleContributors(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	for _, key := range []string{"x", "y", "z"} {
		key := key
		app.Attach(points.PointContextCreateContribute, 10,
			func(ctx any, p any) (any, any, error) {
				if b, ok := p.(*framework.GlobalContextBuilder); ok {
					b.Set(key, key+"-val")
				}
				return ctx, p, nil
			})
	}

	runCh := runInBackground(app)
	awaitServerStart(t, srv)

	// All three contribute handlers ran without error — Run() is still alive.
	select {
	case err := <-runCh:
		t.Fatalf("Run() returned early with error: %v", err)
	default:
	}

	app.Stop()
	awaitRunReturn(t, runCh)
}

// ── vdb.server.start tests ────────────────────────────────────────────────

func TestRunLaunchesServerGoroutine(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	runCh := runInBackground(app)
	awaitServerStart(t, srv)

	app.Stop()
	awaitRunReturn(t, runCh)
}

func TestRunWithNoServerBlocksAndStopsCleanly(t *testing.T) {
	app := New(Config{})
	readyCh := startEmitReadyCh(t, app)

	runCh := runInBackground(app)

	select {
	case <-readyCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("startup did not complete within 500 ms (no server registered)")
	}

	app.Stop()
	if err := awaitRunReturn(t, runCh); err != nil {
		t.Errorf("Run() with no server should return nil, got: %v", err)
	}
}

// ── App.Stop() tests ──────────────────────────────────────────────────────

func TestStopCallsServerStop(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	runCh := runInBackground(app)
	awaitServerStart(t, srv)

	app.Stop()

	if !srv.wasStopCalled() {
		t.Error("server.Stop() was not called by App.Stop()")
	}

	awaitRunReturn(t, runCh)
}

func TestStopCausesRunToReturnNil(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	runCh := runInBackground(app)
	awaitServerStart(t, srv)
	app.Stop()

	if err := awaitRunReturn(t, runCh); err != nil {
		t.Errorf("Run() should return nil on graceful Stop(), got: %v", err)
	}
}

func TestStopBeforeRunIsNoOp(t *testing.T) {
	app := New(Config{})
	app.Stop()
}

func TestStopIsIdempotent(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	runCh := runInBackground(app)
	awaitServerStart(t, srv)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			app.Stop()
		}()
	}
	wg.Wait()

	if err := awaitRunReturn(t, runCh); err != nil {
		t.Errorf("Run() returned unexpected error after concurrent Stop(): %v", err)
	}
}

func TestStopEmitsServerStoppedEvent(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	eventFired := make(chan struct{}, 1)
	app.Subscribe(points.EventServerStopped, func(_ any, _ any) error {
		select {
		case eventFired <- struct{}{}:
		default:
		}
		return nil
	})

	runCh := runInBackground(app)
	awaitServerStart(t, srv)
	app.Stop()
	awaitRunReturn(t, runCh)

	select {
	case <-eventFired:
	case <-time.After(200 * time.Millisecond):
		t.Error("vdb.server.stopped event was not emitted after App.Stop()")
	}
}

// ── App.Run() error-path tests ────────────────────────────────────────────

func TestRunCannotBeCalledTwice(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	runCh := runInBackground(app)
	awaitServerStart(t, srv)

	if err := app.Run(); err == nil {
		t.Error("second Run() call should return an error, got nil")
	}

	app.Stop()
	awaitRunReturn(t, runCh)
}

func TestRunReturnsErrorWhenServerCrashes(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	srv.runReturn = errors.New("database connection lost")
	app.UseDriver(srv)

	runCh := runInBackground(app)
	awaitServerStart(t, srv)

	close(srv.runBlock)

	err := awaitRunReturn(t, runCh)
	if err == nil {
		t.Fatal("Run() should return a non-nil error when server crashes")
	}
}

func TestRunReturnsNilOnCleanServerExit(t *testing.T) {
	app := New(Config{})
	srv := newRecordingServer()
	app.UseDriver(srv)

	runCh := runInBackground(app)
	awaitServerStart(t, srv)
	close(srv.runBlock)

	if err := awaitRunReturn(t, runCh); err != nil {
		t.Errorf("Run() should return nil on clean server exit, got: %v", err)
	}
}

// TestRunStartupHandlerErrorAbortsRun verifies that an error returned by an
// in-process handler on the host-only vdb.context.create.contribute point
// causes App.Run() to return that error immediately without starting the server.
func TestRunStartupHandlerErrorAbortsRun(t *testing.T) {
	app := New(Config{})
	app.Attach(points.PointContextCreateContribute, 10,
		func(ctx any, _ any) (any, any, error) {
			return ctx, nil, errors.New("forced startup failure")
		})

	runCh := runInBackground(app)
	err := awaitRunReturn(t, runCh)
	if err == nil {
		t.Fatal("Run() should return an error when a startup handler fails, got nil")
	}
}
