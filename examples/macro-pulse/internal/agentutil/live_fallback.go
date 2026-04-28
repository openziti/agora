package agentutil

import (
	"context"

	"github.com/openziti/agora/examples/macro-pulse/internal/live"
	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
	"github.com/openziti/agora/sdk/agent"
)

// WithLiveFallback wraps a snapshot handler with an optional live
// path: when `live` is true, the live fetch is attempted first; on
// any error the snapshot handler is called and the error is logged at
// WARN level. When `live` is false the snapshot handler is returned
// as-is.
func WithLiveFallback[Req any, Resp any](
	a *agent.Agent,
	live bool,
	liveFn func(context.Context, Req) (Resp, error),
	snapshotFn func(context.Context, Req) (Resp, error),
) func(context.Context, Req) (Resp, error) {
	if !live {
		return snapshotFn
	}
	return func(ctx context.Context, req Req) (Resp, error) {
		resp, err := liveFn(ctx, req)
		if err == nil {
			a.Log().Infof("live fetch succeeded; serving live data")
			return resp, nil
		}
		a.Log().Warnf("live fetch failed: %v; falling back to snapshot", err)
		return snapshotFn(ctx, req)
	}
}

// EquityHandleFor returns the snapshot or live+fallback equity handler.
func EquityHandleFor(a *agent.Agent, useLive bool) func(context.Context, payloads.EquityRequest) (payloads.EquityResponse, error) {
	return WithLiveFallback(a, useLive, live.FetchEquity, EquityHandle)
}

func FXHandleFor(a *agent.Agent, useLive bool) func(context.Context, payloads.FXRequest) (payloads.FXResponse, error) {
	return WithLiveFallback(a, useLive, live.FetchFX, FXHandle)
}

func CommoditiesHandleFor(a *agent.Agent, useLive bool) func(context.Context, payloads.CommoditiesRequest) (payloads.CommoditiesResponse, error) {
	return WithLiveFallback(a, useLive, live.FetchCommodities, CommoditiesHandle)
}

func WeatherCurrentHandleFor(a *agent.Agent, useLive bool) func(context.Context, payloads.WeatherCurrentRequest) (payloads.WeatherCurrentResponse, error) {
	return WithLiveFallback(a, useLive, live.FetchWeatherCurrent, WeatherCurrentHandle)
}

func WeatherForecastHandleFor(a *agent.Agent, useLive bool) func(context.Context, payloads.WeatherForecastRequest) (payloads.WeatherForecastResponse, error) {
	return WithLiveFallback(a, useLive, live.FetchWeatherForecast, WeatherForecastHandle)
}

func SearchHandleFor(a *agent.Agent, useLive bool) func(context.Context, payloads.SearchRequest) (payloads.SearchResponse, error) {
	return WithLiveFallback(a, useLive, live.FetchSearchTrends, SearchHandle)
}

func NewsHandleFor(a *agent.Agent, useLive bool) func(context.Context, payloads.NewsRequest) (payloads.NewsResponse, error) {
	return WithLiveFallback(a, useLive, live.FetchNews, NewsHandle)
}
