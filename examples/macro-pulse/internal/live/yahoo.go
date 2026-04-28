package live

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
)

// yahooSymbol resolves an Agora-side symbol (the way it appears in
// requests) to the Yahoo chart-API symbol. Most map 1:1 but FX pairs
// and some commodity futures need a dedicated upstream symbol.
func yahooSymbol(sym string) string {
	switch strings.ToUpper(sym) {
	case "USD-EUR":
		return "EURUSD=X" // we'll invert below to express USD per EUR
	case "USD-JPY":
		return "JPY=X"
	case "USD-GBP":
		return "GBPUSD=X" // we'll invert below
	case "CL_F":
		return "CL=F"
	case "GC_F":
		return "GC=F"
	case "NG_F":
		return "NG=F"
	default:
		return sym
	}
}

// inversePair reports whether the upstream returns the inverse of the
// requested orientation (Agora-side `USD-EUR` requests "USD per EUR"
// while Yahoo's EURUSD=X is "EUR per USD"). For inverted pairs the
// adapter applies 1/x to all values before returning.
func inversePair(sym string) bool {
	switch strings.ToUpper(sym) {
	case "USD-EUR", "USD-GBP":
		return true
	default:
		return false
	}
}

// yahooChartResponse is the subset of the Yahoo Finance chart-API
// payload the adapter parses.
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol              string  `json:"symbol"`
				RegularMarketPrice  float64 `json:"regularMarketPrice"`
				ChartPreviousClose  float64 `json:"chartPreviousClose"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error any `json:"error"`
	} `json:"chart"`
}

// fetchYahooSymbol pulls one daily series from Yahoo's chart endpoint
// for the supplied request symbol.
func fetchYahooSymbol(ctx context.Context, requestSymbol string, windowDays int) (price float64, changePct float64, series []payloads.SeriesPoint, asOf time.Time, err error) {
	rangeParam := chartRangeFor(windowDays)
	upstream := yahooSymbol(requestSymbol)
	u := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=%s",
		url.PathEscape(upstream), rangeParam,
	)
	var raw yahooChartResponse
	if err = getJSON(ctx, u, &raw); err != nil {
		return
	}
	if len(raw.Chart.Result) == 0 {
		err = fmt.Errorf("yahoo chart: empty result for %q", upstream)
		return
	}
	r := raw.Chart.Result[0]
	if len(r.Timestamp) == 0 || len(r.Indicators.Quote) == 0 {
		err = fmt.Errorf("yahoo chart: no series points for %q", upstream)
		return
	}
	closes := r.Indicators.Quote[0].Close

	invert := inversePair(requestSymbol)

	pts := make([]payloads.SeriesPoint, 0, len(r.Timestamp))
	for i, ts := range r.Timestamp {
		if i >= len(closes) {
			break
		}
		c := closes[i]
		if math.IsNaN(c) || c == 0 {
			continue
		}
		v := c
		if invert {
			v = 1 / c
		}
		pts = append(pts, payloads.SeriesPoint{
			T: time.Unix(ts, 0).UTC().Format("2006-01-02"),
			V: round4(v),
		})
	}
	if len(pts) == 0 {
		err = fmt.Errorf("yahoo chart: all close values for %q were nil/zero", upstream)
		return
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].T < pts[j].T })

	last := pts[len(pts)-1].V
	if r.Meta.RegularMarketPrice != 0 {
		// prefer the live price for "current" while keeping the daily
		// series as a stable historical record.
		v := r.Meta.RegularMarketPrice
		if invert {
			v = 1 / v
		}
		price = round4(v)
	} else {
		price = last
	}

	changePct = computeChangePct(pts, windowDays)
	asOf = time.Now().UTC()
	if windowDays > 0 && windowDays < len(pts) {
		series = pts[len(pts)-windowDays:]
	} else {
		series = pts
	}
	return
}

func chartRangeFor(windowDays int) string {
	switch {
	case windowDays <= 5:
		return "5d"
	case windowDays <= 30:
		return "1mo"
	case windowDays <= 90:
		return "3mo"
	case windowDays <= 180:
		return "6mo"
	default:
		return "1y"
	}
}

// computeChangePct returns the percent change from N points back to
// the most recent point, where N = min(windowDays, len(pts)-1).
func computeChangePct(pts []payloads.SeriesPoint, windowDays int) float64 {
	if len(pts) < 2 {
		return 0
	}
	end := pts[len(pts)-1].V
	idx := 0
	if windowDays > 0 && windowDays < len(pts) {
		idx = len(pts) - 1 - windowDays
	}
	start := pts[idx].V
	if start == 0 {
		return 0
	}
	return round2(((end - start) / start) * 100)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// FetchEquity populates an EquityResponse for the requested tickers
// from the Yahoo Finance chart endpoint. Per-ticker failures are
// tolerated (the ticker is omitted); a hard error is returned only
// when no ticker succeeded.
func FetchEquity(ctx context.Context, req payloads.EquityRequest) (payloads.EquityResponse, error) {
	out := payloads.EquityResponse{
		WindowDays: req.WindowDays,
		Tickers:    make(map[string]payloads.EquityTickerData, len(req.Tickers)),
	}
	for _, t := range req.Tickers {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		price, change, series, asOf, err := fetchYahooSymbol(ctx, t, req.WindowDays)
		if err != nil {
			continue
		}
		out.Tickers[t] = payloads.EquityTickerData{Price: price, ChangePct: change, Series: series}
		if out.AsOf.IsZero() || asOf.After(out.AsOf) {
			out.AsOf = asOf
		}
	}
	if len(out.Tickers) == 0 {
		return out, fmt.Errorf("yahoo equity: no tickers returned for %v", req.Tickers)
	}
	return out, nil
}

// FetchFX populates an FXResponse for the requested currency pairs.
func FetchFX(ctx context.Context, req payloads.FXRequest) (payloads.FXResponse, error) {
	out := payloads.FXResponse{
		WindowDays: req.WindowDays,
		Pairs:      make(map[string]payloads.FXPairData, len(req.Pairs)),
	}
	for _, p := range req.Pairs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		rate, change, series, asOf, err := fetchYahooSymbol(ctx, p, req.WindowDays)
		if err != nil {
			continue
		}
		out.Pairs[p] = payloads.FXPairData{Rate: rate, ChangePct: change, Series: series}
		if out.AsOf.IsZero() || asOf.After(out.AsOf) {
			out.AsOf = asOf
		}
	}
	if len(out.Pairs) == 0 {
		return out, fmt.Errorf("yahoo fx: no pairs returned for %v", req.Pairs)
	}
	return out, nil
}

// FetchCommodities populates a CommoditiesResponse for the requested
// futures symbols.
func FetchCommodities(ctx context.Context, req payloads.CommoditiesRequest) (payloads.CommoditiesResponse, error) {
	out := payloads.CommoditiesResponse{
		WindowDays: req.WindowDays,
		Symbols:    make(map[string]payloads.CommoditySymbolData, len(req.Symbols)),
	}
	for _, s := range req.Symbols {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		price, change, series, asOf, err := fetchYahooSymbol(ctx, s, req.WindowDays)
		if err != nil {
			continue
		}
		out.Symbols[s] = payloads.CommoditySymbolData{Price: price, ChangePct: change, Series: series}
		if out.AsOf.IsZero() || asOf.After(out.AsOf) {
			out.AsOf = asOf
		}
	}
	if len(out.Symbols) == 0 {
		return out, fmt.Errorf("yahoo commodities: no symbols returned for %v", req.Symbols)
	}
	return out, nil
}
