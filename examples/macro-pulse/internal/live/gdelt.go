package live

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
)

// gdeltQueryByTopic maps a Macro-Pulse topic to the GDELT DOC-API query
// string used to filter the timeline. GDELT supports rich query syntax
// (phrase, theme:, sourcecountry:, sourcelang:, etc.); we keep things
// simple with phrase queries that surface meaningful tone signal.
var gdeltQueryByTopic = map[string]string{
	"financial":    `("financial markets" OR "stock market" OR "S&P 500")`,
	"energy":       `("oil price" OR "natural gas" OR "OPEC")`,
	"supply-chain": `("supply chain" OR shipping OR logistics)`,
}

type gdeltTimelineResponse struct {
	Timeline []struct {
		Series string `json:"series"`
		Data   []struct {
			Date  string  `json:"date"`
			Value float64 `json:"value"`
		} `json:"data"`
	} `json:"timeline"`
}

func gdeltURL(query string, days int) string {
	if days < 1 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	return fmt.Sprintf(
		"https://api.gdeltproject.org/api/v2/doc/doc?query=%s&mode=timelinetone&format=json&timespan=%dd",
		url.QueryEscape(query),
		days,
	)
}

// gdeltMinInterval is the minimum gap between successive GDELT
// requests. The public API documents "one request every 5 seconds"
// for free use; we add a small margin.
const gdeltMinInterval = 6 * time.Second

// FetchNews populates a NewsResponse using GDELT's timelinetone mode
// per topic. Volume is derived as a coarse proxy from the number of
// data points returned; tone is the GDELT "Average Tone" series.
func FetchNews(ctx context.Context, req payloads.NewsRequest) (payloads.NewsResponse, error) {
	out := payloads.NewsResponse{
		AsOf:       time.Now().UTC(),
		WindowDays: req.WindowDays,
		Topics:     make(map[string]payloads.NewsTopicData, len(req.Topics)),
	}
	window := req.WindowDays
	if window <= 0 {
		window = 7
	}
	first := true
	var lastErr error
	for _, topic := range req.Topics {
		topic = strings.TrimSpace(strings.ToLower(topic))
		query, ok := gdeltQueryByTopic[topic]
		if !ok {
			continue
		}
		if !first {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(gdeltMinInterval):
			}
		}
		first = false

		var raw gdeltTimelineResponse
		if err := getJSON(ctx, gdeltURL(query, window), &raw); err != nil {
			lastErr = fmt.Errorf("topic %q: %w", topic, err)
			continue
		}
		if len(raw.Timeline) == 0 || len(raw.Timeline[0].Data) == 0 {
			lastErr = fmt.Errorf("topic %q: empty timeline", topic)
			continue
		}
		toneSeries := raw.Timeline[0].Data

		series := make([]payloads.NewsSeriesPoint, 0, len(toneSeries))
		var sum float64
		for _, p := range toneSeries {
			day := parseGDELTDate(p.Date)
			if day.IsZero() {
				continue
			}
			series = append(series, payloads.NewsSeriesPoint{
				T:      day.Format("2006-01-02"),
				Volume: 1000, // GDELT timelinetone returns no per-point volume; placeholder.
				Tone:   round2(p.Value),
			})
			sum += p.Value
		}
		if len(series) == 0 {
			continue
		}
		// Aggregate: tone = average of per-point tones; volume = data
		// point count * 1000 as a coarse proxy for upstream activity.
		avgTone := sum / float64(len(series))
		out.Topics[topic] = payloads.NewsTopicData{
			Volume: len(series) * 1000,
			Tone:   round2(avgTone),
			Series: series,
		}
	}
	if len(out.Topics) == 0 {
		if lastErr != nil {
			return out, fmt.Errorf("gdelt: no topics returned for %v: %w", req.Topics, lastErr)
		}
		return out, fmt.Errorf("gdelt: no topics returned for %v", req.Topics)
	}
	return out, nil
}

func parseGDELTDate(s string) time.Time {
	// GDELT formats dates as YYYYMMDDTHHMMSSZ.
	t, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		// Try YYYYMMDD only.
		if t2, err2 := time.Parse("20060102", s); err2 == nil {
			return t2.UTC()
		}
		return time.Time{}
	}
	return t.UTC()
}
