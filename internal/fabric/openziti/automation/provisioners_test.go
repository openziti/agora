package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/openziti/edge-api/rest_model"
)

func TestEnvironmentEnableAndDisable(t *testing.T) {
	identity := &fakeIdentityOperations{
		createID: "identity-1",
		enroll:   []byte(`{"id":"identity-1"}`),
	}
	erp := &fakeEdgeRouterPolicyOperations{createID: "erp-1"}

	provisioner := &EnvironmentProvisioner{
		identities:         identity,
		edgeRouterPolicies: erp,
	}

	result, err := provisioner.Enable(context.Background(), EnvironmentSpec{
		OrganizationID: "org_test00000001",
		AccountID:      "ac_test00000001",
		EnvironmentID:  "ev_test00000001",
		IdentityName:   "env-1",
		Version:        "test",
	})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if result.IdentityID != "identity-1" {
		t.Fatalf("expected identity id, got %q", result.IdentityID)
	}
	if result.PolicyID != "erp-1" {
		t.Fatalf("expected policy id, got %q", result.PolicyID)
	}
	if identity.created == nil || identity.created.Name != "env-1" {
		t.Fatalf("expected created identity options to be captured")
	}
	if erp.created == nil || len(erp.created.EdgeRouterRoles) != 1 || erp.created.EdgeRouterRoles[0] != defaultEdgeRouterRole {
		t.Fatalf("expected default edge-router role %q, got %#v", defaultEdgeRouterRole, erp.created)
	}

	if err := provisioner.Disable(context.Background(), DeprovisionEnvironmentSpec{
		IdentityID:         result.IdentityID,
		EdgeRouterPolicyID: result.PolicyID,
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if len(identity.deleted) != 1 || identity.deleted[0] != "identity-1" {
		t.Fatalf("expected identity delete, got %#v", identity.deleted)
	}
	if len(erp.deleted) != 1 || erp.deleted[0] != "erp-1" {
		t.Fatalf("expected erp delete, got %#v", erp.deleted)
	}
}

func TestEnvironmentEnableCleansUpOnPolicyFailure(t *testing.T) {
	identity := &fakeIdentityOperations{
		createID: "identity-1",
		enroll:   []byte(`{"id":"identity-1"}`),
	}
	erp := &fakeEdgeRouterPolicyOperations{createErr: errors.New("boom")}

	provisioner := &EnvironmentProvisioner{
		identities:         identity,
		edgeRouterPolicies: erp,
	}

	if _, err := provisioner.Enable(context.Background(), EnvironmentSpec{
		OrganizationID: "org_test00000002",
		AccountID:      "ac_test00000002",
		EnvironmentID:  "ev_test00000002",
		IdentityName:   "env-1",
	}); err == nil {
		t.Fatal("expected enable failure")
	}
	if len(identity.deleted) != 1 || identity.deleted[0] != "identity-1" {
		t.Fatalf("expected identity cleanup, got %#v", identity.deleted)
	}
}

func TestTunnelProvisionAndDeprovision(t *testing.T) {
	services := &fakeServiceOperations{createID: "service-1"}
	policies := &fakeServicePolicyOperations{
		createIDs: []string{"bind-1", "dial-1"},
	}
	serp := &fakeServiceEdgeRouterPolicyOperations{createID: "serp-1"}

	provisioner := &TunnelProvisioner{
		services:                  services,
		servicePolicies:           policies,
		serviceEdgeRouterPolicies: serp,
	}

	result, err := provisioner.Provision(context.Background(), TunnelSpec{
		OrganizationID:    "org_test00000003",
		AccountID:         "ac_test00000003",
		EnvironmentID:     "ev_test00000003",
		TunnelID:          "tt_test00000003",
		TunnelName:        "llm-gateway",
		ServiceName:       "tt_test00000003",
		BindIdentityRoles: []string{"@identity-1"},
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if result.ServiceID != "service-1" || result.BindPolicyID != "bind-1" || result.ServiceEdgeRouterPolicyID != "serp-1" {
		t.Fatalf("unexpected tunnel result: %#v", result)
	}
	if len(policies.created) != 1 {
		t.Fatalf("expected 1 service policy, got %d", len(policies.created))
	}
	if policies.created[0].PolicyType != rest_model.DialBindBind {
		t.Fatalf("expected bind policy first, got %v", policies.created[0].PolicyType)
	}
	if serp.created == nil || serp.created.Name != "tt_test00000003-serp" {
		t.Fatalf("expected service-edge-router policy name to be tunnel-id based, got %#v", serp.created)
	}
	if len(serp.created.EdgeRouterRoles) != 1 || serp.created.EdgeRouterRoles[0] != defaultEdgeRouterRole {
		t.Fatalf("expected default edge-router role %q, got %#v", defaultEdgeRouterRole, serp.created.EdgeRouterRoles)
	}

	if err := provisioner.Deprovision(context.Background(), DeprovisionTunnelSpec{
		ServiceID:                 result.ServiceID,
		BindPolicyID:              result.BindPolicyID,
		ServiceEdgeRouterPolicyID: result.ServiceEdgeRouterPolicyID,
	}); err != nil {
		t.Fatalf("deprovision: %v", err)
	}
	if len(services.deleted) != 1 || services.deleted[0] != "service-1" {
		t.Fatalf("expected service delete, got %#v", services.deleted)
	}
}

type fakeTerminatorOperations struct {
	deleteFilters []string
}

func (f *fakeTerminatorOperations) Find(context.Context, *FilterOptions) ([]*rest_model.TerminatorDetail, error) {
	return nil, nil
}
func (f *fakeTerminatorOperations) Delete(context.Context, string) error { return nil }
func (f *fakeTerminatorOperations) DeleteWithFilter(_ context.Context, filter string) error {
	f.deleteFilters = append(f.deleteFilters, filter)
	return nil
}

// pins the OpenZiti terminator filter syntax: takeover evicts by service id, retirement evicts by
// hostId (the binding identity). hostId -- not identity -- is the filterable field; filtering on
// 'identity' returns 400 INVALID_FILTER from the real fabric.
func TestEvictTerminatorsUsesCorrectFilters(t *testing.T) {
	term := &fakeTerminatorOperations{}
	p := &TunnelProvisioner{terminators: term}

	if err := p.EvictTerminatorsByService(context.Background(), "svc-1"); err != nil {
		t.Fatalf("evict by service: %v", err)
	}
	if err := p.EvictTerminatorsByIdentity(context.Background(), "id-1"); err != nil {
		t.Fatalf("evict by identity: %v", err)
	}

	want := []string{`service="svc-1"`, `hostId="id-1"`}
	if len(term.deleteFilters) != 2 || term.deleteFilters[0] != want[0] || term.deleteFilters[1] != want[1] {
		t.Fatalf("expected filters %v, got %v", want, term.deleteFilters)
	}
}

func TestBootstrapVerifiesConnectivityAndEdgeRouters(t *testing.T) {
	identity := &fakeIdentityOperations{}
	edgeRouterID := "er-1"
	edgeRouters := &fakeEdgeRouterOperations{
		items: []*rest_model.EdgeRouterDetail{{BaseEntity: rest_model.BaseEntity{ID: &edgeRouterID}}},
	}

	bootstrapper := &Bootstrapper{
		identities:  identity,
		edgeRouters: edgeRouters,
		cleanup:     func(context.Context, string) error { return nil },
	}

	if err := bootstrapper.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
}

func TestBootstrapFailsWhenNoEdgeRoutersExist(t *testing.T) {
	bootstrapper := &Bootstrapper{
		identities:  &fakeIdentityOperations{},
		edgeRouters: &fakeEdgeRouterOperations{},
		cleanup:     func(context.Context, string) error { return nil },
	}

	if err := bootstrapper.Bootstrap(context.Background()); err == nil {
		t.Fatal("expected bootstrap failure")
	}
}

func TestUnbootstrapCleansAgoraTaggedResources(t *testing.T) {
	var cleanedTag string
	bootstrapper := &Bootstrapper{
		identities:  &fakeIdentityOperations{},
		edgeRouters: &fakeEdgeRouterOperations{},
		cleanup: func(_ context.Context, tag string) error {
			cleanedTag = tag
			return nil
		},
	}

	if err := bootstrapper.Unbootstrap(context.Background()); err != nil {
		t.Fatalf("unbootstrap: %v", err)
	}
	if cleanedTag != "agora" {
		t.Fatalf("expected cleanup tag agora, got %q", cleanedTag)
	}
}

func TestCleanupByTagDeletesInDependencyOrder(t *testing.T) {
	var calls []string
	client := &Client{
		ServiceEdgeRouterPolicies: &orderedServiceEdgeRouterPolicyOperations{calls: &calls},
		ServicePolicies:           &orderedServicePolicyOperations{calls: &calls},
		Services:                  &orderedServiceOperations{calls: &calls},
		EdgeRouterPolicies:        &orderedEdgeRouterPolicyOperations{calls: &calls},
		Identities:                &orderedIdentityOperations{calls: &calls},
	}

	if err := client.CleanupByTag(context.Background(), "agora"); err != nil {
		t.Fatalf("cleanup by tag: %v", err)
	}

	expected := []string{"serp", "service_policy", "service", "edge_router_policy", "identity"}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d calls, got %d (%v)", len(expected), len(calls), calls)
	}
	for i := range expected {
		if calls[i] != expected[i] {
			t.Fatalf("expected call %d to be %q, got %q", i, expected[i], calls[i])
		}
	}
}

func TestTunnelProvisionCleansUpOnServiceEdgeRouterPolicyFailure(t *testing.T) {
	services := &fakeServiceOperations{createID: "service-1"}
	policies := &fakeServicePolicyOperations{
		createIDs: []string{"bind-1"},
	}
	serp := &fakeServiceEdgeRouterPolicyOperations{createErr: errors.New("boom")}

	provisioner := &TunnelProvisioner{
		services:                  services,
		servicePolicies:           policies,
		serviceEdgeRouterPolicies: serp,
	}

	if _, err := provisioner.Provision(context.Background(), TunnelSpec{
		OrganizationID:    "org_test00000004",
		AccountID:         "ac_test00000004",
		EnvironmentID:     "ev_test00000004",
		TunnelID:          "tt_test00000004",
		TunnelName:        "llm-gateway",
		ServiceName:       "tt_test00000004",
		BindIdentityRoles: []string{"@identity-1"},
	}); err == nil {
		t.Fatal("expected provision failure")
	}
	if len(policies.deleted) != 1 || policies.deleted[0] != "bind-1" {
		t.Fatalf("expected bind policy cleanup, got %#v", policies.deleted)
	}
	if len(services.deleted) != 1 || services.deleted[0] != "service-1" {
		t.Fatalf("expected service cleanup, got %#v", services.deleted)
	}
}

func TestTunnelCreateAttachmentDialPolicy(t *testing.T) {
	policies := &fakeServicePolicyOperations{
		createIDs: []string{"dial-1"},
	}

	provisioner := &TunnelProvisioner{
		servicePolicies: policies,
	}

	dialPolicyID, err := provisioner.CreateAttachmentDialPolicy(context.Background(), TunnelAccessSpec{
		OrganizationID:        "org_test00000005",
		AccountID:             "ac_test00000005",
		EnvironmentID:         "ev_test00000005",
		TunnelID:              "tt_test00000005",
		TunnelName:            "llm-gateway",
		AttachmentID:          "ta_test00000005",
		EnvironmentIdentityID: "identity-1",
		ServiceID:             "service-1",
		Version:               DefaultAgoraVersion,
	})
	if err != nil {
		t.Fatalf("create attachment dial policy: %v", err)
	}
	if dialPolicyID != "dial-1" {
		t.Fatalf("expected dial policy id, got %q", dialPolicyID)
	}
	if len(policies.created) != 1 {
		t.Fatalf("expected 1 dial policy create, got %d", len(policies.created))
	}
	if policies.created[0].PolicyType != rest_model.DialBindDial {
		t.Fatalf("expected dial policy type, got %v", policies.created[0].PolicyType)
	}
}

type fakeIdentityOperations struct {
	createID  string
	createErr error
	enroll    []byte
	enrollErr error
	created   *IdentityOptions
	deleted   []string
}

func (f *fakeIdentityOperations) Create(_ context.Context, opts *IdentityOptions) (string, error) {
	f.created = opts
	return f.createID, f.createErr
}
func (f *fakeIdentityOperations) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeIdentityOperations) DeleteWithFilter(_ context.Context, _ string) error {
	return nil
}
func (f *fakeIdentityOperations) Find(context.Context, *FilterOptions) ([]*rest_model.IdentityDetail, error) {
	return nil, nil
}
func (f *fakeIdentityOperations) GetByID(context.Context, string) (*rest_model.IdentityDetail, error) {
	return nil, nil
}
func (f *fakeIdentityOperations) GetByName(context.Context, string) (*rest_model.IdentityDetail, error) {
	return nil, nil
}
func (f *fakeIdentityOperations) Enroll(context.Context, string) ([]byte, error) {
	return f.enroll, f.enrollErr
}

type fakeEdgeRouterPolicyOperations struct {
	createID  string
	createErr error
	created   *EdgeRouterPolicyOptions
	deleted   []string
}

func (f *fakeEdgeRouterPolicyOperations) Create(_ context.Context, opts *EdgeRouterPolicyOptions) (string, error) {
	f.created = opts
	return f.createID, f.createErr
}
func (f *fakeEdgeRouterPolicyOperations) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeEdgeRouterPolicyOperations) DeleteWithFilter(_ context.Context, _ string) error {
	return nil
}
func (f *fakeEdgeRouterPolicyOperations) Find(context.Context, *FilterOptions) ([]*rest_model.EdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (f *fakeEdgeRouterPolicyOperations) GetByID(context.Context, string) (*rest_model.EdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (f *fakeEdgeRouterPolicyOperations) GetByName(context.Context, string) (*rest_model.EdgeRouterPolicyDetail, error) {
	return nil, nil
}

type fakeServiceOperations struct {
	createID  string
	createErr error
	deleted   []string
}

func (f *fakeServiceOperations) Create(context.Context, *ServiceOptions) (string, error) {
	return f.createID, f.createErr
}
func (f *fakeServiceOperations) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeServiceOperations) DeleteWithFilter(_ context.Context, _ string) error {
	return nil
}
func (f *fakeServiceOperations) Find(context.Context, *FilterOptions) ([]*rest_model.ServiceDetail, error) {
	return nil, nil
}
func (f *fakeServiceOperations) GetByID(context.Context, string) (*rest_model.ServiceDetail, error) {
	return nil, nil
}
func (f *fakeServiceOperations) GetByName(context.Context, string) (*rest_model.ServiceDetail, error) {
	return nil, nil
}

type fakeServicePolicyOperations struct {
	createIDs []string
	created   []*ServicePolicyOptions
	bindErr   error
	dialErr   error
	deleted   []string
}

func (f *fakeServicePolicyOperations) Create(context.Context, *ServicePolicyOptions) (string, error) {
	panic("not used")
}
func (f *fakeServicePolicyOperations) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeServicePolicyOperations) DeleteWithFilter(_ context.Context, _ string) error {
	return nil
}
func (f *fakeServicePolicyOperations) Find(context.Context, *FilterOptions) ([]*rest_model.ServicePolicyDetail, error) {
	return nil, nil
}
func (f *fakeServicePolicyOperations) GetByID(context.Context, string) (*rest_model.ServicePolicyDetail, error) {
	return nil, nil
}
func (f *fakeServicePolicyOperations) GetByName(context.Context, string) (*rest_model.ServicePolicyDetail, error) {
	return nil, nil
}
func (f *fakeServicePolicyOperations) CreateBind(_ context.Context, opts *ServicePolicyOptions) (string, error) {
	if f.bindErr != nil {
		return "", f.bindErr
	}
	opts.PolicyType = rest_model.DialBindBind
	f.created = append(f.created, opts)
	return f.createIDs[0], nil
}
func (f *fakeServicePolicyOperations) CreateDial(_ context.Context, opts *ServicePolicyOptions) (string, error) {
	if f.dialErr != nil {
		return "", f.dialErr
	}
	opts.PolicyType = rest_model.DialBindDial
	f.created = append(f.created, opts)
	return f.createIDs[len(f.created)-1], nil
}

type fakeServiceEdgeRouterPolicyOperations struct {
	createID  string
	createErr error
	created   *ServiceEdgeRouterPolicyOptions
	deleted   []string
}

func (f *fakeServiceEdgeRouterPolicyOperations) Create(_ context.Context, opts *ServiceEdgeRouterPolicyOptions) (string, error) {
	f.created = opts
	return f.createID, f.createErr
}
func (f *fakeServiceEdgeRouterPolicyOperations) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeServiceEdgeRouterPolicyOperations) DeleteWithFilter(_ context.Context, _ string) error {
	return nil
}
func (f *fakeServiceEdgeRouterPolicyOperations) Find(context.Context, *FilterOptions) ([]*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (f *fakeServiceEdgeRouterPolicyOperations) GetByID(context.Context, string) (*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (f *fakeServiceEdgeRouterPolicyOperations) GetByName(context.Context, string) (*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	return nil, nil
}

type fakeEdgeRouterOperations struct {
	items []*rest_model.EdgeRouterDetail
	err   error
}

func (f *fakeEdgeRouterOperations) Find(context.Context, *FilterOptions) ([]*rest_model.EdgeRouterDetail, error) {
	return f.items, f.err
}

type orderedIdentityOperations struct{ calls *[]string }

func (o *orderedIdentityOperations) Create(context.Context, *IdentityOptions) (string, error) {
	panic("not used")
}
func (o *orderedIdentityOperations) Delete(context.Context, string) error { return nil }
func (o *orderedIdentityOperations) DeleteWithFilter(context.Context, string) error {
	*o.calls = append(*o.calls, "identity")
	return nil
}
func (o *orderedIdentityOperations) Find(context.Context, *FilterOptions) ([]*rest_model.IdentityDetail, error) {
	return nil, nil
}
func (o *orderedIdentityOperations) GetByID(context.Context, string) (*rest_model.IdentityDetail, error) {
	return nil, nil
}
func (o *orderedIdentityOperations) GetByName(context.Context, string) (*rest_model.IdentityDetail, error) {
	return nil, nil
}
func (o *orderedIdentityOperations) Enroll(context.Context, string) ([]byte, error) { return nil, nil }

type orderedServiceOperations struct{ calls *[]string }

func (o *orderedServiceOperations) Create(context.Context, *ServiceOptions) (string, error) {
	panic("not used")
}
func (o *orderedServiceOperations) Delete(context.Context, string) error { return nil }
func (o *orderedServiceOperations) DeleteWithFilter(context.Context, string) error {
	*o.calls = append(*o.calls, "service")
	return nil
}
func (o *orderedServiceOperations) Find(context.Context, *FilterOptions) ([]*rest_model.ServiceDetail, error) {
	return nil, nil
}
func (o *orderedServiceOperations) GetByID(context.Context, string) (*rest_model.ServiceDetail, error) {
	return nil, nil
}
func (o *orderedServiceOperations) GetByName(context.Context, string) (*rest_model.ServiceDetail, error) {
	return nil, nil
}

type orderedEdgeRouterPolicyOperations struct{ calls *[]string }

func (o *orderedEdgeRouterPolicyOperations) Create(context.Context, *EdgeRouterPolicyOptions) (string, error) {
	panic("not used")
}
func (o *orderedEdgeRouterPolicyOperations) Delete(context.Context, string) error { return nil }
func (o *orderedEdgeRouterPolicyOperations) DeleteWithFilter(context.Context, string) error {
	*o.calls = append(*o.calls, "edge_router_policy")
	return nil
}
func (o *orderedEdgeRouterPolicyOperations) Find(context.Context, *FilterOptions) ([]*rest_model.EdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (o *orderedEdgeRouterPolicyOperations) GetByID(context.Context, string) (*rest_model.EdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (o *orderedEdgeRouterPolicyOperations) GetByName(context.Context, string) (*rest_model.EdgeRouterPolicyDetail, error) {
	return nil, nil
}

type orderedServicePolicyOperations struct{ calls *[]string }

func (o *orderedServicePolicyOperations) Create(context.Context, *ServicePolicyOptions) (string, error) {
	panic("not used")
}
func (o *orderedServicePolicyOperations) Delete(context.Context, string) error { return nil }
func (o *orderedServicePolicyOperations) DeleteWithFilter(context.Context, string) error {
	*o.calls = append(*o.calls, "service_policy")
	return nil
}
func (o *orderedServicePolicyOperations) Find(context.Context, *FilterOptions) ([]*rest_model.ServicePolicyDetail, error) {
	return nil, nil
}
func (o *orderedServicePolicyOperations) GetByID(context.Context, string) (*rest_model.ServicePolicyDetail, error) {
	return nil, nil
}
func (o *orderedServicePolicyOperations) GetByName(context.Context, string) (*rest_model.ServicePolicyDetail, error) {
	return nil, nil
}
func (o *orderedServicePolicyOperations) CreateBind(context.Context, *ServicePolicyOptions) (string, error) {
	panic("not used")
}
func (o *orderedServicePolicyOperations) CreateDial(context.Context, *ServicePolicyOptions) (string, error) {
	panic("not used")
}

type orderedServiceEdgeRouterPolicyOperations struct{ calls *[]string }

func (o *orderedServiceEdgeRouterPolicyOperations) Create(context.Context, *ServiceEdgeRouterPolicyOptions) (string, error) {
	panic("not used")
}
func (o *orderedServiceEdgeRouterPolicyOperations) Delete(context.Context, string) error { return nil }
func (o *orderedServiceEdgeRouterPolicyOperations) DeleteWithFilter(context.Context, string) error {
	*o.calls = append(*o.calls, "serp")
	return nil
}
func (o *orderedServiceEdgeRouterPolicyOperations) Find(context.Context, *FilterOptions) ([]*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (o *orderedServiceEdgeRouterPolicyOperations) GetByID(context.Context, string) (*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	return nil, nil
}
func (o *orderedServiceEdgeRouterPolicyOperations) GetByName(context.Context, string) (*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	return nil, nil
}
