package database

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/protengplus/proteng-conductor/internal/logger"
)

func TestMain(m *testing.M) {
	logger.InitZap()
	os.Exit(m.Run())
}

func TestConnectWithRetry_SucceedsFirstTry(t *testing.T) {
	calls := 0
	connect := func() error {
		calls++
		return nil
	}

	err := ConnectWithRetry(connect, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestConnectWithRetry_SucceedsAfterFailures(t *testing.T) {
	calls := 0
	connect := func() error {
		calls++
		if calls < 3 {
			return errors.New("connection refused")
		}
		return nil
	}

	err := ConnectWithRetry(connect, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestConnectWithRetry_FailsAfterMaxAttempts(t *testing.T) {
	calls := 0
	connect := func() error {
		calls++
		return errors.New("connection refused")
	}

	err := ConnectWithRetry(connect, 3, time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}
