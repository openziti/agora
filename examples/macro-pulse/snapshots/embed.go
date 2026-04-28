// Package snapshots embeds the offline snapshot data shipped with the
// Macro Pulse demo and exposes typed loaders for each capability.
//
// Files under this directory are baked into every demo binary at
// build time so the demo can run with no external dependencies in
// snapshot mode. Adding a new file requires updating the embed
// pattern below.
package snapshots

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
)

//go:embed equity/*.json fx/*.json commodities/*.json weather/*.json search/*.json news/*.json
var fs embed.FS

// LoadEquity reads the embedded snapshot for a single equity ticker
// (e.g. "SPY", "XLK", "XLE", "XLF").
func LoadEquity(symbol string) (*equityFile, error) {
	path := fmt.Sprintf("equity/%s.json", symbol)
	return readJSON[equityFile](path)
}

// LoadFX reads the embedded snapshot for a single FX pair
// (e.g. "USD-EUR", "USD-JPY").
func LoadFX(pair string) (*fxFile, error) {
	path := fmt.Sprintf("fx/%s.json", pair)
	return readJSON[fxFile](path)
}

// LoadCommodity reads the embedded snapshot for a single futures
// symbol (e.g. "CL_F", "GC_F", "NG_F").
func LoadCommodity(symbol string) (*commodityFile, error) {
	path := fmt.Sprintf("commodities/%s.json", symbol)
	return readJSON[commodityFile](path)
}

// LoadWeatherCurrent reads the embedded current-conditions snapshot
// covering all known cities.
func LoadWeatherCurrent() (*weatherCurrentFile, error) {
	return readJSON[weatherCurrentFile]("weather/current.json")
}

// LoadWeatherForecast reads the embedded forecast snapshot covering
// all known cities.
func LoadWeatherForecast() (*weatherForecastFile, error) {
	return readJSON[weatherForecastFile]("weather/forecast.json")
}

// LoadSearchTerm reads the embedded snapshot for a single search term.
func LoadSearchTerm(term string) (*searchTermFile, error) {
	path := fmt.Sprintf("search/%s.json", term)
	return readJSON[searchTermFile](path)
}

// LoadNewsTopic reads the embedded snapshot for a single news topic.
func LoadNewsTopic(topic string) (*newsTopicFile, error) {
	path := fmt.Sprintf("news/%s.json", topic)
	return readJSON[newsTopicFile](path)
}

func readJSON[T any](path string) (*T, error) {
	bytes, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("snapshot %q not embedded: %w", path, err)
	}
	var v T
	if err := json.Unmarshal(bytes, &v); err != nil {
		return nil, fmt.Errorf("decode snapshot %q: %w", path, err)
	}
	return &v, nil
}

// On-disk file shapes. These match the JSON layout authored under
// the per-capability subdirectories.

type equityFile struct {
	Symbol         string                 `json:"symbol"`
	Name           string                 `json:"name"`
	AsOf           time.Time              `json:"as_of"`
	Price          float64                `json:"price"`
	ChangePct1d    float64                `json:"change_pct_1d"`
	ChangePct7d    float64                `json:"change_pct_7d"`
	ChangePct30d   float64                `json:"change_pct_30d"`
	Series         []payloads.SeriesPoint `json:"series"`
}

type fxFile struct {
	Pair           string                 `json:"pair"`
	AsOf           time.Time              `json:"as_of"`
	Rate           float64                `json:"rate"`
	ChangePct1d    float64                `json:"change_pct_1d"`
	ChangePct7d    float64                `json:"change_pct_7d"`
	ChangePct30d   float64                `json:"change_pct_30d"`
	Series         []payloads.SeriesPoint `json:"series"`
}

type commodityFile struct {
	Symbol         string                 `json:"symbol"`
	Name           string                 `json:"name"`
	AsOf           time.Time              `json:"as_of"`
	Price          float64                `json:"price"`
	ChangePct1d    float64                `json:"change_pct_1d"`
	ChangePct7d    float64                `json:"change_pct_7d"`
	ChangePct30d   float64                `json:"change_pct_30d"`
	Series         []payloads.SeriesPoint `json:"series"`
}

type weatherCurrentFile struct {
	AsOf   time.Time                                  `json:"as_of"`
	Cities map[string]payloads.WeatherCurrentCityData `json:"cities"`
}

type weatherForecastFile struct {
	AsOf         time.Time                                    `json:"as_of"`
	HorizonHours int                                          `json:"horizon_hours"`
	Cities       map[string]payloads.WeatherForecastCityData  `json:"cities"`
}

type searchTermFile struct {
	Term         string                 `json:"term"`
	AsOf         time.Time              `json:"as_of"`
	BaselineAvg  float64                `json:"baseline_avg"`
	CurrentAvg   float64                `json:"current_avg"`
	ChangePct    float64                `json:"change_pct"`
	Series       []payloads.SeriesPoint `json:"series"`
}

type newsTopicFile struct {
	Topic   string                     `json:"topic"`
	AsOf    time.Time                  `json:"as_of"`
	Volume  int                        `json:"volume"`
	Tone    float64                    `json:"tone"`
	Series  []payloads.NewsSeriesPoint `json:"series"`
}

// Equity, FX, Commodity, Search, and News file types each project to
// a payloads.* response struct. The methods below produce a per-symbol
// slice of the response shape, after window-trimming.

func (f *equityFile) ToTickerData(windowDays int) (payloads.EquityTickerData, time.Time) {
	return payloads.EquityTickerData{
		Price:     f.Price,
		ChangePct: pickChange(f.ChangePct1d, f.ChangePct7d, f.ChangePct30d, windowDays),
		Series:    trimSeries(f.Series, windowDays),
	}, f.AsOf
}

func (f *fxFile) ToPairData(windowDays int) (payloads.FXPairData, time.Time) {
	return payloads.FXPairData{
		Rate:      f.Rate,
		ChangePct: pickChange(f.ChangePct1d, f.ChangePct7d, f.ChangePct30d, windowDays),
		Series:    trimSeries(f.Series, windowDays),
	}, f.AsOf
}

func (f *commodityFile) ToSymbolData(windowDays int) (payloads.CommoditySymbolData, time.Time) {
	return payloads.CommoditySymbolData{
		Price:     f.Price,
		ChangePct: pickChange(f.ChangePct1d, f.ChangePct7d, f.ChangePct30d, windowDays),
		Series:    trimSeries(f.Series, windowDays),
	}, f.AsOf
}

func (f *searchTermFile) ToTermData(windowDays int) (payloads.SearchTermData, time.Time) {
	return payloads.SearchTermData{
		ChangePct:   f.ChangePct,
		BaselineAvg: f.BaselineAvg,
		CurrentAvg:  f.CurrentAvg,
		Series:      trimSeries(f.Series, windowDays),
	}, f.AsOf
}

func (f *newsTopicFile) ToTopicData(windowDays int) (payloads.NewsTopicData, time.Time) {
	return payloads.NewsTopicData{
		Volume: f.Volume,
		Tone:   f.Tone,
		Series: trimNewsSeries(f.Series, windowDays),
	}, f.AsOf
}

func (f *weatherCurrentFile) Cities_() (time.Time, map[string]payloads.WeatherCurrentCityData) {
	return f.AsOf, f.Cities
}
func (f *weatherForecastFile) ForecastFor() (time.Time, int, map[string]payloads.WeatherForecastCityData) {
	return f.AsOf, f.HorizonHours, f.Cities
}

// pickChange maps a requested window_days to the pre-computed change
// field on the snapshot file. 1d / 7d / 30d are the only granularities
// snapshots track; an unknown value falls back to 7d.
func pickChange(d1, d7, d30 float64, windowDays int) float64 {
	switch {
	case windowDays <= 1:
		return d1
	case windowDays <= 7:
		return d7
	default:
		return d30
	}
}

// trimSeries returns the last min(len, windowDays) points. windowDays
// of 0 means "return all".
func trimSeries(in []payloads.SeriesPoint, windowDays int) []payloads.SeriesPoint {
	if windowDays <= 0 || windowDays >= len(in) {
		return in
	}
	return in[len(in)-windowDays:]
}

func trimNewsSeries(in []payloads.NewsSeriesPoint, windowDays int) []payloads.NewsSeriesPoint {
	if windowDays <= 0 || windowDays >= len(in) {
		return in
	}
	return in[len(in)-windowDays:]
}

// AvailableTickers returns the set of equity tickers that have a
// snapshot embedded. Useful for tests and self-discovery.
func AvailableTickers() []string {
	return availableUnits("equity")
}

func availableUnits(domain string) []string {
	entries, err := fs.ReadDir(domain)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".json"))
	}
	return out
}
