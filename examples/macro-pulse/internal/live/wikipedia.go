package live

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
)

// wikiArticleByTerm maps a Macro-Pulse search term to the Wikipedia
// article whose pageview series serves as a proxy for search interest.
// Picked to be on-topic and have stable enough traffic to show meaningful
// movement, while staying free of disambiguation collisions.
var wikiArticleByTerm = map[string]string{
	"layoffs":         "Layoff",
	"gulf-storm":      "Tropical_cyclone",
	"housing-market":  "United_States_housing_bubble",
}

type wikiPageviewsResponse struct {
	Items []struct {
		Project     string `json:"project"`
		Article     string `json:"article"`
		Granularity string `json:"granularity"`
		Timestamp   string `json:"timestamp"`
		Access      string `json:"access"`
		Agent       string `json:"agent"`
		Views       int    `json:"views"`
	} `json:"items"`
}

func wikiURL(article string, start, end time.Time) string {
	return fmt.Sprintf(
		"https://wikimedia.org/api/rest_v1/metrics/pageviews/per-article/en.wikipedia.org/all-access/all-agents/%s/daily/%s/%s",
		url.PathEscape(article),
		start.Format("20060102"),
		end.Format("20060102"),
	)
}

// FetchSearchTrends populates a SearchResponse using Wikipedia
// Pageviews per term as a proxy for relative search interest.
func FetchSearchTrends(ctx context.Context, req payloads.SearchRequest) (payloads.SearchResponse, error) {
	out := payloads.SearchResponse{
		AsOf:       time.Now().UTC(),
		WindowDays: req.WindowDays,
		Terms:      make(map[string]payloads.SearchTermData, len(req.Terms)),
	}
	end := time.Now().UTC().Add(-24 * time.Hour) // pageviews lag 1 day
	window := req.WindowDays
	if window <= 0 {
		window = 30
	}
	// Pull two windows so we can compute baseline (older) vs current (newer).
	start := end.AddDate(0, 0, -2*window)

	for _, t := range req.Terms {
		t = strings.TrimSpace(strings.ToLower(t))
		article, ok := wikiArticleByTerm[t]
		if !ok {
			continue
		}
		var raw wikiPageviewsResponse
		if err := getJSON(ctx, wikiURL(article, start, end), &raw); err != nil {
			continue
		}
		points := make([]payloads.SeriesPoint, 0, len(raw.Items))
		for _, it := range raw.Items {
			day, parseErr := parseWikiTimestamp(it.Timestamp)
			if parseErr != nil {
				continue
			}
			points = append(points, payloads.SeriesPoint{
				T: day.Format("2006-01-02"),
				V: float64(it.Views),
			})
		}
		if len(points) < 2 {
			continue
		}
		baseline, current := splitForBaselineAvg(points, window)
		change := 0.0
		if baseline > 0 {
			change = round2(((current - baseline) / baseline) * 100)
		}
		series := points
		if window > 0 && window < len(points) {
			series = points[len(points)-window:]
		}
		out.Terms[t] = payloads.SearchTermData{
			ChangePct:   change,
			BaselineAvg: round2(baseline),
			CurrentAvg:  round2(current),
			Series:      series,
		}
	}
	if len(out.Terms) == 0 {
		return out, fmt.Errorf("wikipedia pageviews: no terms returned for %v", req.Terms)
	}
	return out, nil
}

func parseWikiTimestamp(ts string) (time.Time, error) {
	// Format: YYYYMMDDHH (we treat it as a date).
	if len(ts) < 8 {
		return time.Time{}, fmt.Errorf("wiki timestamp too short: %q", ts)
	}
	year, err := strconv.Atoi(ts[:4])
	if err != nil {
		return time.Time{}, err
	}
	month, err := strconv.Atoi(ts[4:6])
	if err != nil {
		return time.Time{}, err
	}
	day, err := strconv.Atoi(ts[6:8])
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// splitForBaselineAvg returns (baselineAvg, currentAvg) where baseline
// is the older windowDays and current is the newer windowDays.
func splitForBaselineAvg(points []payloads.SeriesPoint, windowDays int) (float64, float64) {
	if windowDays <= 0 || len(points) <= windowDays {
		// not enough data to split
		all := avgPoints(points)
		return all, all
	}
	current := points[len(points)-windowDays:]
	baseline := points[:len(points)-windowDays]
	if len(baseline) > windowDays {
		baseline = baseline[len(baseline)-windowDays:]
	}
	return avgPoints(baseline), avgPoints(current)
}

func avgPoints(points []payloads.SeriesPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	var sum float64
	for _, p := range points {
		sum += p.V
	}
	return sum / float64(len(points))
}
