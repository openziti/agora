package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
)

func main() {
	fs := flag.NewFlagSet("pulse-agent", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Emit the brief as a JSON document instead of formatted text")
	markdownPath := fs.String("markdown", "", "Also write the brief as a Markdown document to the given path")

	app := agent.New("pulse-agent",
		agent.WithDescription("Macro Pulse orchestrator: composes across providers and tools to produce the morning brief"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		brief, err := buildBrief(ctx, a)
		if err != nil {
			a.Log().Errorf("build brief: %v", err)
			return err
		}

		if *markdownPath != "" {
			if err := os.WriteFile(*markdownPath, []byte(formatBriefMarkdown(brief)), 0o644); err != nil {
				return fmt.Errorf("write markdown brief: %w", err)
			}
			a.Log().With("path", *markdownPath).Infof("wrote markdown brief")
		}

		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(brief)
		}
		fmt.Print(formatBrief(brief))
		return nil
	}); err != nil {
		os.Exit(1)
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

// query opens a session against ad, sends one envelope of msgType
// carrying req as JSON, decodes the reply into out, and closes.
//
// The 90s timeout here is sized for the slowest --live path: GDELT's
// public API throttles to ~1 request per 5 seconds, so a single news
// query against three topics serially takes ~20-25s before the
// snapshot fallback even starts. Snapshot-only runs return well
// under a second per query.
func query[Req any, Resp any](ctx context.Context, a *agent.Agent, ad api.Advertisement, msgType string, req Req, out *Resp) error {
	if len(ad.WorkgroupScopes) == 0 {
		return fmt.Errorf("advertisement %s has no visible workgroup", ad.ID)
	}
	sessCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	sess, err := session.Propose(sessCtx, a, ad.ID, session.ProposeOptions{
		WorkgroupID: ad.WorkgroupScopes[0],
		Message:     "macro-pulse morning brief",
		Timeout:     20 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("propose %s: %w", msgType, err)
	}
	defer func() {
		_ = sess.Close(ctx, "brief complete")
	}()

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
	return nil
}

func buildBrief(ctx context.Context, a *agent.Agent) (*brief, error) {
	res, err := a.Controller().SearchCatalog(ctx, api.SearchCatalogParams{})
	if err != nil {
		return nil, fmt.Errorf("catalog search: %w", err)
	}
	listing, ok := res.(*api.CatalogSearchResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected catalog search response: %T", res)
	}
	a.Log().With("count", len(listing.Items)).Infof("discovered advertisements via catalog")

	adByCapability := map[string]api.Advertisement{}
	catalog := make([]catalogEntry, 0, len(listing.Items))
	for _, ad := range listing.Items {
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
		if err := query(ctx, a, ad, "markets.equity.request", req, &resp); err != nil {
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
		if err := query(ctx, a, ad, "markets.fx.request", req, &resp); err != nil {
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
		if err := query(ctx, a, ad, "markets.commodities.request", req, &resp); err != nil {
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
		if err := query(ctx, a, ad, "weather.current.request", req, &resp); err != nil {
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
		if err := query(ctx, a, ad, "weather.forecast.request", req, &resp); err != nil {
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
		if err := query(ctx, a, ad, "signals.search.request", req, &resp); err != nil {
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
		if err := query(ctx, a, ad, "signals.news.request", req, &resp); err != nil {
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
			if err := query(ctx, a, ad, "analytics.correlate.request", req, &resp); err != nil {
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
		if err := query(ctx, a, ad, "analytics.narrate.request", req, &resp); err != nil {
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
