package agentutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/openziti/agora/examples/macro-pulse/internal/payloads"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
)

// WeatherRouterHandler is the weather-feed session handler that
// dispatches by inbound `message_type` between the `weather.current`
// and `weather.forecast` capabilities, since both are served by a
// single advertisement. When Live is true the handler attempts each
// upstream call first and falls back to snapshots on any error.
type WeatherRouterHandler struct {
	Agent *agent.Agent
	Live  bool
}

func (h *WeatherRouterHandler) Accept(_ context.Context, proposal session.Proposal) error {
	h.Agent.Log().
		With("session_id", proposal.SessionID).
		With("consumer_account", proposal.ConsumerAccount).
		With("workgroup_id", proposal.WorkgroupID).
		Infof("accepting weather session proposal")
	return nil
}

func (h *WeatherRouterHandler) Serve(ctx context.Context, sess *session.Session) error {
	in, err := sess.Receive(ctx)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	var (
		respMsg     string
		respPayload []byte
	)
	switch in.MessageType {
	case "weather.current.request":
		var req payloads.WeatherCurrentRequest
		if len(in.Payload) > 0 {
			if err := json.Unmarshal(in.Payload, &req); err != nil {
				return fmt.Errorf("decode current request: %w", err)
			}
		}
		resp, err := WeatherCurrentHandleFor(h.Agent, h.Live)(ctx, req)
		if err != nil {
			return err
		}
		respPayload, err = json.Marshal(resp)
		if err != nil {
			return err
		}
		respMsg = "weather.current.response"
	case "weather.forecast.request":
		var req payloads.WeatherForecastRequest
		if len(in.Payload) > 0 {
			if err := json.Unmarshal(in.Payload, &req); err != nil {
				return fmt.Errorf("decode forecast request: %w", err)
			}
		}
		resp, err := WeatherForecastHandleFor(h.Agent, h.Live)(ctx, req)
		if err != nil {
			return err
		}
		respPayload, err = json.Marshal(resp)
		if err != nil {
			return err
		}
		respMsg = "weather.forecast.response"
	default:
		return fmt.Errorf("weather router: unsupported message_type %q", in.MessageType)
	}

	out := session.Envelope{
		MessageType:   respMsg,
		ContentType:   "application/json",
		CorrelationID: in.EnvelopeID,
		Payload:       respPayload,
	}
	if err := sess.Send(ctx, out); err != nil {
		return err
	}
	h.Agent.Log().
		With("session_id", sess.ID).
		With("request_message_type", in.MessageType).
		With("response_message_type", respMsg).
		With("response_bytes", len(respPayload)).
		Infof("served weather request")
	_, _ = sess.Receive(ctx)
	return nil
}
