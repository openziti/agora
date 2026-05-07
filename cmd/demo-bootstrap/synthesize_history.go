package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type syntheticAuditEvent struct {
	OccurredAt      time.Time
	EventType       string
	OrganizationID  string
	AccountID       *string
	WorkgroupID     *string
	SessionID       *string
	AdvertisementID *string
	ContractID      *string
	EnvelopeID      *string
	Data            map[string]any
}

func synthesizeHistory(path string, duration time.Duration, topo *topology, orgIDs map[string]string, accounts map[string]seededAccount, workgroupIDs map[string]string, contractIDs map[string]string) (int, error) {
	if duration <= 0 {
		return 0, nil
	}
	return synthesizeHistoryAt(path, duration, topo, orgIDs, accounts, workgroupIDs, contractIDs, time.Now().UTC())
}

func synthesizeHistoryAt(path string, duration time.Duration, topo *topology, orgIDs map[string]string, accounts map[string]seededAccount, workgroupIDs map[string]string, contractIDs map[string]string, now time.Time) (int, error) {
	if duration <= 0 {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, err
	}

	events, err := synthesizeHistoryEvents(duration, topo, orgIDs, accounts, workgroupIDs, contractIDs, now)
	if err != nil {
		return 0, err
	}
	sql, err := renderSyntheticHistorySQL(events)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, []byte(sql), 0o600); err != nil {
		return 0, err
	}
	return len(events), nil
}

func synthesizeHistoryEvents(duration time.Duration, topo *topology, orgIDs map[string]string, accounts map[string]seededAccount, workgroupIDs map[string]string, contractIDs map[string]string, now time.Time) ([]syntheticAuditEvent, error) {
	if duration <= 0 {
		return nil, nil
	}

	profile := topo.History.withDefaults()
	providers := syntheticProviders(topo, profile)
	consumer, ok := accounts[profile.ConsumerEmail]
	if !ok {
		return nil, fmt.Errorf("synthetic history requires %s account", profile.ConsumerEmail)
	}
	consumerOrgID := orgIDs[consumer.Spec.Organization]
	if consumer.ID == "" || consumerOrgID == "" {
		return nil, fmt.Errorf("synthetic history requires provisioned consumer account and organization ids for %s", profile.ConsumerEmail)
	}

	now = now.UTC().Truncate(time.Hour)
	start := now.Add(-duration).Truncate(time.Hour)
	r := rand.New(rand.NewSource(now.Unix() / int64(time.Hour/time.Second)))
	events := make([]syntheticAuditEvent, 0, int(duration.Hours())*len(providers)*8)

	sessionSeq := 0
	for bucket := start; bucket.Before(now); bucket = bucket.Add(time.Hour) {
		activity := jitteredActivity(hourlyActivity(bucket, profile), profile.HourlyJitter, r)
		for _, provider := range providers {
			providerAccount := accounts[provider.Email]
			providerOrgID := orgIDs[providerAccount.Spec.Organization]
			workgroupID := workgroupIDs[provider.Workgroup]
			contractID := contractIDs[contractKey(provider.Email, provider.Contract)]
			if providerAccount.ID == "" || providerOrgID == "" || workgroupID == "" || contractID == "" {
				continue
			}
			sessions := sampleSessionCount(provider.Weight*activity, profile.SessionBurstProbability, r)
			for i := 0; i < sessions; i++ {
				sessionSeq++
				appendSyntheticSession(&events, syntheticSessionInput{
					Now:             now,
					Bucket:          bucket,
					Activity:        activity,
					Profile:         profile,
					Random:          r,
					Sequence:        sessionSeq,
					Provider:        provider,
					ProviderAccount: providerAccount,
					ProviderOrgID:   providerOrgID,
					Consumer:        consumer,
					ConsumerOrgID:   consumerOrgID,
					WorkgroupID:     workgroupID,
					ContractID:      contractID,
				})
			}
		}
	}

	sort.Slice(events, func(i, j int) bool { return events[i].OccurredAt.Before(events[j].OccurredAt) })
	return events, nil
}

type syntheticSessionInput struct {
	Now             time.Time
	Bucket          time.Time
	Activity        float64
	Profile         historySpec
	Random          *rand.Rand
	Sequence        int
	Provider        syntheticProvider
	ProviderAccount seededAccount
	ProviderOrgID   string
	Consumer        seededAccount
	ConsumerOrgID   string
	WorkgroupID     string
	ContractID      string
}

func appendSyntheticSession(events *[]syntheticAuditEvent, input syntheticSessionInput) {
	r := input.Random
	adID := syntheticAdvertisementID(input.Provider.AdvertisementName)
	sessionID := fmt.Sprintf("sess_seed_%012d", input.Sequence)
	countDelta := sampleEnvelopeCount(input.Profile.EnvelopeCount, input.Provider.EnvelopeMultiplier, input.Activity, r)
	totalCount := countDelta
	proposedAt := input.Bucket.Add(time.Duration(r.Intn(46))*time.Minute + time.Duration(r.Intn(60))*time.Second)
	acceptedAt := proposedAt.Add(time.Duration(1+r.Intn(4)) * time.Second)
	flowedAt := acceptedAt.Add(time.Duration(20+r.Intn(130)) * time.Second)
	closedAt := flowedAt.Add(sampleSessionDuration(input.Profile, r))
	if !closedAt.Before(input.Now) {
		closedAt = input.Now.Add(-1 * time.Minute)
	}
	if !closedAt.After(flowedAt) {
		closedAt = flowedAt.Add(time.Minute)
	}

	closeReason, closeDetail, violationDimension := sampleCloseOutcome(input.Profile, input.Provider.Contract, r)

	sessionData := map[string]any{
		"provider_account_id":      input.ProviderAccount.ID,
		"provider_organization_id": input.ProviderOrgID,
		"consumer_account_id":      input.Consumer.ID,
		"consumer_organization_id": input.ConsumerOrgID,
		"tunnel_mode":              "tcp",
	}
	appendTwoParty(events, proposedAt, "session.proposed", input.ProviderOrgID, input.ProviderAccount.ID, input.ConsumerOrgID, input.Consumer.ID, input.WorkgroupID, sessionID, adID, "", sessionData)
	acceptedData := cloneData(sessionData)
	acceptedData["contract_id"] = input.ContractID
	appendTwoParty(events, acceptedAt, "session.accepted", input.ProviderOrgID, input.ProviderAccount.ID, input.ConsumerOrgID, input.Consumer.ID, input.WorkgroupID, sessionID, adID, input.ContractID, acceptedData)
	flowData := cloneData(sessionData)
	flowData["count_delta"] = countDelta
	flowData["total_count"] = totalCount
	appendTwoParty(events, flowedAt, "envelope.flowed", input.ProviderOrgID, input.ProviderAccount.ID, input.ConsumerOrgID, input.Consumer.ID, input.WorkgroupID, sessionID, adID, input.ContractID, flowData)
	closedData := map[string]any{
		"provider_organization_id": input.ProviderOrgID,
		"consumer_organization_id": input.ConsumerOrgID,
		"close_reason":             closeReason,
		"close_detail":             closeDetail,
		"duration_seconds":         int64(closedAt.Sub(proposedAt).Seconds()),
	}
	if violationDimension != "" {
		closedData["violation_dimension"] = violationDimension
	}
	appendTwoParty(events, closedAt, "session.closed", input.ProviderOrgID, input.ProviderAccount.ID, input.ConsumerOrgID, input.Consumer.ID, input.WorkgroupID, sessionID, adID, input.ContractID, closedData)
}

func renderSyntheticHistorySQL(events []syntheticAuditEvent) (string, error) {
	sort.Slice(events, func(i, j int) bool { return events[i].OccurredAt.Before(events[j].OccurredAt) })
	var b strings.Builder
	b.WriteString("-- Synthetic Agora dashboard history generated by demo-bootstrap.\n")
	b.WriteString("-- Load with psql -f after controller migrations have applied.\n")
	b.WriteString("begin;\n")
	for _, event := range events {
		line, err := renderAuditInsert(event)
		if err != nil {
			return "", err
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("commit;\n")
	return b.String(), nil
}

type syntheticProvider struct {
	Email              string
	AdvertisementName  string
	Workgroup          string
	Contract           string
	Weight             float64
	EnvelopeMultiplier float64
}

func syntheticProviders(topo *topology, profile historySpec) []syntheticProvider {
	providers := make([]syntheticProvider, 0)
	for _, acct := range topo.Accounts {
		if acct.Gateway != "" || acct.Advertisement.Contract == "" {
			continue
		}
		workgroupProfile := profile.workgroup(acct.Advertisement.Workgroup)
		providers = append(providers, syntheticProvider{
			Email:              acct.Email,
			AdvertisementName:  acct.Advertisement.Name,
			Workgroup:          acct.Advertisement.Workgroup,
			Contract:           acct.Advertisement.Contract,
			Weight:             workgroupProfile.Weight,
			EnvelopeMultiplier: workgroupProfile.EnvelopeMultiplier,
		})
	}
	return providers
}

func (h historySpec) withDefaults() historySpec {
	if h.ConsumerEmail == "" &&
		h.WeekdayScale == 0 &&
		h.WeekendScale == 0 &&
		h.OvernightFloor == 0 &&
		h.MorningPeakHour == 0 &&
		h.MorningPeakWeight == 0 &&
		h.BusinessPeakHour == 0 &&
		h.BusinessPeakWeight == 0 &&
		h.EnvelopeCount.Min == 0 &&
		h.EnvelopeCount.Max == 0 &&
		len(h.Workgroups) == 0 {
		return defaultHistorySpec()
	}
	return h
}

func defaultHistorySpec() historySpec {
	return historySpec{
		ConsumerEmail:                     "demo@agora.local",
		WeekdayScale:                      1,
		WeekendScale:                      0.42,
		OvernightFloor:                    0.08,
		MorningPeakHour:                   9,
		MorningPeakWeight:                 0.34,
		BusinessPeakHour:                  14,
		BusinessPeakWeight:                0.92,
		HourlyJitter:                      0.16,
		SessionBurstProbability:           0.18,
		ProviderCloseProbability:          0.16,
		LongTailProbability:               0.1,
		ContractViolationProbability:      0.04,
		TightContractViolationProbability: 0.24,
		EnvelopeCount:                     historyRangeSpec{Min: 8, Max: 42},
	}
}

func (h historySpec) workgroup(name string) historyWorkgroupSpec {
	for _, wg := range h.Workgroups {
		if strings.EqualFold(wg.Name, name) {
			if wg.EnvelopeMultiplier == 0 {
				wg.EnvelopeMultiplier = 1
			}
			return wg
		}
	}
	return defaultHistoryWorkgroup(name)
}

func defaultHistoryWorkgroup(name string) historyWorkgroupSpec {
	switch name {
	case "markets-channel":
		return historyWorkgroupSpec{Name: name, Weight: 0.78, EnvelopeMultiplier: 1.45}
	case "signals-channel":
		return historyWorkgroupSpec{Name: name, Weight: 0.62, EnvelopeMultiplier: 1.22}
	case "analytics-channel":
		return historyWorkgroupSpec{Name: name, Weight: 0.48, EnvelopeMultiplier: 1.05}
	case "weather-channel":
		return historyWorkgroupSpec{Name: name, Weight: 0.34, EnvelopeMultiplier: 0.82}
	default:
		return historyWorkgroupSpec{Name: name, Weight: 0.4, EnvelopeMultiplier: 1}
	}
}

func hourlyActivity(t time.Time, profile historySpec) float64 {
	hour := float64(t.Hour())
	business := profile.BusinessPeakWeight * math.Exp(-math.Pow((hour-float64(profile.BusinessPeakHour))/4.6, 2))
	morning := profile.MorningPeakWeight * math.Exp(-math.Pow((hour-float64(profile.MorningPeakHour))/2.8, 2))
	activity := profile.OvernightFloor + business + morning
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		activity *= profile.WeekendScale
	} else {
		activity *= profile.WeekdayScale
	}
	activity *= dailyVariation(t)
	return clampFloat(activity, 0, 1.6)
}

func dailyVariation(t time.Time) float64 {
	day := float64(t.YearDay())
	weekly := 0.14 * math.Sin((day/7)*2*math.Pi+0.8)
	monthly := 0.07 * math.Sin((day/29)*2*math.Pi+1.7)
	return 1 + weekly + monthly
}

func jitteredActivity(activity, jitter float64, r *rand.Rand) float64 {
	if jitter <= 0 {
		return activity
	}
	factor := 1 + ((r.Float64()*2 - 1) * jitter)
	return clampFloat(activity*factor, 0, 1.8)
}

func sampleSessionCount(expected, burstProbability float64, r *rand.Rand) int {
	if expected <= 0 {
		return 0
	}
	count := int(expected)
	if r.Float64() < expected-float64(count) {
		count++
	}
	if count > 0 && r.Float64() < burstProbability*math.Min(expected, 1) {
		count++
	}
	return count
}

func sampleEnvelopeCount(bounds historyRangeSpec, workgroupMultiplier, activity float64, r *rand.Rand) int {
	minCount := bounds.Min
	maxCount := bounds.Max
	if minCount <= 0 {
		minCount = 8
	}
	if maxCount < minCount {
		maxCount = minCount
	}
	base := minCount
	if maxCount > minCount {
		base += r.Intn(maxCount - minCount + 1)
	}
	if workgroupMultiplier <= 0 {
		workgroupMultiplier = 1
	}
	activityMultiplier := 0.72 + 0.38*math.Min(activity, 1.3)
	noise := 0.85 + r.Float64()*0.3
	return maxInt(1, int(math.Round(float64(base)*workgroupMultiplier*activityMultiplier*noise)))
}

func sampleSessionDuration(profile historySpec, r *rand.Rand) time.Duration {
	duration := time.Duration(40+r.Intn(420)) * time.Second
	if r.Float64() < profile.LongTailProbability {
		duration += time.Duration(15+r.Intn(75)) * time.Minute
	}
	return duration
}

func sampleCloseOutcome(profile historySpec, contract string, r *rand.Rand) (closeReason, closeDetail, violationDimension string) {
	violationProbability := profile.ContractViolationProbability
	if contract == "demo-contract-tight" {
		violationProbability = profile.TightContractViolationProbability
	}
	if r.Float64() < violationProbability {
		dimension := sampleViolationDimension(r)
		return "contract_violation", "synthetic demo history " + strings.ReplaceAll(dimension, "_", "-") + " violation", dimension
	}
	if r.Float64() < profile.ProviderCloseProbability {
		return "provider_close", "synthetic demo history provider close", ""
	}
	return "consumer_close", "synthetic demo history normal close", ""
}

func sampleViolationDimension(r *rand.Rand) string {
	switch r.Intn(3) {
	case 0:
		return "max_duration"
	case 1:
		return "max_envelope_bytes"
	default:
		return "message_type"
	}
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func appendTwoParty(events *[]syntheticAuditEvent, occurredAt time.Time, eventType, providerOrgID, providerAccountID, consumerOrgID, consumerAccountID, workgroupID, sessionID, advertisementID, contractID string, data map[string]any) {
	contractPtr := stringPtrOrNil(contractID)
	*events = append(*events, syntheticAuditEvent{
		OccurredAt:      occurredAt,
		EventType:       eventType,
		OrganizationID:  providerOrgID,
		AccountID:       stringPtrValue(providerAccountID),
		WorkgroupID:     stringPtrValue(workgroupID),
		SessionID:       stringPtrValue(sessionID),
		AdvertisementID: stringPtrValue(advertisementID),
		ContractID:      contractPtr,
		Data:            cloneData(data),
	})
	if providerOrgID == consumerOrgID {
		return
	}
	*events = append(*events, syntheticAuditEvent{
		OccurredAt:      occurredAt,
		EventType:       eventType,
		OrganizationID:  consumerOrgID,
		AccountID:       stringPtrValue(consumerAccountID),
		WorkgroupID:     stringPtrValue(workgroupID),
		SessionID:       stringPtrValue(sessionID),
		AdvertisementID: stringPtrValue(advertisementID),
		ContractID:      contractPtr,
		Data:            cloneData(data),
	})
}

func renderAuditInsert(event syntheticAuditEvent) (string, error) {
	data := event.Data
	if data == nil {
		data = map[string]any{}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"insert into audit_events (occurred_at, event_type, organization_id, account_id, workgroup_id, session_id, advertisement_id, contract_id, envelope_id, data) values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb);",
		sqlString(event.OccurredAt.Format(time.RFC3339Nano)),
		sqlString(event.EventType),
		sqlString(event.OrganizationID),
		sqlStringPtr(event.AccountID),
		sqlStringPtr(event.WorkgroupID),
		sqlStringPtr(event.SessionID),
		sqlStringPtr(event.AdvertisementID),
		sqlStringPtr(event.ContractID),
		sqlStringPtr(event.EnvelopeID),
		sqlString(string(encoded)),
	), nil
}

func syntheticAdvertisementID(name string) string {
	cleaned := strings.NewReplacer("-", "", "_", "", "@", "", ".", "").Replace(name)
	if len(cleaned) > 12 {
		cleaned = cleaned[:12]
	}
	return "ad_seed_" + cleaned
}

func cloneData(data map[string]any) map[string]any {
	cloned := make(map[string]any, len(data))
	for k, v := range data {
		cloned[k] = v
	}
	return cloned
}

func stringPtrValue(v string) *string {
	return &v
}

func stringPtrOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func sqlStringPtr(v *string) string {
	if v == nil {
		return "null"
	}
	return sqlString(*v)
}

func sqlString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}
