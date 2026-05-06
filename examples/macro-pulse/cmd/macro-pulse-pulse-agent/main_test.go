package main

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestEnvBool(t *testing.T) {
	t.Setenv("AGORA_PULSE_LOOP", "yes")
	if !envBool("AGORA_PULSE_LOOP") {
		t.Fatalf("expected yes to enable loop")
	}
	t.Setenv("AGORA_PULSE_LOOP", "0")
	if envBool("AGORA_PULSE_LOOP") {
		t.Fatalf("expected 0 to disable loop")
	}
}

func TestRandomPauseWithinRange(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	min := 20 * time.Second
	max := 60 * time.Second
	for i := 0; i < 100; i++ {
		got := randomPause(r, min, max)
		if got < min || got > max {
			t.Fatalf("pause %s outside [%s, %s]", got, min, max)
		}
	}
	if got := randomPause(r, max, min); got != max {
		t.Fatalf("expected equalized pause %s, got %s", max, got)
	}
}

func TestSleepContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := sleepContext(ctx, time.Minute); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("sleep did not cancel promptly: %s", elapsed)
	}
}

func TestLoopSummaryString(t *testing.T) {
	s := newLoopSummary()
	s.recordSessionProposed()
	s.recordSessionCompleted()
	out := s.String()
	for _, want := range []string{"iterations=0", "sessions_proposed=1", "sessions_completed=1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary %q missing %q", out, want)
		}
	}
}
