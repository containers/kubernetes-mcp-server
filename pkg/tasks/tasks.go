// Package tasks provides a concurrent task runner with optional per-task
// timeouts and concurrency limits.
package tasks

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// Task represents a single self-contained task.
// It carries both the definition (Name, Run) and the outcome
// (Output, Err), along with per-task execution timing and state.
type Task struct {
	Name string // identifier (e.g. "nodes", "pods")
	Run  func(ctx context.Context) (any, error)

	output    any
	err       error
	startTime time.Time
	duration  time.Duration
	executed  atomic.Bool // whether the task has been executed
}

// TaskResult is a shallow, read-only snapshot of a completed task's outcome.
// Returned by Execute and All to prevent mutation of internal state.
// Output is not deep-copied and may still reference mutable data from the task.
type TaskResult struct {
	Name      string        // identifier (e.g. "nodes", "pods")
	Output    any           // result output
	Err       error         // result error
	StartTime time.Time     // when the task started executing
	Duration  time.Duration // how long the task took
}

// result returns a shallow, read-only snapshot of the task's outcome.
func (t *Task) result() TaskResult {
	return TaskResult{
		Name:      t.Name,
		Output:    t.output,
		Err:       t.err,
		StartTime: t.startTime,
		Duration:  t.duration,
	}
}

// Tasks owns a collection of Task pointers and provides methods to add,
// execute, and iterate over them.
type Tasks struct {
	mu        sync.Mutex
	tasks     []*Task
	Timeout   time.Duration // per-task max timeout safety ceiling
	g         *errgroup.Group
	parentCtx context.Context // parent context, used by Execute
	gCtx      context.Context // errgroup-derived context, used by AddTask
	completed atomic.Bool     // set after Complete returns; prevents further AddTask calls
	pending   sync.WaitGroup  // tracks in-flight g.Go submissions from AddTask
}

// New creates a Tasks instance with the given per-task max timeout.
// A zero or negative timeout means no per-task timeout; tasks inherit
// only the parent context's deadline/cancellation.
// The provided context is used as the parent for the internal errgroup
// that synchronizes async task execution.
func New(ctx context.Context, timeout time.Duration) *Tasks {
	if ctx == nil {
		ctx = context.Background()
	}
	g, gCtx := errgroup.WithContext(ctx)
	return &Tasks{
		Timeout:   timeout,
		g:         g,
		gCtx:      gCtx,
		parentCtx: ctx,
	}
}

// NewWithLimit creates a Tasks instance like New but caps the number of
// concurrently executing goroutines to limit. A zero or negative limit
// means no concurrency limit (equivalent to New).
// This is useful when fanning out many tasks that hit the same rate-limited API client.
func NewWithLimit(ctx context.Context, timeout time.Duration, limit int) *Tasks {
	ts := New(ctx, timeout)
	if limit > 0 {
		ts.g.SetLimit(limit)
	}
	return ts
}

// AddTask adds a task and immediately starts executing it in a goroutine
// managed by the internal errgroup. The task inherits the errgroup context
// (derived from parent context passed to New/NewWithLimit).
// Use Complete to wait for all async tasks to finish.
// AddTask is silently ignored after Complete has been called.
//
// Limitation: when using NewWithLimit, AddTask may block until a concurrency
// slot opens. Because errgroup.Group.Go does not accept a context, this blocked
// submission itself is not cancellable by the parent context. Once the
// submission unblocks, the spawned task receives the errgroup context and can
// observe ctx.Done(). A running task must never wait on Complete.
func (ts *Tasks) AddTask(t *Task) {
	if t == nil || t.Run == nil || ts.completed.Load() {
		return
	}
	cp := &Task{
		Name: t.Name,
		Run:  t.Run,
	}
	ts.mu.Lock()
	if ts.completed.Load() {
		ts.mu.Unlock()
		return
	}
	ts.tasks = append(ts.tasks, cp)
	ts.pending.Add(1)
	ts.mu.Unlock()
	// g.Go is called outside the lock because errgroup.SetLimit can make
	// it block until a slot opens, which would otherwise hold ts.mu and
	// prevent Complete, All, and concurrent AddTask calls from proceeding.
	ts.g.Go(func() error {
		ts.executeTask(cp, ts.gCtx)
		return nil
	})
	ts.pending.Done()
}

// Execute adds a task and executes it immediately in blocking mode.
// The task inherits the parent context passed to New/NewWithLimit.
// Execute is not gated by Complete and may be called before, during,
// or after Complete.
// A shallow, read-only TaskResult snapshot is returned.
func (ts *Tasks) Execute(t *Task) *TaskResult {
	if t == nil || t.Run == nil {
		return nil
	}
	cp := &Task{
		Name: t.Name,
		Run:  t.Run,
	}
	ts.executeTask(cp, ts.parentCtx)
	ts.mu.Lock()
	ts.tasks = append(ts.tasks, cp)
	ts.mu.Unlock()
	ret := cp.result()
	return &ret
}

// Complete blocks until all async tasks added via AddTask have finished.
// Cancellation is driven by the parent context or per-task timeout; Complete
// waits for tasks to observe it.
// It is safe to call Complete multiple times.
// After Complete returns, no more tasks can be added via AddTask.
// Execute may still be called after Complete and will run synchronously.
// When using NewWithLimit, tasks must not block waiting for Complete to
// return; doing so can deadlock because Complete waits for in-flight
// AddTask submissions, which may be waiting for a running task slot.
//
// The error returned by g.Wait() is intentionally discarded: the runner
// captures per-task errors and panics in each TaskResult, and propagating
// the errgroup error would cancel the shared context on the first failure
// and abort unrelated in-flight tasks.
func (ts *Tasks) Complete() {
	ts.mu.Lock()
	ts.completed.Store(true)
	ts.mu.Unlock()
	ts.pending.Wait()
	// Intentionally ignore the errgroup error; see the doc comment above.
	_ = ts.g.Wait()
}

// All returns shallow, read-only snapshots of all executed tasks for iteration.
// Only tasks that have finished executing are included. Call Complete
// before All to ensure all tasks added by AddTask have finished.
// Because Execute appends the task only after its synchronous Run returns,
// any concurrent Execute calls must finish before All is called; otherwise
// their results may be omitted from the snapshot.
func (ts *Tasks) All() []TaskResult {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	snapshot := make([]TaskResult, 0, len(ts.tasks))
	for _, t := range ts.tasks {
		// executeTask stores output/err/startTime/duration before calling
		// executed.Store(true). Under the Go memory model, the atomic store
		// happens-after those writes and this atomic load happens-after the
		// store, so reading those fields without further synchronization is safe.
		if t.executed.Load() {
			snapshot = append(snapshot, t.result())
		}
	}
	return snapshot
}

// executeTask runs a single task, optionally applying Timeout as a safety
// ceiling when positive, recording timing and marking it as executed.
// If the task's Run function panics, the panic is recovered and surfaced
// as a non-nil Err on the resulting TaskResult.
func (ts *Tasks) executeTask(t *Task, ctx context.Context) {
	taskCtx := ctx
	if ts.Timeout > 0 {
		var cancel context.CancelFunc
		taskCtx, cancel = context.WithTimeout(ctx, ts.Timeout)
		defer cancel()
	}

	t.startTime = time.Now()
	var output any
	var err error
	defer func() {
		t.duration = time.Since(t.startTime)
		if r := recover(); r != nil {
			output = nil
			err = fmt.Errorf("task %q panicked: %v", t.Name, r)
		}
		t.err = err
		t.output = output
		t.executed.Store(true)
	}()

	output, err = t.Run(taskCtx)
}
