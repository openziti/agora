package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/sdk/agent/networkpb"
)

const (
	tunnelServeHeartbeatInterval      = 15 * time.Second
	tunnelAttachmentHeartbeatInterval = 15 * time.Second

	defaultTunnelRetryInitialDelay = time.Second
	defaultTunnelRetryMaxDelay     = 30 * time.Second
	defaultTunnelRetryMultiplier   = 2.0
	defaultTunnelRetryJitter       = 0.2

	tunnelHeartbeatFailureThreshold = 3
)

var errStaleManagedAttempt = errors.New("stale managed tunnel attempt")

type serveAttempt struct {
	generation   uint64
	env          *env_core.Environment
	desired      env_core.ManagedServe
	identityPath string
}

type connectAttempt struct {
	generation   uint64
	env          *env_core.Environment
	desired      env_core.ManagedConnect
	identityPath string
}

func (a *Runtime) EnsureServe(ctx context.Context, req *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error) {
	desired, err := validateDesiredServe(req)
	if err != nil {
		return nil, err
	}

	actor, oldServe, err := a.prepareServeEnsure(desired)
	if err != nil {
		return nil, err
	}
	if oldServe != nil {
		a.stopServeActor(oldServe)
	}

	if err := a.runServeAttempt(ctx, actor, 0, true); err != nil {
		return nil, err
	}
	return &networkpb.EnsureServeResponse{Serve: a.snapshotServeStatus(actor)}, nil
}

func (a *Runtime) RemoveServe(_ context.Context, req *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error) {
	actor, err := a.removeServeActor(req.ServeId, req.TunnelId, req.Name)
	if err != nil {
		return nil, err
	}
	a.stopServeActor(actor)
	return &networkpb.RemoveServeResponse{}, nil
}

func (a *Runtime) EnsureConnect(ctx context.Context, req *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error) {
	desired, err := validateDesiredConnect(req)
	if err != nil {
		return nil, err
	}

	actor, oldConnect, err := a.prepareConnectEnsure(desired)
	if err != nil {
		return nil, err
	}
	if oldConnect != nil {
		a.stopConnectActor(oldConnect)
	}

	if err := a.runConnectAttempt(ctx, actor, 0, true); err != nil {
		return nil, err
	}
	return &networkpb.EnsureConnectResponse{Connect: a.snapshotConnectStatus(actor)}, nil
}

func (a *Runtime) RemoveConnect(_ context.Context, req *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error) {
	actor, err := a.removeConnectActor(req.AttachmentId, req.TunnelId, req.Name, req.ListenAddress)
	if err != nil {
		return nil, err
	}
	a.stopConnectActor(actor)
	return &networkpb.RemoveConnectResponse{}, nil
}

func (a *Runtime) startConfiguredActorsLocked() {
	serves := make([]*managedServe, 0, len(a.serves))
	connects := make([]*managedConnect, 0, len(a.connects))
	for _, actor := range a.serves {
		serves = append(serves, actor)
	}
	for _, actor := range a.connects {
		connects = append(connects, actor)
	}

	for _, actor := range serves {
		go func(actor *managedServe) {
			_ = a.runServeAttempt(context.Background(), actor, 0, true)
		}(actor)
	}
	for _, actor := range connects {
		go func(actor *managedConnect) {
			_ = a.runConnectAttempt(context.Background(), actor, 0, true)
		}(actor)
	}
}

func (a *Runtime) prepareServeEnsure(desired env_core.ManagedServe) (*managedServe, *managedServe, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.env == nil {
		return nil, nil, fmt.Errorf("no environment is enabled")
	}
	if a.identityPath == "" {
		return nil, nil, fmt.Errorf("no identity path configured")
	}

	upsertServe(a.network, desired)
	if err := a.persistNetwork(); err != nil {
		return nil, nil, err
	}

	actor := &managedServe{
		desired:  desired,
		state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
		tunnelID: desired.TunnelID,
	}
	key, old := a.findServeActorLocked(desired.TunnelID, desired.Name)
	if key != "" {
		delete(a.serves, key)
	}
	a.serves[serveKey(desired)] = actor

	return actor, old, nil
}

func (a *Runtime) prepareConnectEnsure(desired env_core.ManagedConnect) (*managedConnect, *managedConnect, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.env == nil {
		return nil, nil, fmt.Errorf("no environment is enabled")
	}
	if a.identityPath == "" {
		return nil, nil, fmt.Errorf("no identity path configured")
	}

	upsertConnect(a.network, desired)
	if err := a.persistNetwork(); err != nil {
		return nil, nil, err
	}

	actor := &managedConnect{
		desired:  desired,
		state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
		tunnelID: desired.TunnelID,
	}
	key, old := a.findConnectActorLocked(desired.TunnelID, desired.Name, desired.ListenAddress)
	if key != "" {
		delete(a.connects, key)
	}
	a.connects[connectKey(desired)] = actor

	return actor, old, nil
}

func (a *Runtime) runServeAttempt(ctx context.Context, actor *managedServe, expectedGeneration uint64, scheduleRetry bool) error {
	attempt, err := a.prepareServeAttempt(actor, expectedGeneration)
	if err != nil {
		if errors.Is(err, errStaleManagedAttempt) {
			return nil
		}
		return err
	}

	tunnel, serve, err := a.controller.StartServe(ctx, attempt.env, attempt.desired)
	if err != nil {
		a.recordServeAttemptFailure(actor, attempt.generation, err, scheduleRetry)
		return err
	}

	desired := attempt.desired
	desired.TunnelID = tunnel.ID
	if err := a.updateServeDesired(actor, attempt.generation, desired); err != nil {
		_ = a.controller.StopServe(context.Background(), attempt.env, serve.ID)
		a.recordServeAttemptFailure(actor, attempt.generation, err, scheduleRetry)
		return err
	}

	actorCtx, cancel := context.WithCancel(context.Background())
	handle, err := a.runtimeHost.StartServe(actorCtx, attempt.identityPath, tunnel)
	if err != nil {
		cancel()
		_ = a.controller.StopServe(context.Background(), attempt.env, serve.ID)
		a.recordServeAttemptFailure(actor, attempt.generation, err, scheduleRetry)
		return err
	}

	startedAt := a.now().UTC()
	if !a.markServeRunning(actor, attempt.generation, attempt.env, tunnel.ID, serve.ID, startedAt, cancel) {
		cancel()
		_ = a.controller.StopServe(context.Background(), attempt.env, serve.ID)
		return nil
	}

	go a.runServeHeartbeatLoop(actorCtx, actor, serve.ID, attempt.generation)
	go a.watchServeActor(actor, handle.Done(), attempt.generation)
	return nil
}

func (a *Runtime) runConnectAttempt(ctx context.Context, actor *managedConnect, expectedGeneration uint64, scheduleRetry bool) error {
	attempt, err := a.prepareConnectAttempt(actor, expectedGeneration)
	if err != nil {
		if errors.Is(err, errStaleManagedAttempt) {
			return nil
		}
		return err
	}

	tunnel, attachment, err := a.controller.StartConnect(ctx, attempt.env, attempt.desired)
	if err != nil {
		a.recordConnectAttemptFailure(actor, attempt.generation, err, scheduleRetry)
		return err
	}

	desired := attempt.desired
	desired.TunnelID = tunnel.ID
	if err := a.updateConnectDesired(actor, attempt.generation, desired); err != nil {
		_ = a.controller.StopAttachment(context.Background(), attempt.env, attachment.ID)
		a.recordConnectAttemptFailure(actor, attempt.generation, err, scheduleRetry)
		return err
	}

	actorCtx, cancel := context.WithCancel(context.Background())
	handle, err := a.runtimeHost.StartConnect(actorCtx, attempt.identityPath, tunnel, attachment.ListenAddress)
	if err != nil {
		cancel()
		_ = a.controller.StopAttachment(context.Background(), attempt.env, attachment.ID)
		a.recordConnectAttemptFailure(actor, attempt.generation, err, scheduleRetry)
		return err
	}

	startedAt := a.now().UTC()
	if !a.markConnectRunning(actor, attempt.generation, attempt.env, tunnel.ID, attachment.ID, startedAt, cancel) {
		cancel()
		_ = a.controller.StopAttachment(context.Background(), attempt.env, attachment.ID)
		return nil
	}

	go a.runConnectHeartbeatLoop(actorCtx, actor, attachment.ID, attempt.generation)
	go a.watchConnectActor(actor, handle.Done(), attempt.generation)
	return nil
}

func (a *Runtime) prepareServeAttempt(actor *managedServe, expectedGeneration uint64) (*serveAttempt, error) {
	a.mu.Lock()
	if !a.hasServeActorLocked(actor) {
		a.mu.Unlock()
		return nil, errStaleManagedAttempt
	}
	if expectedGeneration != 0 && actor.generation != expectedGeneration {
		a.mu.Unlock()
		return nil, errStaleManagedAttempt
	}

	actor.generation++
	generation := actor.generation
	if actor.retryTimer != nil {
		actor.retryTimer.Stop()
		actor.retryTimer = nil
	}
	actor.nextRetryAt = nil
	oldCancel := actor.cancel
	oldEnv := cloneEnvironment(actor.env)
	oldServeID := actor.serveID
	actor.cancel = nil
	actor.env = nil
	actor.serveID = ""
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_STARTING
	actor.lastError = ""
	actor.transientHeartbeatFailures = 0
	env := cloneEnvironment(a.env)
	desired := actor.desired
	identityPath := a.identityPath
	a.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	if oldServeID != "" && oldEnv != nil {
		if err := a.controller.StopServe(context.Background(), oldEnv, oldServeID); err != nil {
			dl.Warnf("agora network failed to delete serve_id='%s': %v", oldServeID, err)
		}
	}

	if env == nil {
		err := fmt.Errorf("no environment is enabled")
		a.recordServePrerequisiteFailure(actor, generation, err)
		return nil, err
	}

	if identityPath == "" {
		err := fmt.Errorf("no identity path configured")
		a.recordServePrerequisiteFailure(actor, generation, err)
		return nil, err
	}

	return &serveAttempt{
		generation:   generation,
		env:          env,
		desired:      desired,
		identityPath: identityPath,
	}, nil
}

func (a *Runtime) prepareConnectAttempt(actor *managedConnect, expectedGeneration uint64) (*connectAttempt, error) {
	a.mu.Lock()
	if !a.hasConnectActorLocked(actor) {
		a.mu.Unlock()
		return nil, errStaleManagedAttempt
	}
	if expectedGeneration != 0 && actor.generation != expectedGeneration {
		a.mu.Unlock()
		return nil, errStaleManagedAttempt
	}

	actor.generation++
	generation := actor.generation
	if actor.retryTimer != nil {
		actor.retryTimer.Stop()
		actor.retryTimer = nil
	}
	actor.nextRetryAt = nil
	oldCancel := actor.cancel
	oldEnv := cloneEnvironment(actor.env)
	oldAttachmentID := actor.attachmentID
	actor.cancel = nil
	actor.env = nil
	actor.attachmentID = ""
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_STARTING
	actor.lastError = ""
	actor.transientHeartbeatFailures = 0
	env := cloneEnvironment(a.env)
	desired := actor.desired
	identityPath := a.identityPath
	a.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	if oldAttachmentID != "" && oldEnv != nil {
		if err := a.controller.StopAttachment(context.Background(), oldEnv, oldAttachmentID); err != nil {
			dl.Warnf("agora network failed to delete attachment_id='%s': %v", oldAttachmentID, err)
		}
	}

	if env == nil {
		err := fmt.Errorf("no environment is enabled")
		a.recordConnectPrerequisiteFailure(actor, generation, err)
		return nil, err
	}

	if identityPath == "" {
		err := fmt.Errorf("no identity path configured")
		a.recordConnectPrerequisiteFailure(actor, generation, err)
		return nil, err
	}

	return &connectAttempt{
		generation:   generation,
		env:          env,
		desired:      desired,
		identityPath: identityPath,
	}, nil
}

func (a *Runtime) updateServeDesired(actor *managedServe, generation uint64, desired env_core.ManagedServe) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasServeActorLocked(actor) || actor.generation != generation {
		return errStaleManagedAttempt
	}
	actor.desired = desired
	actor.tunnelID = desired.TunnelID
	upsertServe(a.network, desired)
	if err := a.persistNetwork(); err != nil {
		return err
	}
	a.rekeyServeActorLocked(actor)
	return nil
}

func (a *Runtime) updateConnectDesired(actor *managedConnect, generation uint64, desired env_core.ManagedConnect) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasConnectActorLocked(actor) || actor.generation != generation {
		return errStaleManagedAttempt
	}
	actor.desired = desired
	actor.tunnelID = desired.TunnelID
	upsertConnect(a.network, desired)
	if err := a.persistNetwork(); err != nil {
		return err
	}
	a.rekeyConnectActorLocked(actor)
	return nil
}

func (a *Runtime) recordServePrerequisiteFailure(actor *managedServe, generation uint64, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasServeActorLocked(actor) || actor.generation != generation {
		return
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.lastError = err.Error()
	actor.retryAttempt = 0
	actor.nextRetryAt = nil
}

func (a *Runtime) recordConnectPrerequisiteFailure(actor *managedConnect, generation uint64, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasConnectActorLocked(actor) || actor.generation != generation {
		return
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.lastError = err.Error()
	actor.retryAttempt = 0
	actor.nextRetryAt = nil
}

func (a *Runtime) recordServeAttemptFailure(actor *managedServe, generation uint64, err error, scheduleRetry bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasServeActorLocked(actor) || actor.generation != generation {
		return
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.lastError = err.Error()
	actor.cancel = nil
	actor.serveID = ""
	actor.env = nil
	actor.transientHeartbeatFailures = 0
	if !scheduleRetry {
		actor.nextRetryAt = nil
		return
	}
	a.scheduleServeRetryLocked(actor)
}

func (a *Runtime) recordConnectAttemptFailure(actor *managedConnect, generation uint64, err error, scheduleRetry bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasConnectActorLocked(actor) || actor.generation != generation {
		return
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.lastError = err.Error()
	actor.cancel = nil
	actor.attachmentID = ""
	actor.env = nil
	actor.transientHeartbeatFailures = 0
	if !scheduleRetry {
		actor.nextRetryAt = nil
		return
	}
	a.scheduleConnectRetryLocked(actor)
}

func (a *Runtime) markServeRunning(actor *managedServe, generation uint64, env *env_core.Environment, tunnelID, serveID string, startedAt time.Time, cancel context.CancelFunc) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasServeActorLocked(actor) || actor.generation != generation {
		return false
	}
	actor.env = cloneEnvironment(env)
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_RUNNING
	actor.tunnelID = tunnelID
	actor.serveID = serveID
	actor.lastError = ""
	actor.lastStartedAt = &startedAt
	actor.nextRetryAt = nil
	actor.retryAttempt = 0
	actor.transientHeartbeatFailures = 0
	actor.cancel = cancel
	return true
}

func (a *Runtime) markConnectRunning(actor *managedConnect, generation uint64, env *env_core.Environment, tunnelID, attachmentID string, startedAt time.Time, cancel context.CancelFunc) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasConnectActorLocked(actor) || actor.generation != generation {
		return false
	}
	actor.env = cloneEnvironment(env)
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_RUNNING
	actor.tunnelID = tunnelID
	actor.attachmentID = attachmentID
	actor.lastError = ""
	actor.lastStartedAt = &startedAt
	actor.nextRetryAt = nil
	actor.retryAttempt = 0
	actor.transientHeartbeatFailures = 0
	actor.cancel = cancel
	return true
}

func (a *Runtime) watchServeActor(actor *managedServe, done <-chan error, generation uint64) {
	err := <-done

	a.mu.Lock()
	if !a.hasServeActorLocked(actor) || actor.generation != generation {
		a.mu.Unlock()
		return
	}
	cancel := actor.cancel
	env := cloneEnvironment(actor.env)
	serveID := actor.serveID
	actor.cancel = nil
	actor.env = nil
	actor.serveID = ""
	if err != nil {
		actor.lastError = err.Error()
	} else {
		actor.lastError = "serve runtime exited"
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.transientHeartbeatFailures = 0
	a.scheduleServeRetryLocked(actor)
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if serveID != "" && env != nil {
		if stopErr := a.controller.StopServe(context.Background(), env, serveID); stopErr != nil {
			dl.Warnf("agora network serve cleanup failed serve_id='%s': %v", serveID, stopErr)
		}
	}
}

func (a *Runtime) watchConnectActor(actor *managedConnect, done <-chan error, generation uint64) {
	err := <-done

	a.mu.Lock()
	if !a.hasConnectActorLocked(actor) || actor.generation != generation {
		a.mu.Unlock()
		return
	}
	cancel := actor.cancel
	env := cloneEnvironment(actor.env)
	attachmentID := actor.attachmentID
	actor.cancel = nil
	actor.env = nil
	actor.attachmentID = ""
	if err != nil {
		actor.lastError = err.Error()
	} else {
		actor.lastError = "connect runtime exited"
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.transientHeartbeatFailures = 0
	a.scheduleConnectRetryLocked(actor)
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if attachmentID != "" && env != nil {
		if stopErr := a.controller.StopAttachment(context.Background(), env, attachmentID); stopErr != nil {
			dl.Warnf("agora network attachment cleanup failed attachment_id='%s': %v", attachmentID, stopErr)
		}
	}
}

func (a *Runtime) stopServeActor(actor *managedServe) {
	if actor == nil {
		return
	}

	a.mu.Lock()
	actor.generation++
	if actor.retryTimer != nil {
		actor.retryTimer.Stop()
		actor.retryTimer = nil
	}
	actor.nextRetryAt = nil
	cancel := actor.cancel
	env := cloneEnvironment(actor.env)
	serveID := actor.serveID
	actor.cancel = nil
	actor.env = nil
	actor.serveID = ""
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if serveID != "" && env != nil {
		if err := a.controller.StopServe(context.Background(), env, serveID); err != nil {
			dl.Warnf("agora network failed to delete serve_id='%s': %v", serveID, err)
		}
	}
}

func (a *Runtime) stopConnectActor(actor *managedConnect) {
	if actor == nil {
		return
	}

	a.mu.Lock()
	actor.generation++
	if actor.retryTimer != nil {
		actor.retryTimer.Stop()
		actor.retryTimer = nil
	}
	actor.nextRetryAt = nil
	cancel := actor.cancel
	env := cloneEnvironment(actor.env)
	attachmentID := actor.attachmentID
	actor.cancel = nil
	actor.env = nil
	actor.attachmentID = ""
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if attachmentID != "" && env != nil {
		if err := a.controller.StopAttachment(context.Background(), env, attachmentID); err != nil {
			dl.Warnf("agora network failed to delete attachment_id='%s': %v", attachmentID, err)
		}
	}
}

func (a *Runtime) runServeHeartbeatLoop(ctx context.Context, actor *managedServe, serveID string, generation uint64) {
	ticker := time.NewTicker(a.serveHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.controller.HeartbeatServe(context.Background(), actor.env, serveID); err != nil {
				if isCurrentRuntimeControllerError(err) {
					dl.Warnf("agora network serve heartbeat failed permanently serve_id='%s': %v", serveID, err)
					go func() {
						_ = a.runServeAttempt(context.Background(), actor, generation, true)
					}()
					return
				}
				if thresholdReached := a.incrementServeHeartbeatFailures(actor, generation); thresholdReached {
					dl.Warnf("agora network serve heartbeat failed repeatedly serve_id='%s': %v", serveID, err)
					go func() {
						_ = a.runServeAttempt(context.Background(), actor, generation, true)
					}()
					return
				}
				dl.Warnf("agora network serve heartbeat failed serve_id='%s': %v", serveID, err)
				continue
			}
			a.resetServeHeartbeatFailures(actor, generation)
		}
	}
}

func (a *Runtime) runConnectHeartbeatLoop(ctx context.Context, actor *managedConnect, attachmentID string, generation uint64) {
	ticker := time.NewTicker(a.connectHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.controller.HeartbeatAttachment(context.Background(), actor.env, attachmentID); err != nil {
				if isCurrentRuntimeControllerError(err) {
					dl.Warnf("agora network attachment heartbeat failed permanently attachment_id='%s': %v", attachmentID, err)
					go func() {
						_ = a.runConnectAttempt(context.Background(), actor, generation, true)
					}()
					return
				}
				if thresholdReached := a.incrementConnectHeartbeatFailures(actor, generation); thresholdReached {
					dl.Warnf("agora network attachment heartbeat failed repeatedly attachment_id='%s': %v", attachmentID, err)
					go func() {
						_ = a.runConnectAttempt(context.Background(), actor, generation, true)
					}()
					return
				}
				dl.Warnf("agora network attachment heartbeat failed attachment_id='%s': %v", attachmentID, err)
				continue
			}
			a.resetConnectHeartbeatFailures(actor, generation)
		}
	}
}

func (a *Runtime) incrementServeHeartbeatFailures(actor *managedServe, generation uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasServeActorLocked(actor) || actor.generation != generation {
		return false
	}
	actor.transientHeartbeatFailures++
	return actor.transientHeartbeatFailures >= tunnelHeartbeatFailureThreshold
}

func (a *Runtime) incrementConnectHeartbeatFailures(actor *managedConnect, generation uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasConnectActorLocked(actor) || actor.generation != generation {
		return false
	}
	actor.transientHeartbeatFailures++
	return actor.transientHeartbeatFailures >= tunnelHeartbeatFailureThreshold
}

func (a *Runtime) resetServeHeartbeatFailures(actor *managedServe, generation uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasServeActorLocked(actor) || actor.generation != generation {
		return
	}
	actor.transientHeartbeatFailures = 0
}

func (a *Runtime) resetConnectHeartbeatFailures(actor *managedConnect, generation uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasConnectActorLocked(actor) || actor.generation != generation {
		return
	}
	actor.transientHeartbeatFailures = 0
}

func (a *Runtime) scheduleServeRetryLocked(actor *managedServe) {
	actor.retryAttempt++
	delay := a.retryDelay(actor.retryAttempt)
	nextRetryAt := a.now().UTC().Add(delay)
	actor.nextRetryAt = &nextRetryAt
	generation := actor.generation
	actor.retryTimer = time.AfterFunc(delay, func() {
		_ = a.runServeAttempt(context.Background(), actor, generation, true)
	})
}

func (a *Runtime) scheduleConnectRetryLocked(actor *managedConnect) {
	actor.retryAttempt++
	delay := a.retryDelay(actor.retryAttempt)
	nextRetryAt := a.now().UTC().Add(delay)
	actor.nextRetryAt = &nextRetryAt
	generation := actor.generation
	actor.retryTimer = time.AfterFunc(delay, func() {
		_ = a.runConnectAttempt(context.Background(), actor, generation, true)
	})
}

func (a *Runtime) retryDelay(attempt uint32) time.Duration {
	if attempt == 0 {
		return 0
	}

	delay := float64(a.retryInitialDelay)
	if attempt > 1 {
		delay = delay * math.Pow(a.retryMultiplier, float64(attempt-1))
	}
	if maxDelay := float64(a.retryMaxDelay); maxDelay > 0 && delay > maxDelay {
		delay = maxDelay
	}
	if a.retryJitter > 0 {
		jitter := 1 + ((a.retryRand()*2)-1)*a.retryJitter
		delay = delay * jitter
	}
	if delay < 0 {
		delay = 0
	}
	return time.Duration(delay)
}

func (a *Runtime) removeServeActor(serveID, tunnelID, name string) (*managedServe, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var key string
	var actor *managedServe
	if serveID != "" {
		key, actor = a.findServeByServeIDLocked(serveID)
	} else {
		key, actor = a.findServeActorLocked(tunnelID, name)
	}
	if actor == nil {
		return nil, fmt.Errorf("managed serve not found")
	}
	delete(a.serves, key)
	removeServe(a.network, actor.desired.TunnelID, actor.desired.Name)
	if err := a.persistNetwork(); err != nil {
		a.serves[key] = actor
		return nil, err
	}
	return actor, nil
}

func (a *Runtime) removeConnectActor(attachmentID, tunnelID, name, listenAddress string) (*managedConnect, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var key string
	var actor *managedConnect
	if attachmentID != "" {
		key, actor = a.findConnectByAttachmentIDLocked(attachmentID)
	} else {
		key, actor = a.findConnectActorLocked(tunnelID, name, listenAddress)
	}
	if actor == nil {
		return nil, fmt.Errorf("managed connect not found")
	}
	delete(a.connects, key)
	removeConnect(a.network, actor.desired.TunnelID, actor.desired.Name, actor.desired.ListenAddress)
	if err := a.persistNetwork(); err != nil {
		a.connects[key] = actor
		return nil, err
	}
	return actor, nil
}

func (a *Runtime) shutdownManagedActors() {
	a.mu.Lock()
	serves := make([]*managedServe, 0, len(a.serves))
	connects := make([]*managedConnect, 0, len(a.connects))
	for _, actor := range a.serves {
		serves = append(serves, actor)
	}
	for _, actor := range a.connects {
		connects = append(connects, actor)
	}
	a.serves = configuredServes(a.network)
	a.connects = configuredConnects(a.network)
	a.mu.Unlock()

	for _, actor := range serves {
		a.stopServeActor(actor)
	}
	for _, actor := range connects {
		a.stopConnectActor(actor)
	}
}

func (a *Runtime) snapshotServeStatus(actor *managedServe) *networkpb.ManagedServeStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return serveStatusProto(actor)
}

func (a *Runtime) snapshotConnectStatus(actor *managedConnect) *networkpb.ManagedConnectStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return connectStatusProto(actor)
}

func (a *Runtime) hasServeActorLocked(actor *managedServe) bool {
	for _, current := range a.serves {
		if current == actor {
			return true
		}
	}
	return false
}

func (a *Runtime) hasConnectActorLocked(actor *managedConnect) bool {
	for _, current := range a.connects {
		if current == actor {
			return true
		}
	}
	return false
}

func (a *Runtime) findServeActorLocked(tunnelID, name string) (string, *managedServe) {
	for key, actor := range a.serves {
		if matchesServe(actor.desired, tunnelID, name) {
			return key, actor
		}
	}
	return "", nil
}

func (a *Runtime) findConnectActorLocked(tunnelID, name, listenAddress string) (string, *managedConnect) {
	for key, actor := range a.connects {
		if matchesConnect(actor.desired, tunnelID, name, listenAddress) {
			return key, actor
		}
	}
	return "", nil
}

func (a *Runtime) findServeByServeIDLocked(serveID string) (string, *managedServe) {
	for key, actor := range a.serves {
		if actor.serveID == serveID {
			return key, actor
		}
	}
	return "", nil
}

func (a *Runtime) findConnectByAttachmentIDLocked(attachmentID string) (string, *managedConnect) {
	for key, actor := range a.connects {
		if actor.attachmentID == attachmentID {
			return key, actor
		}
	}
	return "", nil
}

func (a *Runtime) rekeyServeActorLocked(actor *managedServe) {
	for key, current := range a.serves {
		if current != actor {
			continue
		}
		desiredKey := serveKey(actor.desired)
		if key == desiredKey {
			return
		}
		delete(a.serves, key)
		a.serves[desiredKey] = actor
		return
	}
}

func (a *Runtime) rekeyConnectActorLocked(actor *managedConnect) {
	for key, current := range a.connects {
		if current != actor {
			continue
		}
		desiredKey := connectKey(actor.desired)
		if key == desiredKey {
			return
		}
		delete(a.connects, key)
		a.connects[desiredKey] = actor
		return
	}
}

func serveKey(desired env_core.ManagedServe) string {
	if desired.TunnelID != "" {
		return desired.TunnelID
	}
	return strings.ToLower(strings.TrimSpace(desired.Name))
}

func connectKey(desired env_core.ManagedConnect) string {
	if desired.TunnelID != "" {
		return desired.TunnelID + "|" + desired.ListenAddress
	}
	return strings.ToLower(strings.TrimSpace(desired.Name)) + "|" + desired.ListenAddress
}
