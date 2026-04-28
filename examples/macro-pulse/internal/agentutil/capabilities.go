package agentutil

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
	"github.com/openziti/agora/examples/macro-pulse/snapshots"
)

// EquityHandle serves equity-feed requests against the embedded
// snapshots. Unknown tickers are silently skipped; the response
// returns whatever subset was found.
func EquityHandle(_ context.Context, req payloads.EquityRequest) (payloads.EquityResponse, error) {
	out := payloads.EquityResponse{
		WindowDays: req.WindowDays,
		Tickers:    make(map[string]payloads.EquityTickerData, len(req.Tickers)),
	}
	for _, t := range normalize(req.Tickers) {
		f, err := snapshots.LoadEquity(t)
		if err != nil {
			continue
		}
		data, asOf := f.ToTickerData(req.WindowDays)
		out.Tickers[t] = data
		if out.AsOf.IsZero() {
			out.AsOf = asOf
		}
	}
	if len(out.Tickers) == 0 {
		return out, fmt.Errorf("no known tickers in request: %v", req.Tickers)
	}
	return out, nil
}

// FXHandle serves fx-feed requests.
func FXHandle(_ context.Context, req payloads.FXRequest) (payloads.FXResponse, error) {
	out := payloads.FXResponse{
		WindowDays: req.WindowDays,
		Pairs:      make(map[string]payloads.FXPairData, len(req.Pairs)),
	}
	for _, p := range normalize(req.Pairs) {
		f, err := snapshots.LoadFX(p)
		if err != nil {
			continue
		}
		data, asOf := f.ToPairData(req.WindowDays)
		out.Pairs[p] = data
		if out.AsOf.IsZero() {
			out.AsOf = asOf
		}
	}
	if len(out.Pairs) == 0 {
		return out, fmt.Errorf("no known pairs in request: %v", req.Pairs)
	}
	return out, nil
}

// CommoditiesHandle serves commodities-feed requests.
func CommoditiesHandle(_ context.Context, req payloads.CommoditiesRequest) (payloads.CommoditiesResponse, error) {
	out := payloads.CommoditiesResponse{
		WindowDays: req.WindowDays,
		Symbols:    make(map[string]payloads.CommoditySymbolData, len(req.Symbols)),
	}
	for _, s := range normalize(req.Symbols) {
		f, err := snapshots.LoadCommodity(s)
		if err != nil {
			continue
		}
		data, asOf := f.ToSymbolData(req.WindowDays)
		out.Symbols[s] = data
		if out.AsOf.IsZero() {
			out.AsOf = asOf
		}
	}
	if len(out.Symbols) == 0 {
		return out, fmt.Errorf("no known symbols in request: %v", req.Symbols)
	}
	return out, nil
}

// WeatherCurrentHandle serves weather.current requests against the
// embedded snapshot. The single embedded file holds all known cities;
// the response is filtered to whatever subset was requested.
func WeatherCurrentHandle(_ context.Context, req payloads.WeatherCurrentRequest) (payloads.WeatherCurrentResponse, error) {
	f, err := snapshots.LoadWeatherCurrent()
	if err != nil {
		return payloads.WeatherCurrentResponse{}, err
	}
	asOf, all := f.Cities_()
	out := payloads.WeatherCurrentResponse{
		AsOf:   asOf,
		Cities: make(map[string]payloads.WeatherCurrentCityData, len(req.Cities)),
	}
	for _, c := range normalize(req.Cities) {
		if data, ok := all[c]; ok {
			out.Cities[c] = data
		}
	}
	if len(out.Cities) == 0 {
		return out, fmt.Errorf("no known cities in request: %v", req.Cities)
	}
	return out, nil
}

// WeatherForecastHandle serves weather.forecast requests.
func WeatherForecastHandle(_ context.Context, req payloads.WeatherForecastRequest) (payloads.WeatherForecastResponse, error) {
	f, err := snapshots.LoadWeatherForecast()
	if err != nil {
		return payloads.WeatherForecastResponse{}, err
	}
	asOf, horizonHours, all := f.ForecastFor()
	if req.HorizonHours > 0 && req.HorizonHours < horizonHours {
		horizonHours = req.HorizonHours
	}
	out := payloads.WeatherForecastResponse{
		AsOf:         asOf,
		HorizonHours: horizonHours,
		Cities:       make(map[string]payloads.WeatherForecastCityData, len(req.Cities)),
	}
	maxDays := int(math.Ceil(float64(horizonHours) / 24))
	for _, c := range normalize(req.Cities) {
		if data, ok := all[c]; ok {
			daily := data.Daily
			if maxDays > 0 && len(daily) > maxDays {
				daily = daily[:maxDays]
			}
			out.Cities[c] = payloads.WeatherForecastCityData{Daily: daily}
		}
	}
	if len(out.Cities) == 0 {
		return out, fmt.Errorf("no known cities in request: %v", req.Cities)
	}
	return out, nil
}

// SearchHandle serves signals.search requests.
func SearchHandle(_ context.Context, req payloads.SearchRequest) (payloads.SearchResponse, error) {
	out := payloads.SearchResponse{
		WindowDays: req.WindowDays,
		Terms:      make(map[string]payloads.SearchTermData, len(req.Terms)),
	}
	for _, t := range normalize(req.Terms) {
		f, err := snapshots.LoadSearchTerm(t)
		if err != nil {
			continue
		}
		data, asOf := f.ToTermData(req.WindowDays)
		out.Terms[t] = data
		if out.AsOf.IsZero() {
			out.AsOf = asOf
		}
	}
	if len(out.Terms) == 0 {
		return out, fmt.Errorf("no known terms in request: %v", req.Terms)
	}
	return out, nil
}

// NewsHandle serves signals.news requests.
func NewsHandle(_ context.Context, req payloads.NewsRequest) (payloads.NewsResponse, error) {
	out := payloads.NewsResponse{
		WindowDays: req.WindowDays,
		Topics:     make(map[string]payloads.NewsTopicData, len(req.Topics)),
	}
	for _, t := range normalize(req.Topics) {
		f, err := snapshots.LoadNewsTopic(t)
		if err != nil {
			continue
		}
		data, asOf := f.ToTopicData(req.WindowDays)
		out.Topics[t] = data
		if out.AsOf.IsZero() {
			out.AsOf = asOf
		}
	}
	if len(out.Topics) == 0 {
		return out, fmt.Errorf("no known topics in request: %v", req.Topics)
	}
	return out, nil
}

// CorrelateHandle computes a Pearson correlation coefficient between
// two aligned-by-date time series. Only points with matching `t`
// values contribute; the response carries the count and the labels
// (no leak of underlying tickers/cities/terms).
func CorrelateHandle(_ context.Context, req payloads.CorrelateRequest) (payloads.CorrelateResponse, error) {
	a := req.SeriesA.Points
	b := req.SeriesB.Points
	bByT := make(map[string]float64, len(b))
	for _, p := range b {
		bByT[p.T] = p.V
	}
	xs := make([]float64, 0, len(a))
	ys := make([]float64, 0, len(a))
	for _, p := range a {
		if v, ok := bByT[p.T]; ok {
			xs = append(xs, p.V)
			ys = append(ys, v)
		}
	}
	resp := payloads.CorrelateResponse{
		LabelA: req.SeriesA.Label,
		LabelB: req.SeriesB.Label,
		N:      len(xs),
	}
	if resp.N < 2 {
		return resp, fmt.Errorf("not enough overlapping observations: %d", resp.N)
	}
	resp.PearsonR = pearson(xs, ys)
	return resp, nil
}

// NarrateHandle composes a deterministic prose summary of the
// brief's structured inputs. Template-driven; no LLM.
func NarrateHandle(_ context.Context, req payloads.NarrateRequest) (payloads.NarrateResponse, error) {
	var b strings.Builder
	if len(req.Inputs.Markets) > 0 {
		marketsHave := func(label string) (payloads.NarrateMarketLine, bool) {
			for _, m := range req.Inputs.Markets {
				if strings.EqualFold(m.Label, label) {
					return m, true
				}
			}
			return payloads.NarrateMarketLine{}, false
		}
		var pieces []string
		if m, ok := marketsHave("Gold"); ok && m.ChangePct > 1 {
			pieces = append(pieces, "gold bid suggests defensive positioning")
		}
		if m, ok := marketsHave("S&P 500"); ok && m.ChangePct < 0 {
			pieces = append(pieces, "equity softness")
		}
		if len(pieces) > 0 {
			b.WriteString("Markets show ")
			b.WriteString(joinList(pieces))
			b.WriteString(". ")
		}
	}
	stormSeen := false
	for _, w := range req.Inputs.Weather {
		if strings.Contains(strings.ToLower(w.Summary), "storm") || strings.Contains(strings.ToLower(w.Summary), "tropical") {
			stormSeen = true
		}
	}
	if stormSeen {
		b.WriteString("Energy complex faces near-term upside risk from Gulf weather. ")
	}
	for _, s := range req.Inputs.Signals {
		ll := strings.ToLower(s.Label)
		if strings.Contains(ll, "layoffs") && s.Value > 20 {
			b.WriteString("Rising employment-anxiety signals align with equity softness. ")
			break
		}
	}
	highCorrelations := []payloads.NarrateCorrelationLine{}
	for _, c := range req.Inputs.Correlations {
		if math.Abs(c.R) >= 0.4 {
			highCorrelations = append(highCorrelations, c)
		}
	}
	if len(highCorrelations) > 0 {
		sort.Slice(highCorrelations, func(i, j int) bool {
			return math.Abs(highCorrelations[i].R) > math.Abs(highCorrelations[j].R)
		})
		c := highCorrelations[0]
		b.WriteString(fmt.Sprintf("Strongest cross-domain signal: %s (r=%+.2f). ", c.Pair, c.R))
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		text = "No notable cross-domain signals in this window."
	}
	return payloads.NarrateResponse{Text: text}, nil
}

// --- internals --------------------------------------------------------------

func pearson(xs, ys []float64) float64 {
	n := float64(len(xs))
	if n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
		sumY2 += ys[i] * ys[i]
	}
	num := n*sumXY - sumX*sumY
	den := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}

func normalize(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func joinList(ss []string) string {
	switch len(ss) {
	case 0:
		return ""
	case 1:
		return ss[0]
	case 2:
		return ss[0] + " and " + ss[1]
	default:
		return strings.Join(ss[:len(ss)-1], ", ") + ", and " + ss[len(ss)-1]
	}
}

// Used by the orchestrator to convert duration to a label.
var _ = time.Duration(0)
