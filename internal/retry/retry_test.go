package retry

import (
	"errors"
	"testing"
	"time"
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

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expectedValue {
				t.Errorf("expected value %v but got %v", tt.expectedValue, result)
			}

			if callCount != tt.expectedCalls {
				t.Errorf("expected %d calls but got %d", tt.expectedCalls, callCount)
			}
		})
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

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expectedValue {
				t.Errorf("expected value %v but got %v", tt.expectedValue, result)
			}

			if callCount != tt.expectedCalls {
				t.Errorf("expected %d calls but got %d", tt.expectedCalls, callCount)
			}
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
			if err == nil {
				t.Fatal("expected error but got nil")
			}

			if err.Error() != tt.want.Error() {
				t.Errorf("got error %q, want %q", err, tt.want)
			}
		})
	}
}

func TestRetryWithDifferentTypes(t *testing.T) {
	t.Run("works with string", func(t *testing.T) {
		result, err := UnlimitedRetry("string-operation", func() (string, error) {
			return "test", nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result != "test" {
			t.Errorf("expected string \"test\" but got %s", result)
		}
	})

	t.Run("works with int", func(t *testing.T) {
		result, err := UnlimitedRetry("int-operation", func() (int, error) {
			return 123, nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result != 123 {
			t.Errorf("expected int 123 but got %d", result)
		}
	})

	type testStruct struct {
		value string
	}

	t.Run("works with struct", func(t *testing.T) {
		result, err := UnlimitedRetry("struct-operation", func() (testStruct, error) {
			return testStruct{value: "test"}, nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result.value != "test" {
			t.Errorf("expected 'test' in struct.value but got %s", result.value)
		}
	})
}
