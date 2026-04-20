package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/environment/env_core"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
)

const (
	tunnelServeHeartbeatInterval      = 15 * time.Second
	tunnelAttachmentHeartbeatInterval = 15 * time.Second
)

func (a *Agent) EnsureServe(ctx context.Context, req *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error) {
	desired, err := validateDesiredServe(req)
	if err != nil {
		return nil, err
	}

	root, env, actor, oldServe, err := a.prepareServeEnsure(desired)
	if err != nil {
		return nil, err
	}
	if oldServe != nil {
		a.stopServeActor(oldServe)
	}

	identityPath, err := root.ZitiIdentityNamed(environmentIdentityName)
	if err != nil {
		a.failServeActor(actor, err)
		return nil, err
	}

	tunnel, serve, err := a.controller.StartServe(ctx, env, desired)
	if err != nil {
		a.failServeActor(actor, err)
		return nil, err
	}

	desired.TunnelID = tunnel.ID
	if err := a.updateServeDesired(actor, desired); err != nil {
		_ = a.controller.StopServe(context.Background(), env, serve.ID)
		a.failServeActor(actor, err)
		return nil, err
	}

	actorCtx, cancel := context.WithCancel(context.Background())
	handle, err := a.runtimeHost.StartServe(actorCtx, identityPath, tunnel)
	if err != nil {
		cancel()
		_ = a.controller.StopServe(context.Background(), env, serve.ID)
		a.failServeActor(actor, err)
		return nil, err
	}

	startedAt := a.now().UTC()
	if !a.markServeRunning(actor, env, tunnel.ID, serve.ID, startedAt, cancel) {
		cancel()
		_ = a.controller.StopServe(context.Background(), env, serve.ID)
		return nil, fmt.Errorf("managed serve start was canceled")
	}

	go a.runServeHeartbeatLoop(actorCtx, actor, serve.ID)
	go a.watchServeActor(actor, handle.Done())

	return &networkpb.EnsureServeResponse{Serve: a.snapshotServeStatus(actor)}, nil
}

func (a *Agent) RemoveServe(_ context.Context, req *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error) {
	actor, err := a.removeServeActor(req.ServeId, req.TunnelId, req.Name)
	if err != nil {
		return nil, err
	}
	a.stopServeActor(actor)
	return &networkpb.RemoveServeResponse{}, nil
}

func (a *Agent) EnsureConnect(ctx context.Context, req *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error) {
	desired, err := validateDesiredConnect(req)
	if err != nil {
		return nil, err
	}

	root, env, actor, oldConnect, err := a.prepareConnectEnsure(desired)
	if err != nil {
		return nil, err
	}
	if oldConnect != nil {
		a.stopConnectActor(oldConnect)
	}

	identityPath, err := root.ZitiIdentityNamed(environmentIdentityName)
	if err != nil {
		a.failConnectActor(actor, err)
		return nil, err
	}

	tunnel, attachment, err := a.controller.StartConnect(ctx, env, desired)
	if err != nil {
		a.failConnectActor(actor, err)
		return nil, err
	}

	desired.TunnelID = tunnel.ID
	if err := a.updateConnectDesired(actor, desired); err != nil {
		_ = a.controller.StopAttachment(context.Background(), env, attachment.ID)
		a.failConnectActor(actor, err)
		return nil, err
	}

	actorCtx, cancel := context.WithCancel(context.Background())
	handle, err := a.runtimeHost.StartConnect(actorCtx, identityPath, tunnel, attachment.ListenAddress)
	if err != nil {
		cancel()
		_ = a.controller.StopAttachment(context.Background(), env, attachment.ID)
		a.failConnectActor(actor, err)
		return nil, err
	}

	startedAt := a.now().UTC()
	if !a.markConnectRunning(actor, env, tunnel.ID, attachment.ID, startedAt, cancel) {
		cancel()
		_ = a.controller.StopAttachment(context.Background(), env, attachment.ID)
		return nil, fmt.Errorf("managed connect start was canceled")
	}

	go a.runConnectHeartbeatLoop(actorCtx, actor, attachment.ID)
	go a.watchConnectActor(actor, handle.Done())

	return &networkpb.EnsureConnectResponse{Connect: a.snapshotConnectStatus(actor)}, nil
}

func (a *Agent) RemoveConnect(_ context.Context, req *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error) {
	actor, err := a.removeConnectActor(req.AttachmentId, req.TunnelId, req.Name, req.ListenAddress)
	if err != nil {
		return nil, err
	}
	a.stopConnectActor(actor)
	return &networkpb.RemoveConnectResponse{}, nil
}

func (a *Agent) prepareServeEnsure(desired env_core.ManagedServe) (env_core.Root, *env_core.Environment, *managedServe, *managedServe, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.env == nil {
		return nil, nil, nil, nil, fmt.Errorf("no environment is enabled")
	}
	env := cloneEnvironment(a.env)
	root := a.root

	upsertServe(a.network, desired)
	if err := persistNetwork(a.root, a.network); err != nil {
		return nil, nil, nil, nil, err
	}

	actor := &managedServe{
		desired:  desired,
		state:    networkpb.RuntimeState_RUNTIME_STATE_STARTING,
		tunnelID: desired.TunnelID,
	}
	key, old := a.findServeActorLocked(desired.TunnelID, desired.Name)
	if key != "" {
		delete(a.serves, key)
	}
	a.serves[serveKey(desired)] = actor

	return root, env, actor, old, nil
}

func (a *Agent) prepareConnectEnsure(desired env_core.ManagedConnect) (env_core.Root, *env_core.Environment, *managedConnect, *managedConnect, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.env == nil {
		return nil, nil, nil, nil, fmt.Errorf("no environment is enabled")
	}
	env := cloneEnvironment(a.env)
	root := a.root

	upsertConnect(a.network, desired)
	if err := persistNetwork(a.root, a.network); err != nil {
		return nil, nil, nil, nil, err
	}

	actor := &managedConnect{
		desired:  desired,
		state:    networkpb.RuntimeState_RUNTIME_STATE_STARTING,
		tunnelID: desired.TunnelID,
	}
	key, old := a.findConnectActorLocked(desired.TunnelID, desired.Name, desired.ListenAddress)
	if key != "" {
		delete(a.connects, key)
	}
	a.connects[connectKey(desired)] = actor

	return root, env, actor, old, nil
}

func (a *Agent) updateServeDesired(actor *managedServe, desired env_core.ManagedServe) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasServeActorLocked(actor) {
		return fmt.Errorf("managed serve start was canceled")
	}
	actor.desired = desired
	actor.tunnelID = desired.TunnelID
	upsertServe(a.network, desired)
	if err := persistNetwork(a.root, a.network); err != nil {
		return err
	}
	a.rekeyServeActorLocked(actor)
	return nil
}

func (a *Agent) updateConnectDesired(actor *managedConnect, desired env_core.ManagedConnect) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasConnectActorLocked(actor) {
		return fmt.Errorf("managed connect start was canceled")
	}
	actor.desired = desired
	actor.tunnelID = desired.TunnelID
	upsertConnect(a.network, desired)
	if err := persistNetwork(a.root, a.network); err != nil {
		return err
	}
	a.rekeyConnectActorLocked(actor)
	return nil
}

func (a *Agent) failServeActor(actor *managedServe, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.hasServeActorLocked(actor) {
		return
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.serveID = ""
	actor.lastError = err.Error()
	actor.cancel = nil
}

func (a *Agent) failConnectActor(actor *managedConnect, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.hasConnectActorLocked(actor) {
		return
	}
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
	actor.attachmentID = ""
	actor.lastError = err.Error()
	actor.cancel = nil
}

func (a *Agent) markServeRunning(actor *managedServe, env *env_core.Environment, tunnelID, serveID string, startedAt time.Time, cancel context.CancelFunc) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.hasServeActorLocked(actor) {
		return false
	}
	actor.env = cloneEnvironment(env)
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_RUNNING
	actor.tunnelID = tunnelID
	actor.serveID = serveID
	actor.lastError = ""
	actor.lastStartedAt = &startedAt
	actor.cancel = cancel
	return true
}

func (a *Agent) markConnectRunning(actor *managedConnect, env *env_core.Environment, tunnelID, attachmentID string, startedAt time.Time, cancel context.CancelFunc) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.hasConnectActorLocked(actor) {
		return false
	}
	actor.env = cloneEnvironment(env)
	actor.state = networkpb.RuntimeState_RUNTIME_STATE_RUNNING
	actor.tunnelID = tunnelID
	actor.attachmentID = attachmentID
	actor.lastError = ""
	actor.lastStartedAt = &startedAt
	actor.cancel = cancel
	return true
}

func (a *Agent) watchServeActor(actor *managedServe, done <-chan error) {
	err := <-done
	a.mu.Lock()
	if !a.hasServeActorLocked(actor) {
		a.mu.Unlock()
		return
	}
	cancel := actor.cancel
	env := cloneEnvironment(actor.env)
	serveID := actor.serveID
	actor.cancel = nil
	actor.serveID = ""
	if err != nil {
		actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
		actor.lastError = err.Error()
	} else {
		actor.state = networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED
	}
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

func (a *Agent) watchConnectActor(actor *managedConnect, done <-chan error) {
	err := <-done
	a.mu.Lock()
	if !a.hasConnectActorLocked(actor) {
		a.mu.Unlock()
		return
	}
	cancel := actor.cancel
	env := cloneEnvironment(actor.env)
	attachmentID := actor.attachmentID
	actor.cancel = nil
	actor.attachmentID = ""
	if err != nil {
		actor.state = networkpb.RuntimeState_RUNTIME_STATE_ERROR
		actor.lastError = err.Error()
	} else {
		actor.state = networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED
	}
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

func (a *Agent) stopServeActor(actor *managedServe) {
	if actor == nil {
		return
	}
	if actor.cancel != nil {
		actor.cancel()
	}
	if actor.serveID != "" && actor.env != nil {
		if err := a.controller.StopServe(context.Background(), actor.env, actor.serveID); err != nil {
			dl.Warnf("agora network failed to delete serve_id='%s': %v", actor.serveID, err)
		}
	}
}

func (a *Agent) stopConnectActor(actor *managedConnect) {
	if actor == nil {
		return
	}
	if actor.cancel != nil {
		actor.cancel()
	}
	if actor.attachmentID != "" && actor.env != nil {
		if err := a.controller.StopAttachment(context.Background(), actor.env, actor.attachmentID); err != nil {
			dl.Warnf("agora network failed to delete attachment_id='%s': %v", actor.attachmentID, err)
		}
	}
}

func (a *Agent) runServeHeartbeatLoop(ctx context.Context, actor *managedServe, serveID string) {
	ticker := time.NewTicker(tunnelServeHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.controller.HeartbeatServe(context.Background(), actor.env, serveID); err != nil {
				dl.Warnf("agora network serve heartbeat failed serve_id='%s': %v", serveID, err)
			}
		}
	}
}

func (a *Agent) runConnectHeartbeatLoop(ctx context.Context, actor *managedConnect, attachmentID string) {
	ticker := time.NewTicker(tunnelAttachmentHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.controller.HeartbeatAttachment(context.Background(), actor.env, attachmentID); err != nil {
				dl.Warnf("agora network attachment heartbeat failed attachment_id='%s': %v", attachmentID, err)
			}
		}
	}
}

func (a *Agent) removeServeActor(serveID, tunnelID, name string) (*managedServe, error) {
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
	if err := persistNetwork(a.root, a.network); err != nil {
		a.serves[key] = actor
		return nil, err
	}
	return actor, nil
}

func (a *Agent) removeConnectActor(attachmentID, tunnelID, name, listenAddress string) (*managedConnect, error) {
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
	if err := persistNetwork(a.root, a.network); err != nil {
		a.connects[key] = actor
		return nil, err
	}
	return actor, nil
}

func (a *Agent) shutdownManagedActors() {
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

func (a *Agent) snapshotServeStatus(actor *managedServe) *networkpb.ManagedServeStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return serveStatusProto(actor)
}

func (a *Agent) snapshotConnectStatus(actor *managedConnect) *networkpb.ManagedConnectStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return connectStatusProto(actor)
}

func (a *Agent) hasServeActorLocked(actor *managedServe) bool {
	for _, current := range a.serves {
		if current == actor {
			return true
		}
	}
	return false
}

func (a *Agent) hasConnectActorLocked(actor *managedConnect) bool {
	for _, current := range a.connects {
		if current == actor {
			return true
		}
	}
	return false
}

func (a *Agent) findServeActorLocked(tunnelID, name string) (string, *managedServe) {
	for key, actor := range a.serves {
		if matchesServe(actor.desired, tunnelID, name) {
			return key, actor
		}
	}
	return "", nil
}

func (a *Agent) findConnectActorLocked(tunnelID, name, listenAddress string) (string, *managedConnect) {
	for key, actor := range a.connects {
		if matchesConnect(actor.desired, tunnelID, name, listenAddress) {
			return key, actor
		}
	}
	return "", nil
}

func (a *Agent) findServeByServeIDLocked(serveID string) (string, *managedServe) {
	for key, actor := range a.serves {
		if actor.serveID == serveID {
			return key, actor
		}
	}
	return "", nil
}

func (a *Agent) findConnectByAttachmentIDLocked(attachmentID string) (string, *managedConnect) {
	for key, actor := range a.connects {
		if actor.attachmentID == attachmentID {
			return key, actor
		}
	}
	return "", nil
}

func (a *Agent) rekeyServeActorLocked(actor *managedServe) {
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

func (a *Agent) rekeyConnectActorLocked(actor *managedConnect) {
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
