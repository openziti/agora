package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultEnvironmentHeartbeatInterval       = 15 * time.Second
	defaultEnvironmentHeartbeatTTL            = 45 * time.Second
	defaultEnvironmentHeartbeatRequestTimeout = 10 * time.Second
)

type environmentHeartbeat struct {
	generation    uint64
	loopStartedAt *time.Time
	lastAttemptAt *time.Time
	lastSuccessAt *time.Time
	lastError     string
	permanent     bool
	cancel        context.CancelFunc
	done          chan struct{}
}

func (a *Runtime) replaceEnvironmentHeartbeatLoopLocked() {
	a.heartbeat.generation++
	generation := a.heartbeat.generation

	if a.heartbeat.cancel != nil {
		a.heartbeat.cancel()
		a.heartbeat.cancel = nil
		a.heartbeat.done = nil
	}

	if a.env == nil {
		a.resetEnvironmentHeartbeatLocked()
		return
	}

	startedAt := a.now().UTC()
	a.heartbeat.loopStartedAt = &startedAt
	a.heartbeat.lastAttemptAt = nil
	a.heartbeat.lastSuccessAt = nil
	a.heartbeat.lastError = ""
	a.heartbeat.permanent = false

	env := cloneEnvironment(a.env)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a.heartbeat.cancel = cancel
	a.heartbeat.done = done

	go a.runEnvironmentHeartbeatLoop(ctx, done, env, generation)
}

func (a *Runtime) stopEnvironmentHeartbeatLoop() {
	a.mu.Lock()
	a.heartbeat.generation++
	cancel := a.heartbeat.cancel
	done := a.heartbeat.done
	a.heartbeat.cancel = nil
	a.heartbeat.done = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (a *Runtime) resetEnvironmentHeartbeatLocked() {
	a.heartbeat.loopStartedAt = nil
	a.heartbeat.lastAttemptAt = nil
	a.heartbeat.lastSuccessAt = nil
	a.heartbeat.lastError = ""
	a.heartbeat.permanent = false
}

func (a *Runtime) runEnvironmentHeartbeatLoop(ctx context.Context, done chan struct{}, env *env_core.Environment, generation uint64) {
	defer close(done)

	dl.Infof("agora network environment heartbeat started environment_id='%s'", env.EnvironmentID)
	defer dl.Infof("agora network environment heartbeat stopped environment_id='%s'", env.EnvironmentID)

	if stop := a.performEnvironmentHeartbeat(ctx, env, generation); stop {
		return
	}

	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if stop := a.performEnvironmentHeartbeat(ctx, env, generation); stop {
				return
			}
		}
	}
}

func (a *Runtime) performEnvironmentHeartbeat(ctx context.Context, env *env_core.Environment, generation uint64) bool {
	attemptAt := a.now().UTC()
	permanent, err := a.heartbeatSender(ctx, env)
	if err != nil {
		if ctx.Err() != nil {
			return true
		}
		if transition := a.recordEnvironmentHeartbeatFailure(attemptAt, err.Error(), permanent, generation); transition || permanent {
			if permanent {
				dl.Warnf("agora network environment heartbeat failed permanently environment_id='%s': %v", env.EnvironmentID, err)
			} else {
				dl.Warnf("agora network environment heartbeat failed environment_id='%s': %v", env.EnvironmentID, err)
			}
		}
		return permanent
	}

	if transition := a.recordEnvironmentHeartbeatSuccess(attemptAt, generation); transition {
		dl.Debugf("agora network environment heartbeat online environment_id='%s'", env.EnvironmentID)
	}
	return false
}

func (a *Runtime) sendEnvironmentHeartbeat(parent context.Context, env *env_core.Environment) (bool, error) {
	if env.EnvironmentID == "" {
		return true, fmt.Errorf("enabled environment is missing environment id")
	}
	if env.APIEndpoint == "" {
		return true, fmt.Errorf("enabled environment is missing api endpoint")
	}
	if env.AccountToken == "" {
		return true, fmt.Errorf("enabled environment is missing account token")
	}

	client, err := newAccountAPIClient(env.APIEndpoint, env.AccountToken)
	if err != nil {
		return true, fmt.Errorf("create environment heartbeat client: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, a.heartbeatRequestTimeout)
	defer cancel()

	res, err := client.HeartbeatEnvironment(ctx, api.HeartbeatEnvironmentParams{EnvironmentId: env.EnvironmentID})
	if err != nil {
		return false, fmt.Errorf("send environment heartbeat: %w", err)
	}

	switch resp := res.(type) {
	case *api.HeartbeatEnvironmentNoContent:
		return false, nil
	case *api.HeartbeatEnvironmentUnauthorized:
		return true, fmt.Errorf("environment heartbeat unauthorized")
	case *api.HeartbeatEnvironmentNotFound:
		return true, fmt.Errorf("environment heartbeat not found")
	case *api.HeartbeatEnvironmentInternalServerError:
		return false, fmt.Errorf("environment heartbeat internal error: %s", resp.Message)
	default:
		return false, fmt.Errorf("environment heartbeat unexpected response: %T", res)
	}
}

func (a *Runtime) recordEnvironmentHeartbeatSuccess(attemptAt time.Time, generation uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if generation != a.heartbeat.generation {
		return false
	}
	transition := a.heartbeat.lastSuccessAt == nil || a.heartbeat.permanent || a.heartbeat.lastError != ""
	a.heartbeat.lastAttemptAt = &attemptAt
	a.heartbeat.lastSuccessAt = &attemptAt
	a.heartbeat.lastError = ""
	a.heartbeat.permanent = false
	return transition
}

func (a *Runtime) recordEnvironmentHeartbeatFailure(attemptAt time.Time, lastError string, permanent bool, generation uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if generation != a.heartbeat.generation {
		return false
	}
	prev := a.environmentHeartbeatStateLocked(attemptAt)
	prevError := a.heartbeat.lastError
	a.heartbeat.lastAttemptAt = &attemptAt
	a.heartbeat.lastError = lastError
	a.heartbeat.permanent = permanent
	return a.environmentHeartbeatStateLocked(attemptAt) != prev || prevError != lastError
}

func (a *Runtime) environmentHeartbeatStatusLocked(now time.Time) *networkpb.EnvironmentHeartbeatStatus {
	status := &networkpb.EnvironmentHeartbeatStatus{
		State: a.environmentHeartbeatStateLocked(now),
	}
	if a.heartbeat.lastAttemptAt != nil {
		status.LastAttemptAt = timestamppb.New(a.heartbeat.lastAttemptAt.UTC())
	}
	if a.heartbeat.lastSuccessAt != nil {
		status.LastSuccessAt = timestamppb.New(a.heartbeat.lastSuccessAt.UTC())
	}
	if a.heartbeat.lastError != "" {
		status.LastError = a.heartbeat.lastError
	}
	return status
}

func (a *Runtime) environmentHeartbeatStateLocked(now time.Time) networkpb.EnvironmentHeartbeatState {
	if a.env == nil {
		return networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_DISABLED
	}
	if a.heartbeat.permanent {
		return networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_ERROR
	}
	if a.heartbeat.lastSuccessAt != nil {
		if now.Sub(*a.heartbeat.lastSuccessAt) <= a.heartbeatTTL {
			return networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_ONLINE
		}
		return networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_STALE
	}
	if a.heartbeat.loopStartedAt == nil {
		return networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_STARTING
	}
	if now.Sub(*a.heartbeat.loopStartedAt) <= a.heartbeatTTL {
		return networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_STARTING
	}
	return networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_STALE
}
