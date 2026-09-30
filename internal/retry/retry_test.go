package retry

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"task-runner-launcher/internal/logs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRetryTimings(t *testing.T) func() {
	t.Helper()
	origMaxRetryTime := DefaultMaxRetryTime
	origMaxRetries := DefaultMaxRetries
	origWaitTime := DefaultWaitTimeBetweenRetries

	DefaultMaxRetryTime = 100 * time.Millisecond
	DefaultMaxRetries = 3
	DefaultWaitTimeBetweenRetries = 10 * time.Millisecond

	return func() {
		DefaultMaxRetryTime = origMaxRetryTime
		DefaultMaxRetries = origMaxRetries
		DefaultWaitTimeBetweenRetries = origWaitTime
	}
}

func TestUnlimitedRetry(t *testing.T) {
	restoreFn := setRetryTimings(t)
	defer restoreFn()

	tests := []struct {
		name          string
		operationFn   func() (string, error)
		expectedCalls int
		expectError   bool
		expectedValue string
	}{
		{
			name: "succeeds on first try",
			operationFn: func() (string, error) {
				return "success", nil
			},
			expectedCalls: 1,
			expectedValue: "success",
			expectError:   false,
		},
		{
			name: "succeeds after multiple retries",
			operationFn: (func() func() (string, error) {
				count := 0
				return func() (string, error) {
					count++
					if count < 3 {
						return "", errors.New("temporary error")
					}
					return "success after retries", nil
				}
			})(),
			expectedCalls: 3,
			expectedValue: "success after retries",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			trackedFn := func() (string, error) {
				callCount++
				return tt.operationFn()
			}

			result, err := UnlimitedRetry("test-operation", trackedFn)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedValue, result)
			assert.Equal(t, tt.expectedCalls, callCount)
		})
	}
}

func TestUnlimitedRetryWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempted := make(chan struct{})
	done := make(chan error, 1)
	logger := logs.NewLogger(logs.InfoLevel, "")

	go func() {
		_, err := UnlimitedRetryWithContext(ctx, "test-operation", time.Hour, 0, logger, func() (string, error) {
			close(attempted)
			return "", errors.New("temporary error")
		})
		done <- err
	}()

	<-attempted
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("retry did not stop after cancellation")
	}
}

func TestLimitedRetry(t *testing.T) {
	restoreFn := setRetryTimings(t)
	defer restoreFn()

	tests := []struct {
		name          string
		operationFn   func() (string, error)
		expectedCalls int
		expectError   bool
		expectedValue string
	}{
		{
			name: "succeeds on first try",
			operationFn: func() (string, error) {
				return "success", nil
			},
			expectedCalls: 1,
			expectedValue: "success",
			expectError:   false,
		},
		{
			name: "succeeds within retry limits",
			operationFn: (func() func() (string, error) {
				count := 0
				return func() (string, error) {
					count++
					if count < 3 {
						return "", errors.New("dummy error")
					}
					return "success after retries", nil
				}
			})(),
			expectedCalls: 3,
			expectedValue: "success after retries",
			expectError:   false,
		},
		{
			name: "fails after max attempts",
			operationFn: func() (string, error) {
				return "", errors.New("persistent error")
			},
			expectedCalls: DefaultMaxRetries,
			expectError:   true,
			expectedValue: "",
		},
		{
			name: "fails after max retry time",
			operationFn: func() (string, error) {
				time.Sleep(DefaultMaxRetryTime + time.Second)
				return "", errors.New("timeout error")
			},
			expectedCalls: 1,
			expectError:   true,
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			trackedFn := func() (string, error) {
				callCount++
				return tt.operationFn()
			}

			result, err := LimitedRetry("test-operation", trackedFn)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedValue, result)
			assert.Equal(t, tt.expectedCalls, callCount)
		})
	}
}

func TestRetryConfiguration(t *testing.T) {
	tests := []struct {
		name string
		cfg  retryConfig
		fn   func() (string, error)
		want error
	}{
		{
			name: "respects custom max retry time",
			cfg: retryConfig{
				MaxRetryTime:           100 * time.Millisecond,
				MaxAttempts:            0,
				WaitTimeBetweenRetries: time.Millisecond,
			},
			fn: func() (string, error) {
				return "", errors.New("error")
			},
			want: errors.New("gave up retrying operation `test` on reaching max retry time 100ms, last error: error"),
		},
		{
			name: "respects custom max attempts",
			cfg: retryConfig{
				MaxRetryTime:           0,
				MaxAttempts:            2,
				WaitTimeBetweenRetries: time.Millisecond,
			},
			fn: func() (string, error) {
				return "", errors.New("error")
			},
			want: errors.New("gave up retrying operation `test` on reaching max retry attempts 2, last error: error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := retry("test", tt.fn, tt.cfg)
			assert.Error(t, err)
			assert.Equal(t, tt.want.Error(), err.Error())
		})
	}
}

func TestRetryWithDifferentTypes(t *testing.T) {
	t.Run("works with string", func(t *testing.T) {
		result, err := UnlimitedRetry("string-operation", func() (string, error) {
			return "test", nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "test", result)
	})

	t.Run("works with int", func(t *testing.T) {
		result, err := UnlimitedRetry("int-operation", func() (int, error) {
			return 123, nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 123, result)
	})

	type testStruct struct {
		value string
	}

	t.Run("works with struct", func(t *testing.T) {
		result, err := UnlimitedRetry("struct-operation", func() (testStruct, error) {
			return testStruct{value: "test"}, nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "test", result.value)
	})
}

func captureStdout(t *testing.T) func() string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			_ = w.Close()
			os.Stdout = orig
		})
	}
	t.Cleanup(restore)

	return func() string {
		restore()
		out, _ := io.ReadAll(r)
		return string(out)
	}
}

func TestBackoffNextDoublesToCeiling(t *testing.T) {
	b := &Backoff{Base: 100 * time.Millisecond, Max: 500 * time.Millisecond}
	b.rand = func() float64 { return 0.5 }

	expected := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
		500 * time.Millisecond,
	}

	for i, want := range expected {
		got := b.Next()
		assert.Equal(t, want, got, "attempt %d", i+1)
	}
}

func TestBackoffNextJitterBounds(t *testing.T) {
	steps := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}

	tests := []struct {
		name string
		rand float64
	}{
		{name: "rand at 0 gives 0.8x step", rand: 0},
		{name: "rand just below 1 gives ~1.2x step", rand: 0.999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backoff{Base: 100 * time.Millisecond, Max: 500 * time.Millisecond}
			b.rand = func() float64 { return tt.rand }

			factor := 0.8 + 0.4*tt.rand
			for i, step := range steps {
				got := b.Next()
				want := time.Duration(float64(step) * factor)
				if want > 500*time.Millisecond {
					want = 500 * time.Millisecond
				}
				assert.InDelta(t, float64(want), float64(got), float64(2*time.Millisecond), "attempt %d", i+1)
				assert.LessOrEqual(t, got, 500*time.Millisecond)
			}
		})
	}
}

func TestBackoffNextFlatWhenBaseAboveCeiling(t *testing.T) {
	b := &Backoff{Base: time.Second, Max: 100 * time.Millisecond}
	b.rand = func() float64 { return 0.5 }

	for i := 0; i < 3; i++ {
		got := b.Next()
		assert.Equal(t, time.Second, got, "attempt %d", i+1)
	}
}

func TestWaitReturnsCanceledPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Wait(ctx, time.Hour)
	}()

	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Wait did not return promptly after cancellation")
	}
}

func TestUnlimitedRetryWithContextGrowsSpacing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base := 10 * time.Millisecond
	maxWait := 40 * time.Millisecond
	logger := logs.NewLogger(logs.InfoLevel, "")

	var mu sync.Mutex
	var timestamps []time.Time
	attempt := 0

	_, err := UnlimitedRetryWithContext(ctx, "test-operation", base, maxWait, logger, func() (string, error) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		attempt++
		n := attempt
		mu.Unlock()
		if n >= 5 {
			return "done", nil
		}
		return "", errors.New("temporary error")
	})

	require.NoError(t, err)
	require.Len(t, timestamps, 5)

	gaps := make([]time.Duration, 0, 4)
	for i := 1; i < len(timestamps); i++ {
		gaps = append(gaps, timestamps[i].Sub(timestamps[i-1]))
	}

	for k, gap := range gaps {
		if k == 0 {
			continue
		}
		lowerBound := time.Duration(0.8 * float64(min(base<<uint(k), maxWait)))
		assert.GreaterOrEqual(t, gap, lowerBound-15*time.Millisecond, "gap %d too short", k+1)
	}
	assert.Greater(t, gaps[len(gaps)-1], gaps[0])
}

func TestUnlimitedRetryFlatSpacingWhenNoCeiling(t *testing.T) {
	restoreFn := setRetryTimings(t)
	defer restoreFn()
	DefaultWaitTimeBetweenRetries = 50 * time.Millisecond
	DefaultMaxRetryTime = time.Second

	var timestamps []time.Time
	count := 0
	_, err := UnlimitedRetry("test-operation", func() (string, error) {
		timestamps = append(timestamps, time.Now())
		count++
		if count >= 3 {
			return "done", nil
		}
		return "", errors.New("temporary error")
	})

	require.NoError(t, err)
	require.Len(t, timestamps, 3)

	gap1 := timestamps[1].Sub(timestamps[0])
	gap2 := timestamps[2].Sub(timestamps[1])

	assert.InDelta(t, float64(DefaultWaitTimeBetweenRetries), float64(gap1), float64(15*time.Millisecond))
	assert.InDelta(t, float64(DefaultWaitTimeBetweenRetries), float64(gap2), float64(15*time.Millisecond))
}

func TestBackoffZeroValueBeforeFirstNext(t *testing.T) {
	b := &Backoff{Base: 100 * time.Millisecond, Max: 500 * time.Millisecond}

	assert.Equal(t, 0, b.Attempts())
	assert.Equal(t, time.Duration(0), b.Since())
	assert.False(t, b.AtCeiling())
}

func TestBackoffNextStaysAtCeilingAfterManyCalls(t *testing.T) {
	b := &Backoff{Base: 100 * time.Millisecond, Max: 500 * time.Millisecond}
	b.rand = func() float64 { return 0.5 }

	b.Next()
	b.Next()
	b.Next()

	for i := 0; i < 100; i++ {
		got := b.Next()
		assert.Equal(t, 500*time.Millisecond, got, "call %d", i+1)
	}
	assert.True(t, b.AtCeiling())
}

func TestBackoffNextClampsNegativeJitterFactor(t *testing.T) {
	b := &Backoff{Base: 100 * time.Millisecond, Max: 500 * time.Millisecond}
	b.rand = func() float64 { return -10 }

	got := b.Next()

	assert.Equal(t, time.Duration(0), got)
}

func TestWaitReturnsPromptlyForZeroAndNegativeDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
	}{
		{name: "zero duration", d: 0},
		{name: "negative duration", d: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			done := make(chan error, 1)
			go func() { done <- Wait(ctx, tt.d) }()

			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(50 * time.Millisecond):
				t.Fatal("Wait did not return promptly")
			}
		})
	}
}

func TestUnlimitedRetryWithContextLogsCeilingOnce(t *testing.T) {
	readOutput := captureStdout(t)
	logger := logs.NewLogger(logs.InfoLevel, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	count := 0
	_, err := UnlimitedRetryWithContext(ctx, "test-operation", 10*time.Millisecond, 40*time.Millisecond, logger, func() (string, error) {
		count++
		if count >= 5 {
			return "done", nil
		}
		return "", errors.New("temporary error")
	})
	output := readOutput()

	require.NoError(t, err)

	warnLines := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "WARN") {
			warnLines++
		}
	}
	assert.Equal(t, 1, warnLines, "reaching the ceiling should log exactly one WARN line")
	assert.Contains(t, output, "attempt")
	assert.Contains(t, output, "failing for")
}

func TestBackoffNoCeilingLogWhenBaseAtOrAboveMax(t *testing.T) {
	readOutput := captureStdout(t)
	logger := logs.NewLogger(logs.InfoLevel, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count := 0
	_, err := UnlimitedRetryWithContext(ctx, "test-operation", 200*time.Millisecond, 100*time.Millisecond, logger, func() (string, error) {
		count++
		if count >= 3 {
			return "done", nil
		}
		return "", errors.New("temporary error")
	})
	output := readOutput()

	require.NoError(t, err)
	assert.NotContains(t, output, "WARN")
}
