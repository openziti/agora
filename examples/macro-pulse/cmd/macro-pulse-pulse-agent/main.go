package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/agentutil"
	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
)

func main() {
	fs := flag.NewFlagSet("pulse-agent", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Emit the brief as a JSON document instead of formatted text")
	markdownPath := fs.String("markdown", "", "Also write the brief as a Markdown document to the given path")
	loop := fs.Bool("loop", envBool("AGORA_PULSE_LOOP"), "Run continuously until interrupted")
	loopPauseMin := fs.Duration("loop-pause-min", 20*time.Second, "Minimum pause between loop iterations")
	loopPauseMax := fs.Duration("loop-pause-max", 60*time.Second, "Maximum pause between loop iterations")
	profilePath := fs.String("profile", "", "Load loop activity profile YAML")

	app := agent.New("pulse-agent",
		agent.WithDescription("Macro Pulse orchestrator: composes across providers and tools to produce the morning brief"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		runner := briefRunner{
			jsonOut:      *jsonOut,
			markdownPath: *markdownPath,
		}
		if *loop {
			var profile *activityProfile
			if *profilePath != "" {
				loaded, err := loadActivityProfile(*profilePath)
				if err != nil {
					return err
				}
				profile = loaded
			}
			return runner.runLoop(ctx, a, loopOptions{
				pauseMin: *loopPauseMin,
				pauseMax: *loopPauseMax,
				profile:  profile,
				rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
			})
		}
		brief, err := buildBrief(ctx, a, nil, nil)
		if err != nil {
			a.Log().Errorf("build brief: %v", err)
			return err
		}
		return runner.emit(a, brief)
	}); err != nil {
		os.Exit(1)
	}
}

type briefRunner struct {
	jsonOut      bool
	markdownPath string
}

type loopOptions struct {
	pauseMin time.Duration
	pauseMax time.Duration
	profile  *activityProfile
	rand     *rand.Rand
}

const (
	tightContractName        = "demo-contract-tight"
	warmupReaperWait         = 45 * time.Second
	warmupNormalCloseDetail  = "D.2 warm-up normal close"
	briefNormalCloseDetail   = "brief complete"
	sessionClosePollInterval = 2 * time.Second
)

type sessionOutcome string

const (
	sessionOutcomeNormal           sessionOutcome = "normal"
	sessionOutcomeRuntimeViolation sessionOutcome = "runtime_violation"
	sessionOutcomeReaperViolation  sessionOutcome = "reaper_violation"
	sessionOutcomeLongTail         sessionOutcome = "long_tail"
)

type runtimeViolationKind string

const (
	runtimeViolationOversize   runtimeViolationKind = "oversize"
	runtimeViolationDisallowed runtimeViolationKind = "disallowed_message_type"
)

type queryOptions struct {
	outcome              sessionOutcome
	closeDetail          string
	holdAfterReply       time.Duration
	reaperHold           time.Duration
	reaperWait           time.Duration
	runtimeViolation     runtimeViolationKind
	oversizePayloadBytes int
	disallowedMessage    string
}

type loopOutcomeSelector struct {
	rand                      *rand.Rand
	profile                   *activityProfile
	workgroups                map[string]struct{}
	tightAdvertisementID      string
	contractByAdvertisementID map[string]*api.Contract
}

type loopSummary struct {
	startedAt           time.Time
	iterations          int64
	iterationsSucceeded int64
	iterationsFailed    int64
	sessionsProposed    int64
	sessionsCompleted   int64
}

func newLoopSummary() *loopSummary {
	return &loopSummary{startedAt: time.Now().UTC()}
}

func (s *loopSummary) recordSessionProposed() {
	if s != nil {
		atomic.AddInt64(&s.sessionsProposed, 1)
	}
}

func (s *loopSummary) recordSessionCompleted() {
	if s != nil {
		atomic.AddInt64(&s.sessionsCompleted, 1)
	}
}

func (s *loopSummary) String() string {
	if s == nil {
		return "iterations=0 sessions_proposed=0 sessions_completed=0"
	}
	elapsed := time.Since(s.startedAt).Round(time.Second)
	return fmt.Sprintf(
		"iterations=%d iterations_succeeded=%d iterations_failed=%d sessions_proposed=%d sessions_completed=%d elapsed=%s",
		atomic.LoadInt64(&s.iterations),
		atomic.LoadInt64(&s.iterationsSucceeded),
		atomic.LoadInt64(&s.iterationsFailed),
		atomic.LoadInt64(&s.sessionsProposed),
		atomic.LoadInt64(&s.sessionsCompleted),
		elapsed,
	)
}

func (r briefRunner) runLoop(ctx context.Context, a *agent.Agent, opts loopOptions) error {
	if opts.rand == nil {
		opts.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if opts.profile == nil {
		if opts.pauseMin <= 0 {
			opts.pauseMin = 20 * time.Second
		}
		if opts.pauseMax <= 0 {
			opts.pauseMax = 60 * time.Second
		}
		if opts.pauseMax < opts.pauseMin {
			return fmt.Errorf("loop-pause-max must be >= loop-pause-min")
		}
		opts.profile = defaultActivityProfile(opts.pauseMin, opts.pauseMax)
	}
	if err := opts.profile.validate(); err != nil {
		return err
	}

	summary := newLoopSummary()
	defer fmt.Fprintf(os.Stderr, "macro-pulse loop summary: %s\n", summary)

	workgroups, err := resolveProfileWorkgroups(ctx, a, opts.profile)
	if err != nil {
		return err
	}
	outcomeCatalog, err := r.runWarmup(ctx, a, summary, workgroups)
	if err != nil {
		return err
	}
	selector := &loopOutcomeSelector{
		rand:                      opts.rand,
		profile:                   opts.profile,
		workgroups:                workgroups,
		tightAdvertisementID:      outcomeCatalog.tightAd.ID,
		contractByAdvertisementID: outcomeCatalog.contractByAdvertisementID,
	}

	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		iteration := atomic.AddInt64(&summary.iterations, 1)
		a.Log().With("iteration", iteration).Infof("starting macro-pulse loop iteration")
		brief, err := buildBrief(ctx, a, summary, selector)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			atomic.AddInt64(&summary.iterationsFailed, 1)
			a.Log().With("iteration", iteration).Warnf("macro-pulse loop iteration failed: %v", err)
		} else {
			if err := r.emit(a, brief); err != nil {
				atomic.AddInt64(&summary.iterationsFailed, 1)
				return err
			}
			atomic.AddInt64(&summary.iterationsSucceeded, 1)
		}

		pause := sampleDurationDistribution(opts.rand, opts.profile.Pause)
		a.Log().With("iteration", iteration).With("pause", pause.String()).Infof("macro-pulse loop iteration complete")
		if err := sleepContext(ctx, pause); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

type outcomeCatalog struct {
	tightAd                   api.Advertisement
	tightContract             *api.Contract
	defaultAdByCapability     map[string]api.Advertisement
	contractByAdvertisementID map[string]*api.Contract
}

func (r briefRunner) runWarmup(ctx context.Context, a *agent.Agent, summary *loopSummary, workgroups map[string]struct{}) (*outcomeCatalog, error) {
	catalog, err := loadOutcomeCatalog(ctx, a, workgroups)
	if err != nil {
		return nil, err
	}
	a.Log().
		With("advertisement_id", catalog.tightAd.ID).
		With("contract", tightContractName).
		Infof("starting D.2 deterministic warm-up")

	if err := runReaperOutcome(ctx, a, catalog.tightAd, catalog.tightContract, summary, queryOptions{
		outcome:     sessionOutcomeReaperViolation,
		closeDetail: "D.2 warm-up reaper contract-violation hold",
		reaperHold:  holdPastDurationCap(catalog.tightContract),
		reaperWait:  warmupReaperWait,
	}); err != nil {
		return nil, fmt.Errorf("D.2 warm-up reaper contract violation: %w", err)
	}

	ad, capability, contract, ok := catalog.defaultWarmupAdvertisement()
	if !ok {
		return nil, fmt.Errorf("D.2 warm-up requires at least one visible default-contract Macro Pulse advertisement; run F.1 demo bootstrap/topology before starting the loop")
	}
	hold := warmupLongTailHold(contract)
	if hold <= 0 {
		return nil, fmt.Errorf("D.2 warm-up default-contract advertisement %q has no safe long-tail hold below its duration cap", ad.ID)
	}
	if err := runWarmupCapabilityQuery(ctx, a, ad, capability, summary, queryOptions{
		outcome:        sessionOutcomeLongTail,
		closeDetail:    "D.2 warm-up long-tail close",
		holdAfterReply: hold,
	}); err != nil {
		return nil, fmt.Errorf("D.2 warm-up long-tail close: %w", err)
	}
	if err := runWarmupCapabilityQuery(ctx, a, ad, capability, summary, queryOptions{
		outcome:     sessionOutcomeNormal,
		closeDetail: warmupNormalCloseDetail,
	}); err != nil {
		return nil, fmt.Errorf("D.2 warm-up normal close: %w", err)
	}

	a.Log().Infof("completed D.2 deterministic warm-up")
	return catalog, nil
}

func loadOutcomeCatalog(ctx context.Context, a *agent.Agent, workgroups map[string]struct{}) (*outcomeCatalog, error) {
	res, err := a.Controller().SearchCatalog(ctx, api.SearchCatalogParams{})
	if err != nil {
		return nil, fmt.Errorf("catalog search: %w", err)
	}
	listing, ok := res.(*api.CatalogSearchResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected catalog search response: %T", res)
	}
	catalog := &outcomeCatalog{
		defaultAdByCapability:     map[string]api.Advertisement{},
		contractByAdvertisementID: map[string]*api.Contract{},
	}
	for _, ad := range listing.Items {
		if !advertisementInWorkgroups(ad, workgroups) {
			continue
		}
		var contract *api.Contract
		if ad.ContractId.Set && ad.ContractId.Value != "" {
			contract, err = getVisibleContract(ctx, a, ad.ContractId.Value)
			if err != nil {
				return nil, fmt.Errorf("get contract %q for advertisement %q: %w", ad.ContractId.Value, ad.ID, err)
			}
			catalog.contractByAdvertisementID[ad.ID] = contract
		}
		if contract != nil && contract.Name == tightContractName {
			catalog.tightAd = ad
			catalog.tightContract = contract
			continue
		}
		for _, capability := range ad.Capabilities {
			if _, exists := catalog.defaultAdByCapability[capability.Name]; !exists {
				catalog.defaultAdByCapability[capability.Name] = ad
			}
		}
	}
	if catalog.tightAd.ID == "" {
		return nil, fmt.Errorf("D.2 warm-up requires a visible advertisement using contract %q; run F.1 demo bootstrap/topology so news-pulse@signals-co publishes with the tight-contract assignment", tightContractName)
	}
	return catalog, nil
}

func getVisibleContract(ctx context.Context, a *agent.Agent, contractID string) (*api.Contract, error) {
	res, err := a.Controller().GetContract(ctx, api.GetContractParams{ContractId: contractID})
	if err != nil {
		return nil, err
	}
	contract, ok := res.(*api.Contract)
	if !ok {
		return nil, fmt.Errorf("unexpected get contract response: %T", res)
	}
	return contract, nil
}

var warmupCapabilityOrder = []string{
	"markets.fx",
	"signals.search",
	"weather.current",
	"markets.equity",
	"markets.commodities",
	"weather.forecast",
	"signals.news",
	"analytics.narrate",
	"analytics.correlate",
}

func (c *outcomeCatalog) defaultWarmupAdvertisement() (api.Advertisement, string, *api.Contract, bool) {
	for _, capability := range warmupCapabilityOrder {
		ad, ok := c.defaultAdByCapability[capability]
		if !ok {
			continue
		}
		return ad, capability, c.contractByAdvertisementID[ad.ID], true
	}
	return api.Advertisement{}, "", nil, false
}

func warmupLongTailHold(contract *api.Contract) time.Duration {
	hold := time.Minute
	if contract == nil || contract.MaxDurationSeconds <= 0 {
		return hold
	}
	ceiling := time.Duration(contract.MaxDurationSeconds)*time.Second - 15*time.Second
	if ceiling <= 0 {
		return 0
	}
	if ceiling < hold {
		return ceiling
	}
	return hold
}

func runWarmupCapabilityQuery(ctx context.Context, a *agent.Agent, ad api.Advertisement, capability string, summary *loopSummary, opts queryOptions) error {
	switch capability {
	case "markets.equity":
		req := payloads.EquityRequest{Tickers: []string{"SPY", "XLK"}, WindowDays: 7}
		var resp payloads.EquityResponse
		return query(ctx, a, ad, "markets.equity.request", req, &resp, summary, opts)
	case "markets.fx":
		req := payloads.FXRequest{Pairs: []string{"USD-EUR", "USD-JPY"}, WindowDays: 7}
		var resp payloads.FXResponse
		return query(ctx, a, ad, "markets.fx.request", req, &resp, summary, opts)
	case "markets.commodities":
		req := payloads.CommoditiesRequest{Symbols: []string{"CL_F", "GC_F"}, WindowDays: 7}
		var resp payloads.CommoditiesResponse
		return query(ctx, a, ad, "markets.commodities.request", req, &resp, summary, opts)
	case "weather.current":
		req := payloads.WeatherCurrentRequest{Cities: []string{"new-york", "houston"}}
		var resp payloads.WeatherCurrentResponse
		return query(ctx, a, ad, "weather.current.request", req, &resp, summary, opts)
	case "weather.forecast":
		req := payloads.WeatherForecastRequest{Cities: []string{"houston"}, HorizonHours: 72}
		var resp payloads.WeatherForecastResponse
		return query(ctx, a, ad, "weather.forecast.request", req, &resp, summary, opts)
	case "signals.search":
		req := payloads.SearchRequest{Terms: []string{"layoffs", "gulf-storm"}, WindowDays: 30}
		var resp payloads.SearchResponse
		return query(ctx, a, ad, "signals.search.request", req, &resp, summary, opts)
	case "signals.news":
		req := payloads.NewsRequest{Topics: []string{"financial", "energy"}, WindowDays: 7}
		var resp payloads.NewsResponse
		return query(ctx, a, ad, "signals.news.request", req, &resp, summary, opts)
	case "analytics.correlate":
		req := payloads.CorrelateRequest{
			SeriesA: payloads.CorrelateLabeledSeries{Label: "warm-up A", Points: []payloads.SeriesPoint{{T: "2026-01-01", V: 1}, {T: "2026-01-02", V: 2}, {T: "2026-01-03", V: 3}}},
			SeriesB: payloads.CorrelateLabeledSeries{Label: "warm-up B", Points: []payloads.SeriesPoint{{T: "2026-01-01", V: 1}, {T: "2026-01-02", V: 2}, {T: "2026-01-03", V: 4}}},
		}
		var resp payloads.CorrelateResponse
		return query(ctx, a, ad, "analytics.correlate.request", req, &resp, summary, opts)
	case "analytics.narrate":
		req := payloads.NarrateRequest{Template: "macro-pulse-default", Inputs: payloads.NarrateInputs{}}
		var resp payloads.NarrateResponse
		return query(ctx, a, ad, "analytics.narrate.request", req, &resp, summary, opts)
	default:
		return fmt.Errorf("unsupported warm-up capability %q", capability)
	}
}

func (r briefRunner) emit(a *agent.Agent, brief *brief) error {
	if r.markdownPath != "" {
		if err := os.WriteFile(r.markdownPath, []byte(formatBriefMarkdown(brief)), 0o644); err != nil {
			return fmt.Errorf("write markdown brief: %w", err)
		}
		a.Log().With("path", r.markdownPath).Infof("wrote markdown brief")
	}

	if r.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(brief)
	}
	fmt.Print(formatBrief(brief))
	return nil
}

func randomPause(r *rand.Rand, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return min + time.Duration(r.Int63n(int64(max-min)+1))
}

func classifySessionOutcome(p float64) sessionOutcome {
	return classifySessionOutcomeForProfile(p, defaultOutcomeProbabilities())
}

func randomLongTailHold(r *rand.Rand, contract *api.Contract) time.Duration {
	minHold := time.Minute
	maxHold := 5 * time.Minute
	if contract != nil && contract.MaxDurationSeconds > 0 {
		ceiling := time.Duration(contract.MaxDurationSeconds)*time.Second - 15*time.Second
		if ceiling <= 0 {
			return 0
		}
		if ceiling < minHold {
			if ceiling < 5*time.Second {
				return ceiling
			}
			minHold = ceiling / 2
			if minHold < 5*time.Second {
				minHold = 5 * time.Second
			}
			maxHold = ceiling
		} else if ceiling < maxHold {
			maxHold = ceiling
		}
	}
	return randomPause(r, minHold, maxHold)
}

func holdPastDurationCap(contract *api.Contract) time.Duration {
	if contract == nil || contract.MaxDurationSeconds <= 0 {
		return time.Minute
	}
	capDuration := time.Duration(contract.MaxDurationSeconds) * time.Second
	hold := capDuration + 20*time.Second
	if hold < time.Minute {
		return time.Minute
	}
	return hold
}

func oversizePayloadBytes(contract *api.Contract) int {
	if contract == nil || contract.MaxEnvelopeBytes <= 0 {
		return 4096
	}
	size := contract.MaxEnvelopeBytes * 4
	if size < 4096 {
		return 4096
	}
	return size
}

func (s *loopOutcomeSelector) queryOptionsFor(ad api.Advertisement) queryOptions {
	if s == nil || s.rand == nil {
		return queryOptions{outcome: sessionOutcomeNormal, closeDetail: briefNormalCloseDetail}
	}
	contract := s.contractByAdvertisementID[ad.ID]
	probabilities := defaultOutcomeProbabilities()
	if s.profile != nil {
		probabilities = s.profile.Outcomes
	}
	outcome := classifySessionOutcomeForProfile(s.rand.Float64(), probabilities)
	return queryOptionsForOutcome(ad.ID, s.tightAdvertisementID, outcome, contract, s.rand)
}

func queryOptionsForOutcome(adID, tightAdvertisementID string, outcome sessionOutcome, contract *api.Contract, r *rand.Rand) queryOptions {
	if adID == tightAdvertisementID {
		switch outcome {
		case sessionOutcomeRuntimeViolation:
			kind := runtimeViolationOversize
			if r != nil && r.Intn(2) == 1 {
				kind = runtimeViolationDisallowed
			}
			return queryOptions{
				outcome:              sessionOutcomeRuntimeViolation,
				closeDetail:          "D.2 random runtime contract-violation attempt",
				runtimeViolation:     kind,
				oversizePayloadBytes: oversizePayloadBytes(contract),
				disallowedMessage:    "macro-pulse.contract-violation.request",
			}
		case sessionOutcomeReaperViolation:
			return queryOptions{
				outcome:     sessionOutcomeReaperViolation,
				closeDetail: "D.2 random reaper contract-violation hold",
				reaperHold:  holdPastDurationCap(contract),
				reaperWait:  warmupReaperWait,
			}
		default:
			return queryOptions{outcome: sessionOutcomeNormal, closeDetail: briefNormalCloseDetail}
		}
	}
	if outcome == sessionOutcomeLongTail {
		hold := randomLongTailHold(r, contract)
		if hold > 0 {
			return queryOptions{
				outcome:        sessionOutcomeLongTail,
				closeDetail:    "D.2 random long-tail close",
				holdAfterReply: hold,
			}
		}
	}
	return queryOptions{outcome: sessionOutcomeNormal, closeDetail: briefNormalCloseDetail}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// brief is the orchestrator's accumulated structured view, assembled
// from per-capability responses. formatBrief renders this to text.
type brief struct {
	AsOf         time.Time                         `json:"as_of"`
	Markets      []marketLine                      `json:"markets"`
	Weather      []weatherLine                     `json:"weather"`
	Signals      []signalLine                      `json:"signals"`
	Correlations []correlationLine                 `json:"correlations"`
	Narrative    string                            `json:"narrative"`
	Catalog      []catalogEntry                    `json:"catalog"`
	rawSeries    map[string][]payloads.SeriesPoint // not exported; correlator inputs
}

type marketLine struct {
	Label     string  `json:"label"`
	Value     string  `json:"value"`
	ChangePct float64 `json:"change_pct"`
	Domain    string  `json:"domain"`
}

type weatherLine struct {
	City    string `json:"city"`
	Summary string `json:"summary"`
}

type signalLine struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type correlationLine struct {
	Pair string  `json:"pair"`
	R    float64 `json:"r"`
}

type catalogEntry struct {
	AdvertisementID string   `json:"advertisement_id"`
	Name            string   `json:"name"`
	Capabilities    []string `json:"capabilities"`
	OwnerAccountID  string   `json:"owner_account_id"`
}

func proposeBriefSession(ctx context.Context, a *agent.Agent, ad api.Advertisement, message string) (*session.Session, error) {
	if len(ad.WorkgroupScopes) == 0 {
		return nil, fmt.Errorf("advertisement %s has no visible workgroup", ad.ID)
	}
	if message == "" {
		message = "macro-pulse morning brief"
	}
	return session.Propose(ctx, a, ad.ID, session.ProposeOptions{
		WorkgroupID: ad.WorkgroupScopes[0],
		Message:     message,
		Timeout:     20 * time.Second,
	})
}

func runReaperOutcome(ctx context.Context, a *agent.Agent, ad api.Advertisement, contract *api.Contract, summary *loopSummary, opts queryOptions) error {
	proposeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	sess, err := proposeBriefSession(proposeCtx, a, ad, opts.closeDetail)
	cancel()
	if err != nil {
		return fmt.Errorf("propose reaper outcome: %w", err)
	}
	summary.recordSessionProposed()

	hold := opts.reaperHold
	if hold <= 0 {
		hold = holdPastDurationCap(contract)
	}
	a.Log().
		With("session_id", sess.ID).
		With("advertisement_id", ad.ID).
		With("hold", hold.String()).
		Infof("holding session past contract duration cap")
	if err := agentutil.HoldPastDurationCap(ctx, sess, hold); err != nil {
		_ = sess.Close(ctx, "D.2 reaper hold interrupted")
		return err
	}

	wait := opts.reaperWait
	if wait <= 0 {
		wait = warmupReaperWait
	}
	closed, err := waitSessionClosed(ctx, a, sess.ID, wait)
	if err != nil {
		_ = sess.Close(ctx, "D.2 reaper hold cleanup")
		return err
	}
	if !closed.CloseReason.Set || closed.CloseReason.Value != api.SessionCloseReasonContractViolation {
		reason := "<unset>"
		if closed.CloseReason.Set {
			reason = string(closed.CloseReason.Value)
		}
		return fmt.Errorf("session %q closed with close_reason=%q, expected %q", sess.ID, reason, api.SessionCloseReasonContractViolation)
	}
	_ = sess.Close(ctx, "D.2 reaper observed closed")
	summary.recordSessionCompleted()
	return nil
}

func waitSessionClosed(ctx context.Context, a *agent.Agent, sessionID string, wait time.Duration) (*api.Session, error) {
	if wait <= 0 {
		wait = sessionClosePollInterval
	}
	deadline := time.Now().Add(wait)
	for {
		current, err := getAPISession(ctx, a, sessionID)
		if err != nil {
			return nil, err
		}
		if current.State == api.SessionStateClosed {
			return current, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("session %q did not close within %s; state=%q", sessionID, wait, current.State)
		}
		if err := sleepContext(ctx, sessionClosePollInterval); err != nil {
			return nil, err
		}
	}
}

func getAPISession(ctx context.Context, a *agent.Agent, sessionID string) (*api.Session, error) {
	res, err := a.Controller().GetSession(ctx, api.GetSessionParams{SessionId: sessionID})
	if err != nil {
		return nil, err
	}
	sess, ok := res.(*api.Session)
	if !ok {
		return nil, fmt.Errorf("unexpected get session response: %T", res)
	}
	return sess, nil
}

func attemptRuntimeViolation(ctx context.Context, sess *session.Session, msgType string, opts queryOptions) error {
	var err error
	switch opts.runtimeViolation {
	case runtimeViolationDisallowed:
		messageType := opts.disallowedMessage
		if messageType == "" {
			messageType = "macro-pulse.contract-violation.request"
		}
		err = agentutil.SendDisallowedMessageType(ctx, sess, messageType)
	default:
		payloadBytes := opts.oversizePayloadBytes
		if payloadBytes <= 0 {
			payloadBytes = 4096
		}
		err = agentutil.SendOversizeEnvelope(ctx, sess, msgType, payloadBytes)
	}
	if err != nil && !agentutil.IsContractViolation(err) {
		return err
	}
	return nil
}

// query opens a session against ad, sends one envelope of msgType
// carrying req as JSON, decodes the reply into out, and closes.
//
// The 90s timeout here is sized for the slowest --live path: GDELT's
// public API throttles to ~1 request per 5 seconds, so a single news
// query against three topics serially takes ~20-25s before the
// snapshot fallback even starts. Snapshot-only runs return well under
// a second per query; D.2 long-tail options intentionally hold the
// session after the reply and before normal close.
func query[Req any, Resp any](ctx context.Context, a *agent.Agent, ad api.Advertisement, msgType string, req Req, out *Resp, summary *loopSummary, opts queryOptions) error {
	if opts.outcome == sessionOutcomeReaperViolation {
		return runReaperOutcome(ctx, a, ad, nil, summary, opts)
	}
	closeDetail := opts.closeDetail
	if closeDetail == "" {
		closeDetail = briefNormalCloseDetail
	}
	sessCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	sess, err := proposeBriefSession(sessCtx, a, ad, "macro-pulse morning brief")
	if err != nil {
		return fmt.Errorf("propose %s: %w", msgType, err)
	}
	summary.recordSessionProposed()
	defer func() {
		if sess.Close(ctx, closeDetail) == nil {
			summary.recordSessionCompleted()
		}
	}()

	if opts.outcome == sessionOutcomeRuntimeViolation {
		if err := attemptRuntimeViolation(sessCtx, sess, msgType, opts); err != nil {
			return fmt.Errorf("runtime violation attempt %s: %w", msgType, err)
		}
		if _, err := waitSessionClosed(ctx, a, sess.ID, 10*time.Second); err != nil {
			a.Log().With("session_id", sess.ID).Warnf("runtime violation attempt did not close before cleanup: %v", err)
		}
		return nil
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", msgType, err)
	}
	if err := sess.Send(sessCtx, session.Envelope{
		MessageType: msgType,
		ContentType: "application/json",
		Payload:     body,
	}); err != nil {
		return fmt.Errorf("send %s: %w", msgType, err)
	}
	reply, err := sess.Receive(sessCtx)
	if err != nil {
		return fmt.Errorf("receive %s reply: %w", msgType, err)
	}
	if len(reply.Payload) == 0 {
		return fmt.Errorf("empty reply payload for %s", msgType)
	}
	if err := json.Unmarshal(reply.Payload, out); err != nil {
		return fmt.Errorf("decode %s reply: %w", msgType, err)
	}
	if opts.holdAfterReply > 0 {
		a.Log().
			With("session_id", sess.ID).
			With("advertisement_id", ad.ID).
			With("hold", opts.holdAfterReply.String()).
			Infof("holding session before normal close")
		if err := sleepContext(ctx, opts.holdAfterReply); err != nil {
			return err
		}
	}
	return nil
}

func buildBrief(ctx context.Context, a *agent.Agent, summary *loopSummary, outcomes *loopOutcomeSelector) (*brief, error) {
	res, err := a.Controller().SearchCatalog(ctx, api.SearchCatalogParams{})
	if err != nil {
		return nil, fmt.Errorf("catalog search: %w", err)
	}
	listing, ok := res.(*api.CatalogSearchResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected catalog search response: %T", res)
	}
	a.Log().With("count", len(listing.Items)).Infof("discovered advertisements via catalog")

	items := listing.Items
	if outcomes != nil {
		items = outcomes.selectAdvertisements(items)
	}
	adByCapability := map[string]api.Advertisement{}
	catalog := make([]catalogEntry, 0, len(items))
	for _, ad := range items {
		caps := make([]string, 0, len(ad.Capabilities))
		for _, c := range ad.Capabilities {
			adByCapability[c.Name] = ad
			caps = append(caps, c.Name)
		}
		catalog = append(catalog, catalogEntry{
			AdvertisementID: ad.ID,
			Name:            ad.Name,
			Capabilities:    caps,
			OwnerAccountID:  ad.AccountId,
		})
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Name < catalog[j].Name })

	b := &brief{
		AsOf:      time.Now().UTC(),
		Catalog:   catalog,
		rawSeries: map[string][]payloads.SeriesPoint{},
	}

	// --- markets: equity ---
	if ad, ok := adByCapability["markets.equity"]; ok {
		req := payloads.EquityRequest{Tickers: []string{"SPY", "XLK", "XLE", "XLF"}, WindowDays: 7}
		var resp payloads.EquityResponse
		if err := query(ctx, a, ad, "markets.equity.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("markets.equity: %v", err)
		} else {
			b.AsOf = newer(b.AsOf, resp.AsOf)
			if d, ok := resp.Tickers["SPY"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "S&P 500", Value: fmt.Sprintf("%.2f", d.Price), ChangePct: d.ChangePct, Domain: "equity"})
				b.rawSeries["SPY"] = d.Series
			}
			if d, ok := resp.Tickers["XLK"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "XLK (tech)", Value: fmt.Sprintf("%.2f", d.Price), ChangePct: d.ChangePct, Domain: "equity"})
			}
			if d, ok := resp.Tickers["XLE"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "XLE (energy)", Value: fmt.Sprintf("%.2f", d.Price), ChangePct: d.ChangePct, Domain: "equity"})
			}
			if d, ok := resp.Tickers["XLF"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "XLF (financials)", Value: fmt.Sprintf("%.2f", d.Price), ChangePct: d.ChangePct, Domain: "equity"})
			}
		}
	}

	// --- markets: fx ---
	if ad, ok := adByCapability["markets.fx"]; ok {
		req := payloads.FXRequest{Pairs: []string{"USD-EUR", "USD-JPY"}, WindowDays: 7}
		var resp payloads.FXResponse
		if err := query(ctx, a, ad, "markets.fx.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("markets.fx: %v", err)
		} else {
			if d, ok := resp.Pairs["USD-EUR"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "USD/EUR", Value: fmt.Sprintf("%.4f", d.Rate), ChangePct: d.ChangePct, Domain: "fx"})
			}
			if d, ok := resp.Pairs["USD-JPY"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "USD/JPY", Value: fmt.Sprintf("%.2f", d.Rate), ChangePct: d.ChangePct, Domain: "fx"})
			}
		}
	}

	// --- markets: commodities ---
	if ad, ok := adByCapability["markets.commodities"]; ok {
		req := payloads.CommoditiesRequest{Symbols: []string{"CL_F", "GC_F", "NG_F"}, WindowDays: 7}
		var resp payloads.CommoditiesResponse
		if err := query(ctx, a, ad, "markets.commodities.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("markets.commodities: %v", err)
		} else {
			if d, ok := resp.Symbols["CL_F"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "WTI crude", Value: fmt.Sprintf("$%.2f", d.Price), ChangePct: d.ChangePct, Domain: "commodities"})
				b.rawSeries["CL_F"] = d.Series
			}
			if d, ok := resp.Symbols["GC_F"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "Gold", Value: fmt.Sprintf("$%.2f", d.Price), ChangePct: d.ChangePct, Domain: "commodities"})
			}
			if d, ok := resp.Symbols["NG_F"]; ok {
				b.Markets = append(b.Markets, marketLine{Label: "Nat gas", Value: fmt.Sprintf("$%.2f", d.Price), ChangePct: d.ChangePct, Domain: "commodities"})
			}
		}
	}

	// --- weather: current ---
	if ad, ok := adByCapability["weather.current"]; ok {
		req := payloads.WeatherCurrentRequest{Cities: []string{"new-york", "houston", "frankfurt", "singapore"}}
		var resp payloads.WeatherCurrentResponse
		if err := query(ctx, a, ad, "weather.current.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("weather.current: %v", err)
		} else {
			for _, c := range []string{"new-york", "houston", "frankfurt", "singapore"} {
				if d, ok := resp.Cities[c]; ok {
					anomalySign := "+"
					if d.AnomalyF < 0 {
						anomalySign = "-"
					}
					b.Weather = append(b.Weather, weatherLine{
						City:    c,
						Summary: fmt.Sprintf("%s, %s%.1f°F vs seasonal", d.Condition, anomalySign, math.Abs(d.AnomalyF)),
					})
				}
			}
		}
	}

	// --- weather: forecast (Houston only — gulf-storm narrative) ---
	if ad, ok := adByCapability["weather.forecast"]; ok {
		req := payloads.WeatherForecastRequest{Cities: []string{"houston"}, HorizonHours: 72}
		var resp payloads.WeatherForecastResponse
		if err := query(ctx, a, ad, "weather.forecast.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("weather.forecast: %v", err)
		} else if d, ok := resp.Cities["houston"]; ok && len(d.Daily) > 0 {
			summaries := make([]string, 0, len(d.Daily))
			for _, day := range d.Daily {
				summaries = append(summaries, day.Condition)
			}
			b.Weather = append(b.Weather, weatherLine{
				City:    "houston (72h forecast)",
				Summary: strings.Join(summaries, " → "),
			})
		}
	}

	// --- signals: search ---
	if ad, ok := adByCapability["signals.search"]; ok {
		req := payloads.SearchRequest{Terms: []string{"layoffs", "gulf-storm", "housing-market"}, WindowDays: 30}
		var resp payloads.SearchResponse
		if err := query(ctx, a, ad, "signals.search.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("signals.search: %v", err)
		} else {
			if d, ok := resp.Terms["layoffs"]; ok {
				b.Signals = append(b.Signals, signalLine{Label: `Search "layoffs"`, Value: d.ChangePct, Unit: "%"})
				b.rawSeries["search:layoffs"] = d.Series
			}
			if d, ok := resp.Terms["gulf-storm"]; ok {
				b.Signals = append(b.Signals, signalLine{Label: `Search "gulf-storm"`, Value: d.ChangePct, Unit: "%"})
				b.rawSeries["search:gulf-storm"] = d.Series
			}
			if d, ok := resp.Terms["housing-market"]; ok {
				b.Signals = append(b.Signals, signalLine{Label: `Search "housing-market"`, Value: d.ChangePct, Unit: "%"})
			}
		}
	}

	// --- signals: news ---
	if ad, ok := adByCapability["signals.news"]; ok {
		req := payloads.NewsRequest{Topics: []string{"financial", "energy", "supply-chain"}, WindowDays: 7}
		var resp payloads.NewsResponse
		if err := query(ctx, a, ad, "signals.news.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("signals.news: %v", err)
		} else {
			if d, ok := resp.Topics["financial"]; ok {
				b.Signals = append(b.Signals, signalLine{Label: "News tone (financial)", Value: d.Tone, Unit: "tone"})
			}
			if d, ok := resp.Topics["energy"]; ok {
				b.Signals = append(b.Signals, signalLine{Label: "News tone (energy)", Value: d.Tone, Unit: "tone"})
			}
		}
	}

	// --- correlations: defer to the analytics.correlate provider, which
	//     sees only the labels and numeric series (no leak of underlying
	//     ticker / city / term identities). ---
	if ad, ok := adByCapability["analytics.correlate"]; ok {
		pairs := []struct {
			label, sa, la, sb, lb string
		}{
			{"WTI crude vs gulf-storm search", "CL_F", "WTI crude (USD/bbl)", "search:gulf-storm", "gulf-storm search interest"},
			{"S&P 500 vs layoffs search", "SPY", "S&P 500 ETF", "search:layoffs", "layoffs search interest"},
		}
		for _, p := range pairs {
			seriesA, ok1 := b.rawSeries[p.sa]
			seriesB, ok2 := b.rawSeries[p.sb]
			if !ok1 || !ok2 {
				continue
			}
			req := payloads.CorrelateRequest{
				SeriesA: payloads.CorrelateLabeledSeries{Label: p.la, Points: seriesA},
				SeriesB: payloads.CorrelateLabeledSeries{Label: p.lb, Points: seriesB},
			}
			var resp payloads.CorrelateResponse
			if err := query(ctx, a, ad, "analytics.correlate.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
				a.Log().Warnf("analytics.correlate %s: %v", p.label, err)
				continue
			}
			b.Correlations = append(b.Correlations, correlationLine{Pair: p.label, R: resp.PearsonR})
		}
	}

	// --- narrative: defer to the analytics.narrate provider. ---
	if ad, ok := adByCapability["analytics.narrate"]; ok {
		inputs := payloads.NarrateInputs{
			Markets:      make([]payloads.NarrateMarketLine, 0, len(b.Markets)),
			Weather:      make([]payloads.NarrateWeatherLine, 0, len(b.Weather)),
			Signals:      make([]payloads.NarrateSignalLine, 0, len(b.Signals)),
			Correlations: make([]payloads.NarrateCorrelationLine, 0, len(b.Correlations)),
		}
		for _, m := range b.Markets {
			inputs.Markets = append(inputs.Markets, payloads.NarrateMarketLine{Label: m.Label, Value: m.Value, ChangePct: m.ChangePct})
		}
		for _, w := range b.Weather {
			inputs.Weather = append(inputs.Weather, payloads.NarrateWeatherLine{City: w.City, Summary: w.Summary})
		}
		for _, s := range b.Signals {
			inputs.Signals = append(inputs.Signals, payloads.NarrateSignalLine{Label: s.Label, Value: s.Value, Unit: s.Unit})
		}
		for _, c := range b.Correlations {
			inputs.Correlations = append(inputs.Correlations, payloads.NarrateCorrelationLine{Pair: c.Pair, R: c.R})
		}
		req := payloads.NarrateRequest{Template: "macro-pulse-default", Inputs: inputs}
		var resp payloads.NarrateResponse
		if err := query(ctx, a, ad, "analytics.narrate.request", req, &resp, summary, outcomes.queryOptionsFor(ad)); err != nil {
			a.Log().Warnf("analytics.narrate: %v", err)
		} else {
			b.Narrative = resp.Text
		}
	}

	return b, nil
}

// formatBrief renders the canonical Macro Pulse text report.
func formatBrief(b *brief) string {
	var s strings.Builder
	asOf := b.AsOf.UTC().Format("2006-01-02 15:04 UTC")
	fmt.Fprintf(&s, "=== Agora Macro Pulse — %s ===\n\n", asOf)

	if len(b.Markets) > 0 {
		fmt.Fprintln(&s, "MARKETS (7d)")
		for _, m := range b.Markets {
			fmt.Fprintf(&s, "  %-18s %-12s %s\n", m.Label, m.Value, signedPct(m.ChangePct))
		}
		fmt.Fprintln(&s)
	}
	if len(b.Weather) > 0 {
		fmt.Fprintln(&s, "WEATHER (current + 72h, economic hubs)")
		for _, w := range b.Weather {
			fmt.Fprintf(&s, "  %-22s %s\n", w.City, w.Summary)
		}
		fmt.Fprintln(&s)
	}
	if len(b.Signals) > 0 {
		fmt.Fprintln(&s, "SIGNALS (search 30d, news 7d)")
		for _, sig := range b.Signals {
			switch sig.Unit {
			case "%":
				fmt.Fprintf(&s, "  %-26s %s\n", sig.Label+":", signedPct(sig.Value))
			case "tone":
				fmt.Fprintf(&s, "  %-26s %+.2f\n", sig.Label+":", sig.Value)
			default:
				fmt.Fprintf(&s, "  %-26s %.2f %s\n", sig.Label+":", sig.Value, sig.Unit)
			}
		}
		fmt.Fprintln(&s)
	}
	if len(b.Correlations) > 0 {
		fmt.Fprintln(&s, "CORRELATIONS (30d, |r| > 0.4)")
		for _, c := range b.Correlations {
			if math.Abs(c.R) < 0.4 {
				continue
			}
			fmt.Fprintf(&s, "  %-36s r = %+.2f\n", c.Pair+":", c.R)
		}
		fmt.Fprintln(&s)
	}
	if b.Narrative != "" {
		fmt.Fprintln(&s, "BRIEF")
		// Word-wrap the narrative at ~74 columns with a 2-space indent.
		for _, line := range wrapLines(b.Narrative, 74) {
			fmt.Fprintf(&s, "  %s\n", line)
		}
		fmt.Fprintln(&s)
	}
	if len(b.Catalog) > 0 {
		fmt.Fprintln(&s, "CATALOG")
		for _, c := range b.Catalog {
			fmt.Fprintf(&s, "  %-20s %s  caps=%v\n", c.Name, c.AdvertisementID, c.Capabilities)
		}
	}
	return s.String()
}

// formatBriefMarkdown renders the brief as a GitHub-flavored Markdown
// document suitable for committing to a repo, posting to an internal
// wiki, or attaching to a chat thread.
func formatBriefMarkdown(b *brief) string {
	var s strings.Builder
	asOf := b.AsOf.UTC().Format("2006-01-02 15:04 UTC")
	fmt.Fprintf(&s, "# Agora Macro Pulse — %s\n\n", asOf)
	fmt.Fprintln(&s, "Composed across independent provider and tool agents over Agora-governed sessions. Snapshot or `--live` mode; per-capability JSON envelopes flow through OpenZiti tunnels under a contract that bounds session duration and message types.")
	fmt.Fprintln(&s)

	if len(b.Markets) > 0 {
		fmt.Fprintln(&s, "## Markets (7d)")
		fmt.Fprintln(&s)
		fmt.Fprintln(&s, "| Asset | Value | Change |")
		fmt.Fprintln(&s, "| --- | ---: | ---: |")
		for _, m := range b.Markets {
			fmt.Fprintf(&s, "| %s | %s | %s |\n", m.Label, m.Value, signedPct(m.ChangePct))
		}
		fmt.Fprintln(&s)
	}
	if len(b.Weather) > 0 {
		fmt.Fprintln(&s, "## Weather (current + 72h, economic hubs)")
		fmt.Fprintln(&s)
		for _, w := range b.Weather {
			fmt.Fprintf(&s, "- **%s** — %s\n", w.City, w.Summary)
		}
		fmt.Fprintln(&s)
	}
	if len(b.Signals) > 0 {
		fmt.Fprintln(&s, "## Signals (search 30d, news 7d)")
		fmt.Fprintln(&s)
		for _, sig := range b.Signals {
			switch sig.Unit {
			case "%":
				fmt.Fprintf(&s, "- %s — %s\n", sig.Label, signedPct(sig.Value))
			case "tone":
				fmt.Fprintf(&s, "- %s — %+.2f\n", sig.Label, sig.Value)
			default:
				fmt.Fprintf(&s, "- %s — %.2f %s\n", sig.Label, sig.Value, sig.Unit)
			}
		}
		fmt.Fprintln(&s)
	}
	notable := make([]correlationLine, 0, len(b.Correlations))
	for _, c := range b.Correlations {
		if math.Abs(c.R) >= 0.4 {
			notable = append(notable, c)
		}
	}
	if len(notable) > 0 {
		fmt.Fprintln(&s, "## Correlations (30d, |r| > 0.4)")
		fmt.Fprintln(&s)
		fmt.Fprintln(&s, "| Pair | Pearson r |")
		fmt.Fprintln(&s, "| --- | ---: |")
		for _, c := range notable {
			fmt.Fprintf(&s, "| %s | %+.2f |\n", c.Pair, c.R)
		}
		fmt.Fprintln(&s)
	}
	if b.Narrative != "" {
		fmt.Fprintln(&s, "## Brief")
		fmt.Fprintln(&s)
		fmt.Fprintln(&s, b.Narrative)
		fmt.Fprintln(&s)
	}
	if len(b.Catalog) > 0 {
		fmt.Fprintln(&s, "## Catalog")
		fmt.Fprintln(&s)
		fmt.Fprintln(&s, "| Advertisement | ID | Capabilities |")
		fmt.Fprintln(&s, "| --- | --- | --- |")
		for _, c := range b.Catalog {
			fmt.Fprintf(&s, "| %s | `%s` | %s |\n", c.Name, c.AdvertisementID, strings.Join(c.Capabilities, ", "))
		}
		fmt.Fprintln(&s)
	}
	fmt.Fprintln(&s, "---")
	fmt.Fprintf(&s, "_Generated by `macro-pulse-pulse-agent` over %d advertisements; deterministic narrator (no LLM)._\n", len(b.Catalog))
	return s.String()
}

// --- helpers ---------------------------------------------------------------

func signedPct(v float64) string {
	sign := "+"
	if v < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s%.1f%%", sign, math.Abs(v))
}

func newer(a, b time.Time) time.Time {
	if a.IsZero() || b.After(a) {
		return b
	}
	return a
}

func wrapLines(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	out := []string{}
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > width {
			out = append(out, cur)
			cur = w
		} else {
			cur += " " + w
		}
	}
	out = append(out, cur)
	return out
}
