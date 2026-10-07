package retry

import (
	"context"
	"errors"
	"io"
	"math"
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

func TestBackoffAdvance(t *testing.T) {
	ms := time.Millisecond

	tests := []struct {
		name          string
		base          time.Duration
		max           time.Duration
		rand          float64
		want          []time.Duration
		calls         int
		wantCeilingAt int
	}{
		{
			name:          "doubles to ceiling",
			base:          100 * ms,
			max:           500 * ms,
			rand:          0.5,
			want:          []time.Duration{100 * ms, 200 * ms, 400 * ms, 500 * ms, 500 * ms},
			wantCeilingAt: 4,
		},
		{
			name:          "step lands exactly on ceiling",
			base:          100 * ms,
			max:           400 * ms,
			rand:          0.5,
			want:          []time.Duration{100 * ms, 200 * ms, 400 * ms, 400 * ms},
			wantCeilingAt: 3,
		},
		{
			name: "flat when base equals max",
			base: 100 * ms,
			max:  100 * ms,
			rand: 0.5,
			want: []time.Duration{100 * ms, 100 * ms, 100 * ms},
		},
		{
			name: "flat at base when base above max",
			base: time.Second,
			max:  100 * ms,
			rand: 0.5,
			want: []time.Duration{time.Second, time.Second, time.Second},
		},
		{
			name:          "low jitter gives 0.8x step",
			base:          100 * ms,
			max:           500 * ms,
			rand:          0,
			want:          []time.Duration{80 * ms, 160 * ms, 320 * ms, 400 * ms, 400 * ms},
			wantCeilingAt: 4,
		},
		{
			name:          "high jitter gives 1.2x step clamped to ceiling",
			base:          100 * ms,
			max:           500 * ms,
			rand:          0.999999,
			want:          []time.Duration{120 * ms, 240 * ms, 480 * ms, 500 * ms, 500 * ms},
			wantCeilingAt: 4,
		},
		{
			name:          "negative jitter factor clamps to zero",
			base:          100 * ms,
			max:           500 * ms,
			rand:          -10,
			want:          []time.Duration{0, 0, 0, 0, 0},
			wantCeilingAt: 4,
		},
		{
			name: "largest duration ceiling does not overflow",
			base: math.MaxInt64,
			max:  math.MaxInt64,
			rand: 0.5,
			want: []time.Duration{math.MaxInt64, math.MaxInt64},
		},
		{
			name:          "long streak stays at ceiling",
			base:          100 * ms,
			max:           500 * ms,
			rand:          0.5,
			want:          []time.Duration{100 * ms, 200 * ms, 400 * ms, 500 * ms},
			calls:         2000,
			wantCeilingAt: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backoff{Base: tt.base, Max: tt.max, rand: func() float64 { return tt.rand }}
			calls := max(tt.calls, len(tt.want))

			for i := range calls {
				call := i + 1
				want := tt.want[min(i, len(tt.want)-1)]

				got, justReachedCeiling := b.Advance(time.Now())

				assert.InDelta(t, float64(want), float64(got), float64(time.Microsecond), "call %d", call)
				assert.Equal(t, call == tt.wantCeilingAt, justReachedCeiling, "call %d", call)
				assert.Equal(t, call, b.Attempts())
			}
			assert.True(t, b.atCeiling())
		})
	}
}

func TestBackoffResetAndClock(t *testing.T) {
	b := &Backoff{Base: 100 * time.Millisecond, Max: 400 * time.Millisecond, rand: func() float64 { return 0.5 }}
	now := time.Now()

	assert.Equal(t, 0, b.Attempts())
	assert.Equal(t, time.Duration(0), b.Since())
	assert.False(t, b.atCeiling())

	b.Advance(now.Add(-time.Hour))

	assert.GreaterOrEqual(t, b.Since(), time.Hour)
	assert.Less(t, b.Since(), 2*time.Hour)

	b.Advance(now.Add(-3 * time.Hour))
	_, justReachedCeiling := b.Advance(now)

	assert.Less(t, b.Since(), 2*time.Hour)
	assert.True(t, justReachedCeiling)

	b.Reset()

	assert.Equal(t, 0, b.Attempts())
	assert.Equal(t, time.Duration(0), b.Since())

	d, justReachedCeiling := b.Advance(now.Add(-2 * time.Hour))

	assert.InDelta(t, float64(100*time.Millisecond), float64(d), float64(time.Microsecond))
	assert.False(t, justReachedCeiling)
	assert.Equal(t, 1, b.Attempts())
	assert.GreaterOrEqual(t, b.Since(), 2*time.Hour)

	b.Advance(now)
	_, justReachedCeiling = b.Advance(now)

	assert.True(t, justReachedCeiling)
}

func TestWait(t *testing.T) {
	tests := []struct {
		name    string
		d       time.Duration
		cancel  bool
		wantErr error
	}{
		{name: "zero duration returns", d: 0},
		{name: "negative duration returns", d: -time.Second},
		{name: "cancelled context interrupts", d: time.Hour, cancel: true, wantErr: context.Canceled},
		{name: "cancelled context wins over zero duration", d: 0, cancel: true, wantErr: context.Canceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if tt.cancel {
				cancel()
			}

			err := Wait(ctx, tt.d)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUnlimitedRetryWithContextBackoff(t *testing.T) {
	ms := time.Millisecond

	tests := []struct {
		name     string
		base     time.Duration
		max      time.Duration
		steps    []time.Duration
		wantWarn bool
	}{
		{
			name:     "growing delay warns once on reaching the ceiling",
			base:     10 * ms,
			max:      40 * ms,
			steps:    []time.Duration{10 * ms, 20 * ms, 40 * ms, 40 * ms},
			wantWarn: true,
		},
		{
			name:  "base above max stays flat without warning",
			base:  20 * ms,
			max:   10 * ms,
			steps: []time.Duration{20 * ms, 20 * ms, 20 * ms, 20 * ms},
		},
		{
			name:  "base equal to max stays flat without warning",
			base:  10 * ms,
			max:   10 * ms,
			steps: []time.Duration{10 * ms, 10 * ms, 10 * ms, 10 * ms},
		},
		{
			name:  "no max stays flat without warning",
			base:  10 * ms,
			max:   0,
			steps: []time.Duration{10 * ms, 10 * ms, 10 * ms, 10 * ms},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readOutput := captureStdout(t)
			logger := logs.NewLogger(logs.InfoLevel, "")
			calls := 0
			operation := func() (string, error) {
				calls++
				if calls > len(tt.steps) {
					return "done", nil
				}
				return "", errors.New("temporary error")
			}
			var sum time.Duration
			for _, step := range tt.steps {
				sum += step
			}

			start := time.Now()
			result, err := UnlimitedRetryWithContext(context.Background(), "test-operation", tt.base, tt.max, logger, operation)
			elapsed := time.Since(start)
			output := readOutput()

			require.NoError(t, err)
			assert.Equal(t, "done", result)
			assert.Equal(t, len(tt.steps)+1, calls)
			assert.GreaterOrEqual(t, elapsed, time.Duration(0.8*float64(sum)))
			if tt.wantWarn {
				assert.Equal(t, 1, strings.Count(output, "WARN"))
				assert.Contains(t, output, "Operation `test-operation` retrying at ceiling: attempt 3,")
				assert.Contains(t, output, "failing for")
				assert.Contains(t, output, "retry interval")
				assert.Contains(t, output, "last error: temporary error")
			} else {
				assert.NotContains(t, output, "WARN")
			}
		})
	}
}

func TestRoundForLog(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: 0, want: "0s"},
		{in: 400*time.Millisecond + 600*time.Microsecond, want: "401ms"},
		{in: 999 * time.Millisecond, want: "999ms"},
		{in: time.Second, want: "1s"},
		{in: 12*time.Second + 400*time.Millisecond, want: "12s"},
		{in: 90 * time.Second, want: "1m30s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, RoundForLog(tt.in).String())
		})
	}
}
