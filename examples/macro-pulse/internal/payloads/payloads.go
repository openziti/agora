// Package payloads holds the JSON-shaped Go types exchanged in
// envelope payloads for the Macro Pulse demo. Per-capability message
// types are documented in each provider/tool agent's README.
package payloads

import "time"

// SeriesPoint is a daily observation in a time series.
type SeriesPoint struct {
	T string  `json:"t"` // ISO date YYYY-MM-DD
	V float64 `json:"v"`
}

// NewsSeriesPoint is one row of a topic's daily volume + tone series.
type NewsSeriesPoint struct {
	T      string  `json:"t"`
	Volume int     `json:"volume"`
	Tone   float64 `json:"tone"`
}

// --- markets.equity ---------------------------------------------------------

type EquityRequest struct {
	Tickers    []string `json:"tickers"`
	WindowDays int      `json:"window_days"`
}

type EquityTickerData struct {
	Price     float64       `json:"price"`
	ChangePct float64       `json:"change_pct"`
	Series    []SeriesPoint `json:"series"`
}

type EquityResponse struct {
	AsOf       time.Time                   `json:"as_of"`
	WindowDays int                         `json:"window_days"`
	Tickers    map[string]EquityTickerData `json:"tickers"`
}

// --- markets.fx -------------------------------------------------------------

type FXRequest struct {
	Pairs      []string `json:"pairs"`
	WindowDays int      `json:"window_days"`
}

type FXPairData struct {
	Rate      float64       `json:"rate"`
	ChangePct float64       `json:"change_pct"`
	Series    []SeriesPoint `json:"series"`
}

type FXResponse struct {
	AsOf       time.Time             `json:"as_of"`
	WindowDays int                   `json:"window_days"`
	Pairs      map[string]FXPairData `json:"pairs"`
}

// --- markets.commodities ----------------------------------------------------

type CommoditiesRequest struct {
	Symbols    []string `json:"symbols"`
	WindowDays int      `json:"window_days"`
}

type CommoditySymbolData struct {
	Price     float64       `json:"price"`
	ChangePct float64       `json:"change_pct"`
	Series    []SeriesPoint `json:"series"`
}

type CommoditiesResponse struct {
	AsOf       time.Time                      `json:"as_of"`
	WindowDays int                            `json:"window_days"`
	Symbols    map[string]CommoditySymbolData `json:"symbols"`
}

// --- weather.current --------------------------------------------------------

type WeatherCurrentRequest struct {
	Cities []string `json:"cities"`
}

type WeatherCurrentCityData struct {
	TempF     float64 `json:"temp_f"`
	Condition string  `json:"condition"`
	AnomalyF  float64 `json:"anomaly_f"`
}

type WeatherCurrentResponse struct {
	AsOf   time.Time                         `json:"as_of"`
	Cities map[string]WeatherCurrentCityData `json:"cities"`
}

// --- weather.forecast -------------------------------------------------------

type WeatherForecastRequest struct {
	Cities       []string `json:"cities"`
	HorizonHours int      `json:"horizon_hours"`
}

type WeatherForecastDay struct {
	Date       string  `json:"date"`
	TempHighF  float64 `json:"temp_high_f"`
	TempLowF   float64 `json:"temp_low_f"`
	Condition  string  `json:"condition"`
}

type WeatherForecastCityData struct {
	Daily []WeatherForecastDay `json:"daily"`
}

type WeatherForecastResponse struct {
	AsOf         time.Time                          `json:"as_of"`
	HorizonHours int                                `json:"horizon_hours"`
	Cities       map[string]WeatherForecastCityData `json:"cities"`
}

// --- signals.search ---------------------------------------------------------

type SearchRequest struct {
	Terms      []string `json:"terms"`
	WindowDays int      `json:"window_days"`
}

type SearchTermData struct {
	ChangePct   float64       `json:"change_pct"`
	BaselineAvg float64       `json:"baseline_avg"`
	CurrentAvg  float64       `json:"current_avg"`
	Series      []SeriesPoint `json:"series"`
}

type SearchResponse struct {
	AsOf       time.Time                 `json:"as_of"`
	WindowDays int                       `json:"window_days"`
	Terms      map[string]SearchTermData `json:"terms"`
}

// --- signals.news -----------------------------------------------------------

type NewsRequest struct {
	Topics     []string `json:"topics"`
	WindowDays int      `json:"window_days"`
}

type NewsTopicData struct {
	Volume int               `json:"volume"`
	Tone   float64           `json:"tone"`
	Series []NewsSeriesPoint `json:"series"`
}

type NewsResponse struct {
	AsOf       time.Time                `json:"as_of"`
	WindowDays int                      `json:"window_days"`
	Topics     map[string]NewsTopicData `json:"topics"`
}

// --- analytics.correlate ----------------------------------------------------

type CorrelateLabeledSeries struct {
	Label  string        `json:"label"`
	Points []SeriesPoint `json:"points"`
}

type CorrelateRequest struct {
	SeriesA CorrelateLabeledSeries `json:"series_a"`
	SeriesB CorrelateLabeledSeries `json:"series_b"`
}

type CorrelateResponse struct {
	PearsonR float64 `json:"pearson_r"`
	N        int     `json:"n"`
	LabelA   string  `json:"label_a"`
	LabelB   string  `json:"label_b"`
}

// --- analytics.narrate ------------------------------------------------------

// NarrateInputs is the structured digest the orchestrator hands to
// the narrator. Fields mirror the brief's section headings.
type NarrateInputs struct {
	Markets      []NarrateMarketLine      `json:"markets"`
	Weather      []NarrateWeatherLine     `json:"weather"`
	Signals      []NarrateSignalLine      `json:"signals"`
	Correlations []NarrateCorrelationLine `json:"correlations"`
}

type NarrateMarketLine struct {
	Label     string  `json:"label"`
	Value     string  `json:"value"`
	ChangePct float64 `json:"change_pct"`
}

type NarrateWeatherLine struct {
	City    string `json:"city"`
	Summary string `json:"summary"`
}

type NarrateSignalLine struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type NarrateCorrelationLine struct {
	Pair string  `json:"pair"`
	R    float64 `json:"r"`
}

type NarrateRequest struct {
	Template string        `json:"template"`
	Inputs   NarrateInputs `json:"inputs"`
}

type NarrateResponse struct {
	Text string `json:"text"`
}
