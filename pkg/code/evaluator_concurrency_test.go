package code

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// EvaluatorConcurrencySuite tests thread-safety of the evaluator.
type EvaluatorConcurrencySuite struct {
	BaseEvaluatorSuite
}

func (s *EvaluatorConcurrencySuite) TestConcurrentEvaluators() {
	s.Run("multiple evaluators can run concurrently without interference", func() {
		const numGoroutines = 10
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)
		results := make(chan string, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				evaluator, err := NewEvaluator(context.Background(), s.kubernetes)
				if err != nil {
					errors <- err
					return
				}

				// Each goroutine runs a different script
				script := `"result-" + ` + strconv.Itoa(id)
				result, err := evaluator.Evaluate(script, time.Second)
				if err != nil {
					errors <- err
					return
				}
				results <- result
			}(i)
		}

		wg.Wait()
		close(errors)
		close(results)

		for err := range errors {
			s.NoError(err)
		}

		count := 0
		for range results {
			count++
		}
		s.Equal(numGoroutines, count, "all goroutines should complete successfully")
	})

	s.Run("evaluators sharing kubernetes client work correctly", func() {
		const numGoroutines = 20
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)

		evaluators := make([]*Evaluator, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			var err error
			evaluators[i], err = NewEvaluator(context.Background(), s.kubernetes)
			s.Require().NoError(err)
		}

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := evaluators[idx].Evaluate(`namespace`, time.Second)
				if err != nil {
					errors <- err
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			s.NoError(err)
		}
	})
}

func (s *EvaluatorConcurrencySuite) TestConcurrentTimeouts() {
	s.Run("multiple timeouts are handled independently", func() {
		const numGoroutines = 5
		var wg sync.WaitGroup
		timeoutErrors := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				evaluator, err := NewEvaluator(context.Background(), s.kubernetes)
				if err != nil {
					return
				}

				_, err = evaluator.Evaluate(`while(true) {}`, 50*time.Millisecond)
				if err != nil {
					timeoutErrors <- true
				}
			}()
		}

		wg.Wait()
		close(timeoutErrors)

		count := 0
		for range timeoutErrors {
			count++
		}
		s.Equal(numGoroutines, count, "all concurrent scripts should timeout independently")
	})
}

func (s *EvaluatorConcurrencySuite) TestDataRaceSafety() {
	s.Run("repeated namespace access has no data race", func() {
		const numGoroutines = 10
		var wg sync.WaitGroup

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				evaluator, err := NewEvaluator(context.Background(), s.kubernetes)
				if err != nil {
					return
				}

				for j := 0; j < 10; j++ {
					_, _ = evaluator.Evaluate(`namespace`, time.Second)
				}
			}()
		}

		wg.Wait()
	})

	s.Run("concurrent k8s client access is safe", func() {
		const numGoroutines = 10
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				evaluator, err := NewEvaluator(context.Background(), s.kubernetes)
				if err != nil {
					errors <- err
					return
				}

				_, err = evaluator.Evaluate(`typeof k8s.coreV1`, time.Second)
				if err != nil {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			s.NoError(err)
		}
	})
}

func TestEvaluatorConcurrency(t *testing.T) {
	suite.Run(t, new(EvaluatorConcurrencySuite))
}
