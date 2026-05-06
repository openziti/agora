package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
)

type activityProfile struct {
	Pause              durationDistribution
	AdvertisementCount countDistribution
	Outcomes           outcomeProbabilities
	Workgroups         []string
}

type durationDistribution struct {
	Distribution string
	Min          time.Duration
	Max          time.Duration
	Mean         time.Duration
	Stddev       time.Duration
}

type countDistribution struct {
	Distribution string
	Min          int
	Max          int
	Mean         float64
	Stddev       float64
}

type outcomeProbabilities struct {
	RuntimeViolation float64
	ReaperViolation  float64
	LongTail         float64
}

func defaultActivityProfile(pauseMin, pauseMax time.Duration) *activityProfile {
	return &activityProfile{
		Pause: durationDistribution{
			Distribution: "uniform",
			Min:          pauseMin,
			Max:          pauseMax,
		},
		Outcomes: defaultOutcomeProbabilities(),
	}
}

func defaultOutcomeProbabilities() outcomeProbabilities {
	return outcomeProbabilities{
		RuntimeViolation: 0.025,
		ReaperViolation:  0.025,
		LongTail:         0.15,
	}
}

func loadActivityProfile(path string) (*activityProfile, error) {
	profile := defaultActivityProfile(20*time.Second, 60*time.Second)
	if err := dd.MergeYAMLFile(profile, path, &dd.Options{}); err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	if err := profile.validate(); err != nil {
		return nil, fmt.Errorf("load profile %q: %w", path, err)
	}
	return profile, nil
}

func (p *activityProfile) validate() error {
	if p == nil {
		return fmt.Errorf("profile is nil")
	}
	if err := p.Pause.validateDuration("pause"); err != nil {
		return err
	}
	if err := p.AdvertisementCount.validateCount("advertisement_count"); err != nil {
		return err
	}
	return p.Outcomes.validate()
}

func (d durationDistribution) validateDuration(name string) error {
	kind := distributionKind(d.Distribution)
	switch kind {
	case "uniform":
		if d.Min <= 0 {
			return fmt.Errorf("%s.min must be > 0", name)
		}
		if d.Max < d.Min {
			return fmt.Errorf("%s.max must be >= %s.min", name, name)
		}
	case "normal":
		if d.Mean <= 0 {
			return fmt.Errorf("%s.mean must be > 0 for normal distribution", name)
		}
		if d.Stddev <= 0 {
			return fmt.Errorf("%s.stddev must be > 0 for normal distribution", name)
		}
		if d.Min > 0 && d.Max > 0 && d.Max < d.Min {
			return fmt.Errorf("%s.max must be >= %s.min", name, name)
		}
	case "exponential":
		if d.Mean <= 0 {
			return fmt.Errorf("%s.mean must be > 0 for exponential distribution", name)
		}
		if d.Min > 0 && d.Max > 0 && d.Max < d.Min {
			return fmt.Errorf("%s.max must be >= %s.min", name, name)
		}
	default:
		return fmt.Errorf("%s.distribution must be uniform, normal, or exponential", name)
	}
	return nil
}

func (d countDistribution) validateCount(name string) error {
	if d.Min == 0 && d.Max == 0 && d.Mean == 0 {
		return nil
	}
	kind := distributionKind(d.Distribution)
	switch kind {
	case "uniform":
		if d.Min <= 0 {
			return fmt.Errorf("%s.min must be > 0", name)
		}
		if d.Max < d.Min {
			return fmt.Errorf("%s.max must be >= %s.min", name, name)
		}
	case "normal":
		if d.Mean <= 0 {
			return fmt.Errorf("%s.mean must be > 0 for normal distribution", name)
		}
		if d.Stddev <= 0 {
			return fmt.Errorf("%s.stddev must be > 0 for normal distribution", name)
		}
		if d.Min > 0 && d.Max > 0 && d.Max < d.Min {
			return fmt.Errorf("%s.max must be >= %s.min", name, name)
		}
	case "exponential":
		if d.Mean <= 0 {
			return fmt.Errorf("%s.mean must be > 0 for exponential distribution", name)
		}
		if d.Min > 0 && d.Max > 0 && d.Max < d.Min {
			return fmt.Errorf("%s.max must be >= %s.min", name, name)
		}
	default:
		return fmt.Errorf("%s.distribution must be uniform, normal, or exponential", name)
	}
	return nil
}

func (p outcomeProbabilities) validate() error {
	for name, value := range map[string]float64{
		"outcomes.runtime_violation": p.RuntimeViolation,
		"outcomes.reaper_violation":  p.ReaperViolation,
		"outcomes.long_tail":         p.LongTail,
	} {
		if value < 0 || value > 1 {
			return fmt.Errorf("%s must be between 0 and 1", name)
		}
	}
	if sum := p.RuntimeViolation + p.ReaperViolation + p.LongTail; sum > 1 {
		return fmt.Errorf("outcome probabilities must sum to <= 1, got %.3f", sum)
	}
	return nil
}

func distributionKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return "uniform"
	}
	return kind
}

func sampleDurationDistribution(r *rand.Rand, d durationDistribution) time.Duration {
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	switch distributionKind(d.Distribution) {
	case "normal":
		sampled := time.Duration(math.Round(float64(d.Mean) + r.NormFloat64()*float64(d.Stddev)))
		return clampDuration(sampled, d.Min, d.Max)
	case "exponential":
		sampled := time.Duration(math.Round(r.ExpFloat64() * float64(d.Mean)))
		return clampDuration(sampled, d.Min, d.Max)
	default:
		return randomPause(r, d.Min, d.Max)
	}
}

func sampleCountDistribution(r *rand.Rand, d countDistribution, available int) int {
	if available <= 0 {
		return 0
	}
	if d.Min == 0 && d.Max == 0 && d.Mean == 0 {
		return available
	}
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	var sampled int
	switch distributionKind(d.Distribution) {
	case "normal":
		sampled = int(math.Round(d.Mean + r.NormFloat64()*d.Stddev))
	case "exponential":
		sampled = int(math.Ceil(r.ExpFloat64() * d.Mean))
	default:
		max := d.Max
		if max <= 0 || max > available {
			max = available
		}
		min := d.Min
		if min <= 0 {
			min = 1
		}
		if min > max {
			min = max
		}
		sampled = min + r.Intn(max-min+1)
	}

	min := d.Min
	if min <= 0 {
		min = 1
	}
	max := d.Max
	if max <= 0 || max > available {
		max = available
	}
	if min > max {
		min = max
	}
	return clampInt(sampled, min, max)
}

func clampDuration(value, min, max time.Duration) time.Duration {
	if min > 0 && value < min {
		return min
	}
	if max > 0 && value > max {
		return max
	}
	if value <= 0 {
		return min
	}
	return value
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func classifySessionOutcomeForProfile(p float64, probabilities outcomeProbabilities) sessionOutcome {
	if p < probabilities.RuntimeViolation {
		return sessionOutcomeRuntimeViolation
	}
	if p < probabilities.RuntimeViolation+probabilities.ReaperViolation {
		return sessionOutcomeReaperViolation
	}
	if p < probabilities.RuntimeViolation+probabilities.ReaperViolation+probabilities.LongTail {
		return sessionOutcomeLongTail
	}
	return sessionOutcomeNormal
}

func resolveProfileWorkgroups(ctx context.Context, a *agent.Agent, profile *activityProfile) (map[string]struct{}, error) {
	if profile == nil || len(profile.Workgroups) == 0 {
		return nil, nil
	}
	res, err := a.Controller().ListWorkgroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workgroups for profile: %w", err)
	}
	listing, ok := res.(*api.ListWorkgroupsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected list workgroups response: %T", res)
	}

	resolved := map[string]struct{}{}
	for _, token := range profile.Workgroups {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		matches := make([]api.Workgroup, 0, 1)
		for _, wg := range *listing {
			if wg.ID == token || wg.Name == token {
				matches = append(matches, wg)
			}
		}
		switch len(matches) {
		case 0:
			return nil, fmt.Errorf("profile workgroup %q is not visible to the pulse-agent account", token)
		case 1:
			resolved[matches[0].ID] = struct{}{}
		default:
			ids := make([]string, 0, len(matches))
			for _, wg := range matches {
				ids = append(ids, wg.ID)
			}
			return nil, fmt.Errorf("profile workgroup %q matches multiple visible workgroups; specify one of %s", token, strings.Join(ids, ", "))
		}
	}
	if len(resolved) == 0 {
		return nil, nil
	}
	return resolved, nil
}

func advertisementInWorkgroups(ad api.Advertisement, workgroups map[string]struct{}) bool {
	if len(workgroups) == 0 {
		return true
	}
	for _, wg := range ad.WorkgroupScopes {
		if _, ok := workgroups[wg]; ok {
			return true
		}
	}
	return false
}

func (s *loopOutcomeSelector) selectAdvertisements(ads []api.Advertisement) []api.Advertisement {
	if s == nil {
		return ads
	}
	filtered := make([]api.Advertisement, 0, len(ads))
	for _, ad := range ads {
		if advertisementInWorkgroups(ad, s.workgroups) {
			filtered = append(filtered, ad)
		}
	}
	if s.profile == nil {
		return filtered
	}
	if s.rand == nil {
		s.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	count := sampleCountDistribution(s.rand, s.profile.AdvertisementCount, len(filtered))
	if count >= len(filtered) {
		return filtered
	}
	perm := s.rand.Perm(len(filtered))
	selected := map[int]struct{}{}
	for _, idx := range perm[:count] {
		selected[idx] = struct{}{}
	}
	out := make([]api.Advertisement, 0, count)
	for idx, ad := range filtered {
		if _, ok := selected[idx]; ok {
			out = append(out, ad)
		}
	}
	return out
}
