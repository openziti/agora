package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func TestAdmissionTunnelLockSerializesAttachAgainstGrantRemoval(t *testing.T) {
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	envLC := &admissionEnvironmentLifecycle{}
	tunnelLC := &admissionTunnelLifecycle{}
	env.service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return envLC, tunnelLC, nil
	}

	orgID, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	bobID, bobToken := env.addAccountToOrg(t, orgID, "bob@example.com")
	alice := env.accountClient(t, aliceToken)
	bob := env.accountClient(t, bobToken)

	aliceEnvID := env.enableEnvironment(t, aliceToken)
	bobEnvID := env.enableEnvironment(t, bobToken)
	createRes, err := alice.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		EnvironmentId: aliceEnvID,
		Name:          "grant-removal-race",
		Mode:          api.TunnelModeTCP,
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}
	tunnel := createRes.(*api.Tunnel)
	if _, err := alice.AddTunnelGrant(env.ctx, &api.AddTunnelGrantRequest{Email: "bob@example.com"}, api.AddTunnelGrantParams{TunnelId: tunnel.ID}); err != nil {
		t.Fatalf("add tunnel grant: %v", err)
	}

	attachRes, err := bob.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{Name: tunnel.Name, EnvironmentId: bobEnvID})
	if err != nil {
		t.Fatalf("initial attach: %v", err)
	}
	if _, ok := attachRes.(*api.ConnectTunnelResponse); !ok {
		t.Fatalf("expected initial attach response, got %T", attachRes)
	}

	block := newAdmissionBlock()
	tunnelLC.blockDeprovision = block
	removeDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := alice.RemoveTunnelGrant(env.ctx, api.RemoveTunnelGrantParams{TunnelId: tunnel.ID, AccountId: bobID})
		removeDone <- admissionOpResult{res: res, err: err}
	}()
	waitForAdmissionBlock(t, block, "grant removal deprovision")

	attachDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := bob.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{Name: tunnel.Name, EnvironmentId: bobEnvID})
		attachDone <- admissionOpResult{res: res, err: err}
	}()
	assertNoAdmissionResult(t, attachDone, "attach while grant removal holds tunnel lock")

	block.releaseNow()
	removeResult := waitForAdmissionResult(t, removeDone, "grant removal")
	if removeResult.err != nil {
		t.Fatalf("remove grant: %v", removeResult.err)
	}
	if _, ok := removeResult.res.(*api.RemoveTunnelGrantNoContent); !ok {
		t.Fatalf("expected remove grant 204, got %T", removeResult.res)
	}
	attachResult := waitForAdmissionResult(t, attachDone, "attach after grant removal")
	if attachResult.err != nil {
		t.Fatalf("racing attach: %v", attachResult.err)
	}
	if _, ok := attachResult.res.(*api.ConnectTunnelNotFound); !ok {
		t.Fatalf("expected racing attach to re-check grant and fail, got %T", attachResult.res)
	}
	if _, err := env.store.TunnelAttachments.GetActiveDialerByEnvironmentAndTunnel(env.ctx, env.store.DB(), bobEnvID, tunnel.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected no active dialer after grant removal, got err=%v", err)
	}
}

func TestAdmissionAttachLockSerializesConcurrentAttachByNaturalKey(t *testing.T) {
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	envLC := &admissionEnvironmentLifecycle{}
	tunnelLC := &admissionTunnelLifecycle{}
	env.service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return envLC, tunnelLC, nil
	}

	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)

	envID := env.enableEnvironment(t, aliceToken)
	createRes, err := alice.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		EnvironmentId: envID,
		Name:          "attach-race",
		Mode:          api.TunnelModeTCP,
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}
	tunnel := createRes.(*api.Tunnel)

	block := newAdmissionBlock()
	tunnelLC.blockAttachment = block
	firstDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := alice.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{Name: tunnel.Name, EnvironmentId: envID})
		firstDone <- admissionOpResult{res: res, err: err}
	}()
	waitForAdmissionBlock(t, block, "first attach dial policy")

	secondDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := alice.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{Name: tunnel.Name, EnvironmentId: envID})
		secondDone <- admissionOpResult{res: res, err: err}
	}()
	assertNoAdmissionResult(t, secondDone, "second attach while first holds natural-key locks")

	block.releaseNow()
	firstResult := waitForAdmissionResult(t, firstDone, "first attach")
	secondResult := waitForAdmissionResult(t, secondDone, "second attach")
	if firstResult.err != nil {
		t.Fatalf("first attach: %v", firstResult.err)
	}
	if secondResult.err != nil {
		t.Fatalf("second attach: %v", secondResult.err)
	}
	first, ok := firstResult.res.(*api.ConnectTunnelResponse)
	if !ok {
		t.Fatalf("expected first attach response, got %T", firstResult.res)
	}
	second, ok := secondResult.res.(*api.ConnectTunnelResponse)
	if !ok {
		t.Fatalf("expected second attach response, got %T", secondResult.res)
	}
	if first.Attachment.ID != second.Attachment.ID {
		t.Fatalf("expected second attach to return existing attachment %q, got %q", first.Attachment.ID, second.Attachment.ID)
	}
	if got := len(tunnelLC.attachmentCallsSnapshot()); got != 1 {
		t.Fatalf("expected exactly one dial policy creation, got %d", got)
	}
}

func TestAdmissionEnvironmentLockSerializesCreateTunnelAgainstDisable(t *testing.T) {
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	envLC := &admissionEnvironmentLifecycle{}
	tunnelLC := &admissionTunnelLifecycle{}
	env.service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return envLC, tunnelLC, nil
	}

	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	envID := env.enableEnvironment(t, aliceToken)

	block := newAdmissionBlock()
	tunnelLC.blockProvision = block
	createDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := alice.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
			EnvironmentId: envID,
			Name:          "disable-race",
			Mode:          api.TunnelModeTCP,
		})
		createDone <- admissionOpResult{res: res, err: err}
	}()
	waitForAdmissionBlock(t, block, "create tunnel provision")

	disableDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := alice.DisableEnvironment(env.ctx, api.DisableEnvironmentParams{EnvironmentId: envID})
		disableDone <- admissionOpResult{res: res, err: err}
	}()
	assertNoAdmissionResult(t, disableDone, "disable environment while create holds environment lock")

	block.releaseNow()
	createResult := waitForAdmissionResult(t, createDone, "create tunnel")
	if createResult.err != nil {
		t.Fatalf("create tunnel: %v", createResult.err)
	}
	created, ok := createResult.res.(*api.Tunnel)
	if !ok {
		t.Fatalf("expected created tunnel, got %T", createResult.res)
	}
	disableResult := waitForAdmissionResult(t, disableDone, "disable environment")
	if disableResult.err != nil {
		t.Fatalf("disable environment: %v", disableResult.err)
	}
	if _, ok := disableResult.res.(*api.DisableEnvironmentNoContent); !ok {
		t.Fatalf("expected disable environment 204, got %T", disableResult.res)
	}
	if _, err := env.store.Tunnels.GetByID(env.ctx, env.store.DB(), created.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected created tunnel to be removed by disable, got err=%v", err)
	}
	if len(tunnelLC.deprovisionCalls()) == 0 {
		t.Fatalf("expected disable environment to deprovision the created tunnel")
	}
}

func TestAdmissionAccountLockSerializesEnableEnvironmentAgainstAccountDelete(t *testing.T) {
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	envLC := &admissionEnvironmentLifecycle{}
	tunnelLC := &admissionTunnelLifecycle{}
	env.service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return envLC, tunnelLC, nil
	}

	orgID, accountID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	admin := env.adminClient(t)

	block := newAdmissionBlock()
	envLC.blockEnable = block
	enableDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := alice.EnableEnvironment(env.ctx, &api.EnableEnvironmentRequest{})
		enableDone <- admissionOpResult{res: res, err: err}
	}()
	waitForAdmissionBlock(t, block, "enable environment")

	deleteDone := make(chan admissionOpResult, 1)
	go func() {
		res, err := admin.DeleteAccount(env.ctx, api.DeleteAccountParams{OrganizationId: orgID, AccountId: accountID})
		deleteDone <- admissionOpResult{res: res, err: err}
	}()
	assertNoAdmissionResult(t, deleteDone, "delete account while enable holds account lock")

	block.releaseNow()
	enableResult := waitForAdmissionResult(t, enableDone, "enable environment")
	if enableResult.err != nil {
		t.Fatalf("enable environment: %v", enableResult.err)
	}
	if _, ok := enableResult.res.(*api.EnableEnvironmentResponse); !ok {
		t.Fatalf("expected enable environment response, got %T", enableResult.res)
	}
	deleteResult := waitForAdmissionResult(t, deleteDone, "delete account")
	if deleteResult.err != nil {
		t.Fatalf("delete account: %v", deleteResult.err)
	}
	if _, ok := deleteResult.res.(*api.DeleteAccountNoContent); !ok {
		t.Fatalf("expected delete account 204, got %T", deleteResult.res)
	}
	if _, err := env.store.Accounts.GetByID(env.ctx, env.store.DB(), accountID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected account deleted, got err=%v", err)
	}
	if len(envLC.disableCallsSnapshot()) == 0 {
		t.Fatalf("expected account delete to disable the just-created environment")
	}
}

type admissionOpResult struct {
	res any
	err error
}

type admissionBlock struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newAdmissionBlock() *admissionBlock {
	return &admissionBlock{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (b *admissionBlock) wait(ctx context.Context) error {
	b.startedOnce.Do(func() { close(b.started) })
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *admissionBlock) releaseNow() {
	b.releaseOnce.Do(func() { close(b.release) })
}

func waitForAdmissionBlock(t *testing.T, block *admissionBlock, name string) {
	t.Helper()
	select {
	case <-block.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s to reach blocked lifecycle call", name)
	}
}

func assertNoAdmissionResult(t *testing.T, ch <-chan admissionOpResult, name string) {
	t.Helper()
	select {
	case result := <-ch:
		t.Fatalf("%s completed before lock release: res=%T err=%v", name, result.res, result.err)
	case <-time.After(250 * time.Millisecond):
	}
}

func waitForAdmissionResult(t *testing.T, ch <-chan admissionOpResult, name string) admissionOpResult {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return admissionOpResult{}
	}
}

type admissionEnvironmentLifecycle struct {
	mu           sync.Mutex
	enableCalls  []automation.EnvironmentSpec
	disableCalls []automation.DeprovisionEnvironmentSpec
	blockEnable  *admissionBlock
}

func (f *admissionEnvironmentLifecycle) Enable(ctx context.Context, spec automation.EnvironmentSpec) (*automation.ProvisionedEnvironment, error) {
	f.mu.Lock()
	f.enableCalls = append(f.enableCalls, spec)
	f.mu.Unlock()
	if f.blockEnable != nil {
		if err := f.blockEnable.wait(ctx); err != nil {
			return nil, err
		}
	}
	return &automation.ProvisionedEnvironment{
		IdentityID:     fmt.Sprintf("%s-identity", spec.EnvironmentID),
		EnrollmentJSON: []byte(`{}`),
		PolicyID:       fmt.Sprintf("%s-erp", spec.EnvironmentID),
	}, nil
}

func (f *admissionEnvironmentLifecycle) Disable(_ context.Context, spec automation.DeprovisionEnvironmentSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disableCalls = append(f.disableCalls, spec)
	return nil
}

func (f *admissionEnvironmentLifecycle) disableCallsSnapshot() []automation.DeprovisionEnvironmentSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]automation.DeprovisionEnvironmentSpec, len(f.disableCalls))
	copy(out, f.disableCalls)
	return out
}

type admissionTunnelLifecycle struct {
	mu               sync.Mutex
	provisionCalls   []automation.TunnelSpec
	attachmentCalls  []automation.TunnelAccessSpec
	deprovisions     []automation.DeprovisionTunnelSpec
	blockProvision   *admissionBlock
	blockAttachment  *admissionBlock
	blockDeprovision *admissionBlock
}

func (f *admissionTunnelLifecycle) Provision(ctx context.Context, spec automation.TunnelSpec) (*automation.ProvisionedTunnel, error) {
	f.mu.Lock()
	f.provisionCalls = append(f.provisionCalls, spec)
	f.mu.Unlock()
	if f.blockProvision != nil {
		if err := f.blockProvision.wait(ctx); err != nil {
			return nil, err
		}
	}
	return &automation.ProvisionedTunnel{
		ServiceID:                 fmt.Sprintf("%s-service", spec.TunnelID),
		BindPolicyID:              fmt.Sprintf("%s-bind", spec.TunnelID),
		ServiceEdgeRouterPolicyID: fmt.Sprintf("%s-serp", spec.TunnelID),
	}, nil
}

func (f *admissionTunnelLifecycle) CreateAttachmentDialPolicy(ctx context.Context, spec automation.TunnelAccessSpec) (string, error) {
	f.mu.Lock()
	f.attachmentCalls = append(f.attachmentCalls, spec)
	f.mu.Unlock()
	if f.blockAttachment != nil {
		if err := f.blockAttachment.wait(ctx); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%s-dial", spec.AttachmentID), nil
}

func (f *admissionTunnelLifecycle) Deprovision(ctx context.Context, spec automation.DeprovisionTunnelSpec) error {
	f.mu.Lock()
	f.deprovisions = append(f.deprovisions, spec)
	f.mu.Unlock()
	if f.blockDeprovision != nil {
		if err := f.blockDeprovision.wait(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (f *admissionTunnelLifecycle) deprovisionCalls() []automation.DeprovisionTunnelSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]automation.DeprovisionTunnelSpec, len(f.deprovisions))
	copy(out, f.deprovisions)
	return out
}

func (f *admissionTunnelLifecycle) attachmentCallsSnapshot() []automation.TunnelAccessSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]automation.TunnelAccessSpec, len(f.attachmentCalls))
	copy(out, f.attachmentCalls)
	return out
}
