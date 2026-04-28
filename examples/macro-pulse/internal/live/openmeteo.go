package live

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
)

// cityCoord is a hand-curated lat/lon for each known economic-hub
// city. Open-Meteo expects coordinates rather than place names and
// has no built-in geocoder on the free forecast endpoint.
type cityCoord struct {
	Lat, Lon float64
	Label    string // human-friendly summary used in current-conditions
}

var cityCoords = map[string]cityCoord{
	"new-york":  {Lat: 40.7128, Lon: -74.0060, Label: "New York"},
	"houston":   {Lat: 29.7604, Lon: -95.3698, Label: "Houston"},
	"frankfurt": {Lat: 50.1109, Lon: 8.6821, Label: "Frankfurt"},
	"singapore": {Lat: 1.3521, Lon: 103.8198, Label: "Singapore"},
}

// seasonalNormalF is a coarse climatological "normal" April-ish high
// per city (in Fahrenheit). Used to compute the displayed anomaly.
// Not climate-quality data; sufficient for the demo's narrative.
var seasonalNormalF = map[string]float64{
	"new-york":  56,
	"houston":   78,
	"frankfurt": 60,
	"singapore": 88,
}

type openMeteoResponse struct {
	Current struct {
		Time          string  `json:"time"`
		Temperature2m float64 `json:"temperature_2m"`
		WeatherCode   int     `json:"weather_code"`
	} `json:"current"`
	Daily struct {
		Time           []string  `json:"time"`
		TempMax        []float64 `json:"temperature_2m_max"`
		TempMin        []float64 `json:"temperature_2m_min"`
		WeatherCode    []int     `json:"weather_code"`
	} `json:"daily"`
}

func openMeteoURL(c cityCoord, forecastDays int) string {
	return fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,weather_code&daily=temperature_2m_max,temperature_2m_min,weather_code&forecast_days=%d&temperature_unit=fahrenheit&timezone=UTC",
		c.Lat, c.Lon, forecastDays,
	)
}

// FetchWeatherCurrent fills in WeatherCurrentResponse from Open-Meteo.
func FetchWeatherCurrent(ctx context.Context, req payloads.WeatherCurrentRequest) (payloads.WeatherCurrentResponse, error) {
	out := payloads.WeatherCurrentResponse{
		AsOf:   time.Now().UTC(),
		Cities: make(map[string]payloads.WeatherCurrentCityData, len(req.Cities)),
	}
	for _, c := range req.Cities {
		c = strings.TrimSpace(strings.ToLower(c))
		coord, ok := cityCoords[c]
		if !ok {
			continue
		}
		var raw openMeteoResponse
		if err := getJSON(ctx, openMeteoURL(coord, 1), &raw); err != nil {
			continue
		}
		anomaly := raw.Current.Temperature2m - seasonalNormalF[c]
		out.Cities[c] = payloads.WeatherCurrentCityData{
			TempF:     round2(raw.Current.Temperature2m),
			Condition: weatherCodeLabel(raw.Current.WeatherCode),
			AnomalyF:  round2(anomaly),
		}
	}
	if len(out.Cities) == 0 {
		return out, fmt.Errorf("open-meteo current: no cities returned for %v", req.Cities)
	}
	return out, nil
}

// FetchWeatherForecast fills in WeatherForecastResponse from Open-Meteo.
func FetchWeatherForecast(ctx context.Context, req payloads.WeatherForecastRequest) (payloads.WeatherForecastResponse, error) {
	horizonHours := req.HorizonHours
	if horizonHours <= 0 {
		horizonHours = 72
	}
	days := int(math.Ceil(float64(horizonHours) / 24))
	if days < 1 {
		days = 1
	}
	if days > 7 {
		days = 7
	}
	out := payloads.WeatherForecastResponse{
		AsOf:         time.Now().UTC(),
		HorizonHours: horizonHours,
		Cities:       make(map[string]payloads.WeatherForecastCityData, len(req.Cities)),
	}
	for _, c := range req.Cities {
		c = strings.TrimSpace(strings.ToLower(c))
		coord, ok := cityCoords[c]
		if !ok {
			continue
		}
		var raw openMeteoResponse
		if err := getJSON(ctx, openMeteoURL(coord, days), &raw); err != nil {
			continue
		}
		daily := make([]payloads.WeatherForecastDay, 0, len(raw.Daily.Time))
		for i, day := range raw.Daily.Time {
			if i >= len(raw.Daily.TempMax) || i >= len(raw.Daily.TempMin) || i >= len(raw.Daily.WeatherCode) {
				break
			}
			daily = append(daily, payloads.WeatherForecastDay{
				Date:      day,
				TempHighF: round2(raw.Daily.TempMax[i]),
				TempLowF:  round2(raw.Daily.TempMin[i]),
				Condition: weatherCodeLabel(raw.Daily.WeatherCode[i]),
			})
		}
		if len(daily) == 0 {
			continue
		}
		out.Cities[c] = payloads.WeatherForecastCityData{Daily: daily}
	}
	if len(out.Cities) == 0 {
		return out, fmt.Errorf("open-meteo forecast: no cities returned for %v", req.Cities)
	}
	return out, nil
}

// weatherCodeLabel maps an Open-Meteo WMO weather code to a short
// human-readable phrase. Reference:
// https://open-meteo.com/en/docs#weathervariables
func weatherCodeLabel(code int) string {
	switch {
	case code == 0:
		return "clear sky"
	case code == 1:
		return "mainly clear"
	case code == 2:
		return "partly cloudy"
	case code == 3:
		return "overcast"
	case code == 45 || code == 48:
		return "fog"
	case code >= 51 && code <= 57:
		return "drizzle"
	case code >= 61 && code <= 67:
		return "rain"
	case code >= 71 && code <= 77:
		return "snow"
	case code >= 80 && code <= 82:
		return "rain showers"
	case code >= 85 && code <= 86:
		return "snow showers"
	case code == 95:
		return "thunderstorm"
	case code == 96 || code == 99:
		return "thunderstorm with hail"
	default:
		return fmt.Sprintf("weather code %d", code)
	}
}
