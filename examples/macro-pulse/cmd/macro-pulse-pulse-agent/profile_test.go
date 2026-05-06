package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
)

func TestLoadActivityProfile(t *testing.T) {
	path := writeProfileFile(t, `
pause:
  distribution: normal
  mean: 45s
  stddev: 5s
  min: 30s
  max: 60s
advertisement_count:
  distribution: uniform
  min: 2
  max: 4
outcomes:
  runtime_violation: 0.01
  reaper_violation: 0.04
  long_tail: 0.25
workgroups:
  - signals-channel
`)

	profile, err := loadActivityProfile(path)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if profile.Pause.Distribution != "normal" || profile.Pause.Mean != 45*time.Second || profile.Pause.Stddev != 5*time.Second {
		t.Fatalf("unexpected pause profile: %+v", profile.Pause)
	}
	if profile.AdvertisementCount.Min != 2 || profile.AdvertisementCount.Max != 4 {
		t.Fatalf("unexpected advertisement count profile: %+v", profile.AdvertisementCount)
	}
	if profile.Outcomes.RuntimeViolation != 0.01 || profile.Outcomes.ReaperViolation != 0.04 || profile.Outcomes.LongTail != 0.25 {
		t.Fatalf("unexpected outcome profile: %+v", profile.Outcomes)
	}
	if len(profile.Workgroups) != 1 || profile.Workgroups[0] != "signals-channel" {
		t.Fatalf("unexpected workgroups: %+v", profile.Workgroups)
	}
}

func TestExampleProfilesLoad(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "..", "..", "etc", "demo-profile.yaml"),
		filepath.Join("..", "..", "..", "..", "etc", "demo-profile-fast.yaml"),
	} {
		profile, err := loadActivityProfile(path)
		if err != nil {
			t.Fatalf("load example profile %s: %v", path, err)
		}
		if profile.AdvertisementCount.Min <= 0 || profile.AdvertisementCount.Max < profile.AdvertisementCount.Min {
			t.Fatalf("example profile %s has invalid advertisement count: %+v", path, profile.AdvertisementCount)
		}
	}
}

func TestLoadActivityProfileRejectsInvalidProbabilities(t *testing.T) {
	path := writeProfileFile(t, `
outcomes:
  runtime_violation: 0.50
  reaper_violation: 0.40
  long_tail: 0.20
`)
	if _, err := loadActivityProfile(path); err == nil {
		t.Fatal("expected invalid outcome probability error")
	}
}

func TestSampleDistributions(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for i := 0; i < 100; i++ {
		got := sampleDurationDistribution(r, durationDistribution{
			Distribution: "normal",
			Mean:         45 * time.Second,
			Stddev:       20 * time.Second,
			Min:          30 * time.Second,
			Max:          60 * time.Second,
		})
		if got < 30*time.Second || got > 60*time.Second {
			t.Fatalf("duration %s outside normal bounds", got)
		}
	}

	if got := sampleCountDistribution(r, countDistribution{}, 7); got != 7 {
		t.Fatalf("empty count distribution should select all ads, got %d", got)
	}
	for i := 0; i < 100; i++ {
		got := sampleCountDistribution(r, countDistribution{Distribution: "uniform", Min: 2, Max: 4}, 8)
		if got < 2 || got > 4 {
			t.Fatalf("count %d outside uniform bounds", got)
		}
	}
	if got := sampleCountDistribution(r, countDistribution{Distribution: "uniform", Min: 2, Max: 4}, 1); got != 1 {
		t.Fatalf("count should cap to available advertisements, got %d", got)
	}
}

func TestClassifySessionOutcomeForProfile(t *testing.T) {
	probabilities := outcomeProbabilities{RuntimeViolation: 0.10, ReaperViolation: 0.20, LongTail: 0.30}
	tests := []struct {
		p    float64
		want sessionOutcome
	}{
		{p: 0.09, want: sessionOutcomeRuntimeViolation},
		{p: 0.10, want: sessionOutcomeReaperViolation},
		{p: 0.29, want: sessionOutcomeReaperViolation},
		{p: 0.31, want: sessionOutcomeLongTail},
		{p: 0.59, want: sessionOutcomeLongTail},
		{p: 0.61, want: sessionOutcomeNormal},
	}
	for _, tt := range tests {
		if got := classifySessionOutcomeForProfile(tt.p, probabilities); got != tt.want {
			t.Fatalf("classifySessionOutcomeForProfile(%f) = %s, want %s", tt.p, got, tt.want)
		}
	}
}

func TestSelectAdvertisementsFiltersWorkgroupsAndCounts(t *testing.T) {
	selector := &loopOutcomeSelector{
		rand: rand.New(rand.NewSource(3)),
		profile: &activityProfile{
			AdvertisementCount: countDistribution{Distribution: "uniform", Min: 2, Max: 2},
		},
		workgroups: map[string]struct{}{"wg_signals": {}},
	}
	ads := []api.Advertisement{
		{ID: "ad_1", WorkgroupScopes: []string{"wg_markets"}},
		{ID: "ad_2", WorkgroupScopes: []string{"wg_signals"}},
		{ID: "ad_3", WorkgroupScopes: []string{"wg_signals"}},
		{ID: "ad_4", WorkgroupScopes: []string{"wg_signals"}},
	}

	selected := selector.selectAdvertisements(ads)
	if len(selected) != 2 {
		t.Fatalf("selected %d advertisements, want 2: %+v", len(selected), selected)
	}
	for _, ad := range selected {
		if !advertisementInWorkgroups(ad, selector.workgroups) {
			t.Fatalf("selected advertisement outside workgroup filter: %+v", ad)
		}
	}
}

func writeProfileFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}
