package main

import (
	"strings"
	"testing"
	"time"
)

func TestHistoryProfileShowsDailyAndWeekendRhythm(t *testing.T) {
	topo, err := loadTopology("")
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}
	profile := topo.History.withDefaults()

	weekdayPeak := hourlyActivity(time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC), profile)
	weekdayNight := hourlyActivity(time.Date(2026, 5, 6, 2, 0, 0, 0, time.UTC), profile)
	weekendPeak := hourlyActivity(time.Date(2026, 5, 3, 14, 0, 0, 0, time.UTC), profile)

	if weekdayPeak <= weekdayNight*4 {
		t.Fatalf("expected weekday peak to dominate overnight activity, peak=%f night=%f", weekdayPeak, weekdayNight)
	}
	if weekendPeak >= weekdayPeak*0.7 {
		t.Fatalf("expected weekend peak to be materially lower than weekday peak, weekday=%f weekend=%f", weekdayPeak, weekendPeak)
	}

	markets := profile.workgroup("markets-channel")
	weather := profile.workgroup("weather-channel")
	if markets.Weight <= weather.Weight || markets.EnvelopeMultiplier <= weather.EnvelopeMultiplier {
		t.Fatalf("expected markets profile to be busier than weather, markets=%+v weather=%+v", markets, weather)
	}
}

func TestSynthesizeHistoryProducesVariedActivity(t *testing.T) {
	topo, err := loadTopology("")
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}
	orgIDs, accounts, workgroupIDs, contractIDs := fakeHistoryInputs(topo)
	now := time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC)

	events, err := synthesizeHistoryEvents(14*24*time.Hour, topo, orgIDs, accounts, workgroupIDs, contractIDs, now)
	if err != nil {
		t.Fatalf("synthesize history: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected synthetic events")
	}

	consumerOrgID := orgIDs["enterprise-client"]
	var businessHours int
	var overnight int
	var weekdayTotal int
	var weekendTotal int
	weekdayDays, weekendDays := countWeekdayWeekendDays(now.Add(-14*24*time.Hour), now)
	workgroupTotals := map[string]int{}
	violationCount := 0

	for _, event := range events {
		if event.OrganizationID != consumerOrgID {
			continue
		}
		switch event.EventType {
		case "envelope.flowed":
			envelopes := eventDataInt(event, "count_delta")
			hour := event.OccurredAt.Hour()
			if hour >= 9 && hour <= 16 {
				businessHours += envelopes
			}
			if hour <= 4 || hour >= 22 {
				overnight += envelopes
			}
			if event.OccurredAt.Weekday() == time.Saturday || event.OccurredAt.Weekday() == time.Sunday {
				weekendTotal += envelopes
			} else {
				weekdayTotal += envelopes
			}
			if event.WorkgroupID != nil {
				workgroupTotals[*event.WorkgroupID] += envelopes
			}
		case "session.closed":
			if eventDataString(event, "close_reason") == "contract_violation" {
				violationCount++
			}
		}
	}

	if businessHours <= overnight*3 {
		t.Fatalf("expected business-hour envelope volume to dominate overnight volume, business=%d overnight=%d", businessHours, overnight)
	}
	if weekdayTotal/weekdayDays <= weekendTotal/weekendDays {
		t.Fatalf("expected weekday average volume to exceed weekend average, weekday=%d/%d weekend=%d/%d", weekdayTotal, weekdayDays, weekendTotal, weekendDays)
	}
	if workgroupTotals[workgroupIDs["markets-channel"]] <= workgroupTotals[workgroupIDs["weather-channel"]]*2 {
		t.Fatalf("expected markets-channel to outweigh weather-channel, totals=%v", workgroupTotals)
	}
	if violationCount == 0 {
		t.Fatal("expected at least one synthetic contract violation")
	}
}

func fakeHistoryInputs(topo *topology) (map[string]string, map[string]seededAccount, map[string]string, map[string]string) {
	orgIDs := map[string]string{}
	for _, org := range topo.Organizations {
		orgIDs[org.Name] = "org_" + historySlug(org.Name)
	}

	workgroupIDs := map[string]string{}
	for _, wg := range topo.Workgroups {
		workgroupIDs[wg.Name] = "wg_" + historySlug(wg.Name)
	}

	accounts := map[string]seededAccount{}
	contractIDs := map[string]string{}
	for _, acct := range topo.Accounts {
		accounts[acct.Email] = seededAccount{
			Spec: acct,
			ID:   "acct_" + historySlug(acct.Email),
		}
		if acct.Advertisement.Contract != "" {
			contractIDs[contractKey(acct.Email, acct.Advertisement.Contract)] = "con_" + historySlug(acct.Email+"_"+acct.Advertisement.Contract)
		}
	}
	return orgIDs, accounts, workgroupIDs, contractIDs
}

func historySlug(value string) string {
	replacer := strings.NewReplacer("@", "_", ".", "_", "-", "_")
	return replacer.Replace(strings.ToLower(value))
}

func eventDataInt(event syntheticAuditEvent, key string) int {
	switch value := event.Data[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func eventDataString(event syntheticAuditEvent, key string) string {
	value, _ := event.Data[key].(string)
	return value
}

func countWeekdayWeekendDays(start, end time.Time) (weekdayDays, weekendDays int) {
	for day := start.UTC().Truncate(24 * time.Hour); day.Before(end); day = day.Add(24 * time.Hour) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			weekendDays++
		} else {
			weekdayDays++
		}
	}
	return weekdayDays, weekendDays
}
