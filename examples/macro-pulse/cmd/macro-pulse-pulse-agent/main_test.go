package main

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
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

func TestClassifySessionOutcome(t *testing.T) {
	tests := []struct {
		p    float64
		want sessionOutcome
	}{
		{p: 0.000, want: sessionOutcomeRuntimeViolation},
		{p: 0.024, want: sessionOutcomeRuntimeViolation},
		{p: 0.025, want: sessionOutcomeReaperViolation},
		{p: 0.049, want: sessionOutcomeReaperViolation},
		{p: 0.050, want: sessionOutcomeLongTail},
		{p: 0.199, want: sessionOutcomeLongTail},
		{p: 0.200, want: sessionOutcomeNormal},
		{p: 0.950, want: sessionOutcomeNormal},
	}
	for _, tt := range tests {
		if got := classifySessionOutcome(tt.p); got != tt.want {
			t.Fatalf("classifySessionOutcome(%f) = %s, want %s", tt.p, got, tt.want)
		}
	}
}

func TestQueryOptionsForOutcomeTargetsTightAdvertisement(t *testing.T) {
	contract := &api.Contract{MaxDurationSeconds: 30, MaxEnvelopeBytes: 1024}

	got := queryOptionsForOutcome("tight", "tight", sessionOutcomeRuntimeViolation, contract, rand.New(rand.NewSource(1)))
	if got.outcome != sessionOutcomeRuntimeViolation || got.oversizePayloadBytes < 4096 {
		t.Fatalf("tight runtime options not configured: %+v", got)
	}

	got = queryOptionsForOutcome("default", "tight", sessionOutcomeRuntimeViolation, contract, rand.New(rand.NewSource(1)))
	if got.outcome != sessionOutcomeNormal {
		t.Fatalf("non-tight runtime outcome should normalize to normal, got %+v", got)
	}

	got = queryOptionsForOutcome("tight", "tight", sessionOutcomeReaperViolation, contract, rand.New(rand.NewSource(1)))
	if got.outcome != sessionOutcomeReaperViolation || got.reaperHold < time.Minute {
		t.Fatalf("tight reaper options not configured: %+v", got)
	}

	got = queryOptionsForOutcome("tight", "tight", sessionOutcomeLongTail, contract, rand.New(rand.NewSource(1)))
	if got.outcome != sessionOutcomeNormal {
		t.Fatalf("tight long-tail outcome should normalize to normal, got %+v", got)
	}

	defaultContract := &api.Contract{MaxDurationSeconds: 600}
	got = queryOptionsForOutcome("default", "tight", sessionOutcomeLongTail, defaultContract, rand.New(rand.NewSource(1)))
	if got.outcome != sessionOutcomeLongTail || got.holdAfterReply < time.Minute || got.holdAfterReply > 5*time.Minute {
		t.Fatalf("default long-tail options outside expected range: %+v", got)
	}
}

func TestLongTailHoldRespectsDurationCap(t *testing.T) {
	r := rand.New(rand.NewSource(17))
	contract := &api.Contract{MaxDurationSeconds: 60}
	for i := 0; i < 100; i++ {
		got := randomLongTailHold(r, contract)
		if got <= 0 || got > 45*time.Second {
			t.Fatalf("long-tail hold %s should stay below 60s cap with 15s margin", got)
		}
	}
	if got := randomLongTailHold(r, &api.Contract{MaxDurationSeconds: 10}); got != 0 {
		t.Fatalf("expected no safe long-tail hold under tiny cap, got %s", got)
	}
}

func TestWarmupHoldDurations(t *testing.T) {
	if got := holdPastDurationCap(&api.Contract{MaxDurationSeconds: 30}); got != time.Minute {
		t.Fatalf("tight hold = %s, want %s", got, time.Minute)
	}
	if got := holdPastDurationCap(&api.Contract{MaxDurationSeconds: 120}); got != 140*time.Second {
		t.Fatalf("large-cap hold = %s, want %s", got, 140*time.Second)
	}
	if got := warmupLongTailHold(&api.Contract{MaxDurationSeconds: 600}); got != time.Minute {
		t.Fatalf("warm-up long-tail hold = %s, want %s", got, time.Minute)
	}
	if got := warmupLongTailHold(&api.Contract{MaxDurationSeconds: 60}); got != 45*time.Second {
		t.Fatalf("warm-up long-tail capped hold = %s, want %s", got, 45*time.Second)
	}
	if got := warmupLongTailHold(&api.Contract{MaxDurationSeconds: 10}); got != 0 {
		t.Fatalf("warm-up long-tail tiny cap = %s, want 0", got)
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
