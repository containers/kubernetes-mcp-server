package tasks_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
	"github.com/stretchr/testify/suite"
)

type TasksSuite struct {
	suite.Suite
}

// resultSnapshot captures the deterministic fields of a TaskResult for
// one-assertion equality checks.
type resultSnapshot struct {
	Name   string
	Output any
	Err    error
}

// snapshot returns a resultSnapshot for a single TaskResult.
func snapshot(r *tasks.TaskResult) resultSnapshot {
	if r == nil {
		return resultSnapshot{}
	}
	return resultSnapshot{
		Name:   r.Name,
		Output: r.Output,
		Err:    r.Err,
	}
}

// snapshots returns resultSnapshots for a slice of TaskResults.
func snapshots(rs []tasks.TaskResult) []resultSnapshot {
	out := make([]resultSnapshot, len(rs))
	for i, r := range rs {
		out[i] = snapshot(&r)
	}
	return out
}

// names extracts task names from a slice of TaskResults.
func names(rs []tasks.TaskResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

func (s *TasksSuite) TestNew() {
	s.Run("creates instance with specified timeout", func() {
		ts := tasks.New(context.Background(), 5*time.Second)
		s.Equal(5*time.Second, ts.Timeout)
	})

	s.Run("returns non-nil Tasks", func() {
		ts := tasks.New(context.Background(), time.Second)
		s.NotNil(ts)
	})
}

func (s *TasksSuite) TestAddTask() {
	s.Run("nil task is ignored", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.AddTask(nil)
		ts.Complete()
		s.Empty(ts.All())
	})

	s.Run("task with nil Run is ignored", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.AddTask(&tasks.Task{Name: "no-run"})
		ts.Complete()
		s.Empty(ts.All())
	})

	s.Run("task is executed asynchronously", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.AddTask(&tasks.Task{
			Name: "async",
			Run: func(_ context.Context) (any, error) {
				return "done", nil
			},
		})
		ts.Complete()
		s.Equal([]resultSnapshot{
			{Name: "async", Output: "done"},
		}, snapshots(ts.All()))
	})

	s.Run("multiple tasks execute concurrently", func() {
		ts := tasks.New(context.Background(), 5*time.Second)
		var running atomic.Int32
		var maxConcurrent atomic.Int32
		for i := 0; i < 3; i++ {
			ts.AddTask(&tasks.Task{
				Name: "concurrent",
				Run: func(_ context.Context) (any, error) {
					cur := running.Add(1)
					for {
						old := maxConcurrent.Load()
						if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
							break
						}
					}
					time.Sleep(50 * time.Millisecond)
					running.Add(-1)
					return nil, nil
				},
			})
		}
		ts.Complete()
		s.Greater(maxConcurrent.Load(), int32(1))
	})

	s.Run("copies task fields", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.AddTask(&tasks.Task{
			Name: "original",
			Run: func(_ context.Context) (any, error) {
				return "output", nil
			},
		})
		ts.Complete()
		s.Equal([]resultSnapshot{
			{Name: "original", Output: "output"},
		}, snapshots(ts.All()))
	})

	s.Run("task panic does not prevent other tasks from completing", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.AddTask(&tasks.Task{
			Name: "panic",
			Run: func(_ context.Context) (any, error) {
				panic("boom")
			},
		})
		ts.AddTask(&tasks.Task{
			Name: "ok",
			Run: func(_ context.Context) (any, error) {
				return "ok", nil
			},
		})
		s.NotPanics(func() {
			ts.Complete()
		})
		s.Equal([]resultSnapshot{
			{Name: "panic", Err: fmt.Errorf("task \"panic\" panicked: boom")},
			{Name: "ok", Output: "ok"},
		}, snapshots(ts.All()))
	})
}

func (s *TasksSuite) TestExecute() {
	s.Run("nil task returns nil", func() {
		ts := tasks.New(context.Background(), time.Second)
		result := ts.Execute(nil)
		s.Nil(result)
	})

	s.Run("task with nil Run returns nil", func() {
		ts := tasks.New(context.Background(), time.Second)
		result := ts.Execute(&tasks.Task{Name: "no-run"})
		s.Nil(result)
	})

	s.Run("returns result with correct output", func() {
		ts := tasks.New(context.Background(), time.Second)
		result := ts.Execute(&tasks.Task{
			Name: "sync",
			Run: func(_ context.Context) (any, error) {
				return "hello", nil
			},
		})
		s.Equal(resultSnapshot{
			Name:   "sync",
			Output: "hello",
		}, snapshot(result))
	})

	s.Run("records error when task fails", func() {
		ts := tasks.New(context.Background(), time.Second)
		expectedErr := errors.New("task failed")
		result := ts.Execute(&tasks.Task{
			Name: "failing",
			Run: func(_ context.Context) (any, error) {
				return nil, expectedErr
			},
		})
		s.Equal(resultSnapshot{
			Name: "failing",
			Err:  expectedErr,
		}, snapshot(result))
	})

	s.Run("records start time", func() {
		ts := tasks.New(context.Background(), time.Second)
		before := time.Now()
		result := ts.Execute(&tasks.Task{
			Name: "timed",
			Run: func(_ context.Context) (any, error) {
				time.Sleep(10 * time.Millisecond)
				return nil, nil
			},
		})
		s.Require().NotNil(result)
		s.True(result.StartTime.Equal(before) || result.StartTime.After(before))
	})

	s.Run("records duration", func() {
		ts := tasks.New(context.Background(), time.Second)
		result := ts.Execute(&tasks.Task{
			Name: "timed",
			Run: func(_ context.Context) (any, error) {
				time.Sleep(10 * time.Millisecond)
				return nil, nil
			},
		})
		s.Require().NotNil(result)
		s.GreaterOrEqual(result.Duration.Milliseconds(), int64(10))
	})

	s.Run("executed task appears in All", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.Execute(&tasks.Task{
			Name: "tracked",
			Run: func(_ context.Context) (any, error) {
				return "tracked-output", nil
			},
		})
		s.Equal([]string{"tracked"}, names(ts.All()))
	})

	s.Run("error task preserves output", func() {
		ts := tasks.New(context.Background(), time.Second)
		result := ts.Execute(&tasks.Task{
			Name: "err-task",
			Run: func(_ context.Context) (any, error) {
				return "partial-result", errors.New("fail")
			},
		})
		s.Equal(resultSnapshot{
			Name:   "err-task",
			Output: "partial-result",
			Err:    errors.New("fail"),
		}, snapshot(result))
	})

	s.Run("task panic is captured as error", func() {
		ts := tasks.New(context.Background(), time.Second)
		result := ts.Execute(&tasks.Task{
			Name: "panic",
			Run: func(_ context.Context) (any, error) {
				panic("boom")
			},
		})
		s.Equal(resultSnapshot{
			Name: "panic",
			Err:  fmt.Errorf("task \"panic\" panicked: boom"),
		}, snapshot(result))
	})
}

func (s *TasksSuite) TestComplete() {
	s.Run("blocks until all async tasks finish", func() {
		ts := tasks.New(context.Background(), 5*time.Second)
		var finished atomic.Bool
		ts.AddTask(&tasks.Task{
			Name: "slow",
			Run: func(_ context.Context) (any, error) {
				time.Sleep(50 * time.Millisecond)
				finished.Store(true)
				return nil, nil
			},
		})
		ts.Complete()
		s.True(finished.Load())
	})

	s.Run("is safe to call with no tasks", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.Complete()
	})

	s.Run("AddTask after Complete is ignored", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.AddTask(&tasks.Task{
			Name: "before",
			Run: func(_ context.Context) (any, error) {
				return "ok", nil
			},
		})
		ts.Complete()
		ts.AddTask(&tasks.Task{
			Name: "after",
			Run: func(_ context.Context) (any, error) {
				return "should not run", nil
			},
		})
		s.Equal([]string{"before"}, names(ts.All()))
	})
}

func (s *TasksSuite) TestAll() {
	s.Run("returns empty slice when no tasks", func() {
		ts := tasks.New(context.Background(), time.Second)
		results := ts.All()
		s.Empty(results)
	})

	s.Run("returns all executed tasks in order", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.Execute(&tasks.Task{
			Name: "first",
			Run: func(_ context.Context) (any, error) {
				return "one", nil
			},
		})
		ts.Execute(&tasks.Task{
			Name: "second",
			Run: func(_ context.Context) (any, error) {
				return "two", nil
			},
		})
		s.Equal([]string{"first", "second"}, names(ts.All()))
	})

	s.Run("includes both sync and async tasks after Complete", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.Execute(&tasks.Task{
			Name: "sync",
			Run: func(_ context.Context) (any, error) {
				return "sync-out", nil
			},
		})
		ts.AddTask(&tasks.Task{
			Name: "async",
			Run: func(_ context.Context) (any, error) {
				return "async-out", nil
			},
		})
		ts.Complete()
		s.Equal([]string{"sync", "async"}, names(ts.All()))
	})

	s.Run("returns snapshots independent of internal state", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.Execute(&tasks.Task{
			Name: "snap",
			Run: func(_ context.Context) (any, error) {
				return "value", nil
			},
		})
		first := ts.All()
		second := ts.All()
		s.Equal(first, second)
	})
}

func (s *TasksSuite) TestNewWithLimitDoesNotBlockMutex() {
	s.Run("AddTask waiting on limit does not block All", func() {
		ts := tasks.NewWithLimit(context.Background(), 5*time.Second, 1)

		started := make(chan struct{})
		release := make(chan struct{})
		ts.AddTask(&tasks.Task{
			Name: "blocker",
			Run: func(_ context.Context) (any, error) {
				close(started)
				<-release
				return "blocking-done", nil
			},
		})
		<-started // blocker is holding the single slot

		// This AddTask will block inside g.Go waiting for the slot.
		// Before the fix it held ts.mu while blocking.
		go ts.AddTask(&tasks.Task{
			Name: "queued",
			Run: func(_ context.Context) (any, error) {
				return "queued-result", nil
			},
		})
		// There is no public signal for the goroutine reaching the blocking
		// g.Go call, so give the scheduler a brief window.
		time.Sleep(50 * time.Millisecond)

		allDone := make(chan struct{})
		go func() {
			ts.All()
			close(allDone)
		}()

		select {
		case <-allDone:
			// All() returned without blocking - correct
		case <-time.After(time.Second):
			s.Fail("All() was blocked by an AddTask call waiting on concurrency limit")
		}

		close(release)
		ts.Complete()
		s.Len(ts.All(), 2)
	})

	s.Run("AddTask waiting on limit does not block Complete", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ts := tasks.NewWithLimit(ctx, 5*time.Second, 1)

		started := make(chan struct{})
		release := make(chan struct{})
		ts.AddTask(&tasks.Task{
			Name: "blocker",
			Run: func(_ context.Context) (any, error) {
				close(started)
				<-release
				return "done", nil
			},
		})
		<-started

		go ts.AddTask(&tasks.Task{
			Name: "queued",
			Run: func(_ context.Context) (any, error) {
				return "ok", nil
			},
		})
		// Give the scheduler a window to let the queued AddTask reach the
		// blocking g.Go call.
		time.Sleep(50 * time.Millisecond)

		completedDone := make(chan struct{})
		go func() {
			close(release)
			ts.Complete()
			close(completedDone)
		}()

		select {
		case <-completedDone:
			// Complete() returned - correct
		case <-time.After(2 * time.Second):
			s.Fail("Complete() was blocked by an AddTask call waiting on concurrency limit")
		}
	})

	s.Run("concurrent AddTask calls do not block each other on limit", func() {
		ts := tasks.NewWithLimit(context.Background(), 5*time.Second, 1)

		started := make(chan struct{})
		release := make(chan struct{})
		ts.AddTask(&tasks.Task{
			Name: "blocker",
			Run: func(_ context.Context) (any, error) {
				close(started)
				<-release
				return nil, nil
			},
		})
		<-started

		for i := 0; i < 3; i++ {
			go ts.AddTask(&tasks.Task{
				Name: "concurrent",
				Run: func(_ context.Context) (any, error) {
					return nil, nil
				},
			})
		}
		// Give the scheduler a window to let the queued AddTasks reach the
		// blocking g.Go calls.
		time.Sleep(50 * time.Millisecond)

		allDone := make(chan struct{})
		go func() {
			ts.All()
			close(allDone)
		}()

		select {
		case <-allDone:
			// Not blocked
		case <-time.After(time.Second):
			s.Fail("All() was blocked by concurrent AddTask calls waiting on limit")
		}

		close(release)
		ts.Complete()
		s.Len(ts.All(), 4)
	})
}

func (s *TasksSuite) TestTimeout() {
	s.Run("task is cancelled when timeout exceeded", func() {
		ts := tasks.New(context.Background(), 50*time.Millisecond)
		result := ts.Execute(&tasks.Task{
			Name: "timeout",
			Run: func(ctx context.Context) (any, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(5 * time.Second):
					return "should not reach", nil
				}
			},
		})
		s.Equal(resultSnapshot{
			Name: "timeout",
			Err:  context.DeadlineExceeded,
		}, snapshot(result))
	})

	s.Run("task completes within timeout", func() {
		ts := tasks.New(context.Background(), time.Second)
		result := ts.Execute(&tasks.Task{
			Name: "fast",
			Run: func(_ context.Context) (any, error) {
				return "quick", nil
			},
		})
		s.Equal(resultSnapshot{
			Name:   "fast",
			Output: "quick",
		}, snapshot(result))
	})
}

func (s *TasksSuite) TestZeroTimeout() {
	s.Run("zero timeout means no per-task timeout", func() {
		ts := tasks.New(context.Background(), 0)
		result := ts.Execute(&tasks.Task{
			Name: "no-timeout",
			Run: func(_ context.Context) (any, error) {
				return "ok", nil
			},
		})
		s.Equal(resultSnapshot{
			Name:   "no-timeout",
			Output: "ok",
		}, snapshot(result))
	})

	s.Run("negative timeout means no per-task timeout", func() {
		ts := tasks.New(context.Background(), -time.Second)
		result := ts.Execute(&tasks.Task{
			Name: "neg-timeout",
			Run: func(_ context.Context) (any, error) {
				return "ok", nil
			},
		})
		s.Equal(resultSnapshot{
			Name:   "neg-timeout",
			Output: "ok",
		}, snapshot(result))
	})
}

func (s *TasksSuite) TestZeroOrNegativeLimit() {
	s.Run("zero limit allows concurrent execution", func() {
		ts := tasks.NewWithLimit(context.Background(), time.Second, 0)
		var running atomic.Int32
		var maxConcurrent atomic.Int32
		for i := 0; i < 3; i++ {
			ts.AddTask(&tasks.Task{
				Name: "unlimited",
				Run: func(_ context.Context) (any, error) {
					cur := running.Add(1)
					for {
						old := maxConcurrent.Load()
						if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
							break
						}
					}
					time.Sleep(50 * time.Millisecond)
					running.Add(-1)
					return nil, nil
				},
			})
		}
		ts.Complete()
		s.Greater(maxConcurrent.Load(), int32(1))
	})

	s.Run("zero limit runs all tasks", func() {
		ts := tasks.NewWithLimit(context.Background(), time.Second, 0)
		for i := 0; i < 3; i++ {
			ts.AddTask(&tasks.Task{
				Name: "unlimited",
				Run: func(_ context.Context) (any, error) {
					return nil, nil
				},
			})
		}
		ts.Complete()
		s.Len(ts.All(), 3)
	})

	s.Run("negative limit allows concurrent execution", func() {
		ts := tasks.NewWithLimit(context.Background(), time.Second, -1)
		var running atomic.Int32
		var maxConcurrent atomic.Int32
		for i := 0; i < 3; i++ {
			ts.AddTask(&tasks.Task{
				Name: "unlimited",
				Run: func(_ context.Context) (any, error) {
					cur := running.Add(1)
					for {
						old := maxConcurrent.Load()
						if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
							break
						}
					}
					time.Sleep(50 * time.Millisecond)
					running.Add(-1)
					return nil, nil
				},
			})
		}
		ts.Complete()
		s.Greater(maxConcurrent.Load(), int32(1))
	})

	s.Run("negative limit runs all tasks", func() {
		ts := tasks.NewWithLimit(context.Background(), time.Second, -1)
		for i := 0; i < 3; i++ {
			ts.AddTask(&tasks.Task{
				Name: "unlimited",
				Run: func(_ context.Context) (any, error) {
					return nil, nil
				},
			})
		}
		ts.Complete()
		s.Len(ts.All(), 3)
	})
}

func (s *TasksSuite) TestContextCancellation() {
	s.Run("parent context cancellation propagates to task", func() {
		ctx, cancel := context.WithCancel(context.Background())
		ts := tasks.New(ctx, 5*time.Second)
		cancel()
		result := ts.Execute(&tasks.Task{
			Name: "cancelled",
			Run: func(ctx context.Context) (any, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(5 * time.Second):
					return "should not reach", nil
				}
			},
		})
		s.Equal(resultSnapshot{
			Name: "cancelled",
			Err:  context.Canceled,
		}, snapshot(result))
	})
}

func (s *TasksSuite) TestNewWithLimit() {
	s.Run("limits concurrency to specified value", func() {
		limit := 2
		ts := tasks.NewWithLimit(context.Background(), 5*time.Second, limit)
		var running atomic.Int32
		var maxConcurrent atomic.Int32
		for i := 0; i < 5; i++ {
			ts.AddTask(&tasks.Task{
				Name: "limited",
				Run: func(_ context.Context) (any, error) {
					cur := running.Add(1)
					for {
						old := maxConcurrent.Load()
						if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
							break
						}
					}
					time.Sleep(50 * time.Millisecond)
					running.Add(-1)
					return nil, nil
				},
			})
		}
		ts.Complete()
		s.LessOrEqual(maxConcurrent.Load(), int32(limit))
	})

	s.Run("runs all tasks", func() {
		ts := tasks.NewWithLimit(context.Background(), 5*time.Second, 2)
		for i := 0; i < 5; i++ {
			ts.AddTask(&tasks.Task{
				Name: "limited",
				Run: func(_ context.Context) (any, error) {
					return nil, nil
				},
			})
		}
		ts.Complete()
		s.Len(ts.All(), 5)
	})
}

func (s *TasksSuite) TestCompleteParentContextCancellation() {
	s.Run("Complete returns promptly when parent context is cancelled", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		ts := tasks.New(ctx, 5*time.Second)
		ts.AddTask(&tasks.Task{
			Name: "blocker",
			Run: func(ctx context.Context) (any, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		})
		start := time.Now()
		ts.Complete()
		elapsed := time.Since(start)
		s.Less(elapsed, 2*time.Second)
	})

	s.Run("multiple calls are safe", func() {
		ts := tasks.New(context.Background(), time.Second)
		ts.AddTask(&tasks.Task{
			Name: "quick",
			Run: func(_ context.Context) (any, error) {
				return "ok", nil
			},
		})
		ts.Complete()
		ts.Complete()
		s.Len(ts.All(), 1)
	})
}

func (s *TasksSuite) TestRaceCompleteAddTask() {
	s.Run("concurrent AddTask and Complete do not race", func() {
		for i := 0; i < 1000; i++ {
			ts := tasks.New(context.Background(), 0)
			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				ts.AddTask(&tasks.Task{
					Name: "race",
					Run: func(ctx context.Context) (any, error) {
						return nil, nil
					},
				})
			}()

			go func() {
				defer wg.Done()
				ts.Complete()
			}()

			wg.Wait()
		}
	})
}

func (s *TasksSuite) TestNilContextGuards() {
	s.Run("New with nil context does not panic", func() {
		s.NotPanics(func() {
			ts := tasks.New(nil, time.Second) //nolint:staticcheck // intentionally testing nil context guard
			s.NotNil(ts)
		})
	})

	s.Run("NewWithLimit with nil context does not panic", func() {
		s.NotPanics(func() {
			ts := tasks.NewWithLimit(nil, time.Second, 2) //nolint:staticcheck // intentionally testing nil context guard
			s.NotNil(ts)
		})
	})
}

func TestTasks(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}
