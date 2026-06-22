package forwarder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tbxark/air780e-sms-forwarder/internal/config"
	serial "go.bug.st/serial"
)

func TestReconnectLoopRetriesAfterClosedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	waits := 0
	errClosed := errors.New("closed")
	err := runReconnectLoopWithOptions(ctx, config.Config{}, &reconnectableExecutor{}, nil, reconnectLoopOptions{
		initialBackoff: time.Second,
		maxBackoff:     4 * time.Second,
		runSession: func(context.Context, config.Config, *reconnectableExecutor, *telegramSender) error {
			runs++
			if runs == 2 {
				cancel()
			}
			return errClosed
		},
		wait: func(context.Context, time.Duration) error {
			waits++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runReconnectLoopWithOptions returned error: %v", err)
	}
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
	if waits != 1 {
		t.Fatalf("waits = %d, want 1", waits)
	}
}

func TestReconnectLoopRespectsContextCancellationDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	err := runReconnectLoopWithOptions(ctx, config.Config{}, &reconnectableExecutor{}, nil, reconnectLoopOptions{
		initialBackoff: time.Second,
		maxBackoff:     4 * time.Second,
		runSession: func(context.Context, config.Config, *reconnectableExecutor, *telegramSender) error {
			runs++
			return errSerialSessionClosed
		},
		wait: func(context.Context, time.Duration) error {
			cancel()
			return context.Canceled
		},
	})
	if err != nil {
		t.Fatalf("runReconnectLoopWithOptions returned error: %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}

func TestNextBackoffCapsAtMax(t *testing.T) {
	if got := nextBackoff(2*time.Second, 30*time.Second); got != 4*time.Second {
		t.Fatalf("nextBackoff = %s, want 4s", got)
	}
	if got := nextBackoff(20*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff = %s, want 30s", got)
	}
	if got := nextBackoff(30*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff = %s, want 30s", got)
	}
}

func TestReconnectLoopResetsBackoffAfterConnectedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := []error{errors.New("open failed"), errors.New("init failed"), errSerialSessionClosed, errors.New("open failed again")}
	var waits []time.Duration
	err := runReconnectLoopWithOptions(ctx, config.Config{}, &reconnectableExecutor{}, nil, reconnectLoopOptions{
		initialBackoff: time.Second,
		maxBackoff:     4 * time.Second,
		runSession: func(context.Context, config.Config, *reconnectableExecutor, *telegramSender) error {
			err := errs[0]
			errs = errs[1:]
			if len(errs) == 0 {
				cancel()
			}
			return err
		},
		wait: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runReconnectLoopWithOptions returned error: %v", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, time.Second}
	if len(waits) != len(want) {
		t.Fatalf("waits = %v, want %v", waits, want)
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("waits = %v, want %v", waits, want)
		}
	}
}

func TestRunSerialSessionAutoDetectsEachAttempt(t *testing.T) {
	originalAutoDetect := autoDetectSerialPort
	originalOpen := openSerialPort
	autoDetectCalls := 0
	autoDetectSerialPort = func(int) (string, error) {
		autoDetectCalls++
		return "/dev/ttyUSB-test", nil
	}
	openSerialPort = func(string, int) (serial.Port, error) {
		return nil, errors.New("open failed")
	}
	defer func() {
		autoDetectSerialPort = originalAutoDetect
		openSerialPort = originalOpen
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	err := runReconnectLoopWithOptions(ctx, config.Config{Baud: 115200}, &reconnectableExecutor{}, nil, reconnectLoopOptions{
		initialBackoff: time.Second,
		maxBackoff:     time.Second,
		runSession: func(ctx context.Context, cfg config.Config, executor *reconnectableExecutor, sender *telegramSender) error {
			runs++
			if runs == 2 {
				cancel()
			}
			return runSerialSession(ctx, cfg, executor, sender)
		},
		wait: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("runReconnectLoopWithOptions returned error: %v", err)
	}
	if autoDetectCalls != 2 {
		t.Fatalf("autoDetectCalls = %d, want 2", autoDetectCalls)
	}
}
