package automation

import (
	"context"

	"github.com/go-openapi/runtime"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router_policy"
	"github.com/openziti/edge-api/rest_model"
)

type EdgeRouterPolicyOperations interface {
	Create(context.Context, *EdgeRouterPolicyOptions) (string, error)
	Delete(context.Context, string) error
	Find(context.Context, *FilterOptions) ([]*rest_model.EdgeRouterPolicyDetail, error)
	GetByID(context.Context, string) (*rest_model.EdgeRouterPolicyDetail, error)
	GetByName(context.Context, string) (*rest_model.EdgeRouterPolicyDetail, error)
}

type EdgeRouterPolicyManager struct {
	client *Client
}

type EdgeRouterPolicyOptions struct {
	BaseOptions
	IdentityRoles   []string
	EdgeRouterRoles []string
	Semantic        rest_model.Semantic
}

func NewEdgeRouterPolicyManager(client *Client) *EdgeRouterPolicyManager {
	return &EdgeRouterPolicyManager{client: client}
}

func (m *EdgeRouterPolicyManager) Create(ctx context.Context, opts *EdgeRouterPolicyOptions) (string, error) {
	req := edge_router_policy.NewCreateEdgeRouterPolicyParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	req.Policy = &rest_model.EdgeRouterPolicyCreate{
		EdgeRouterRoles: opts.EdgeRouterRoles,
		IdentityRoles:   opts.IdentityRoles,
		Name:            &opts.Name,
		Semantic:        &opts.Semantic,
		Tags:            opts.tags().ToRESTModel(),
	}
	resp, err := m.client.edge.EdgeRouterPolicy.CreateEdgeRouterPolicy(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return "", wrapError("edge_router_policy", "create", err)
	}
	return resp.Payload.Data.ID, nil
}

func (m *EdgeRouterPolicyManager) Delete(ctx context.Context, id string) error {
	req := edge_router_policy.NewDeleteEdgeRouterPolicyParamsWithContext(ctx)
	req.SetTimeout(m.client.cfg.OperationTimeout)
	req.ID = id
	_, err := m.client.edge.EdgeRouterPolicy.DeleteEdgeRouterPolicy(req, runtime.ClientAuthInfoWriter(nil))
	return wrapError("edge_router_policy", "delete", err)
}

func (m *EdgeRouterPolicyManager) Find(ctx context.Context, opts *FilterOptions) ([]*rest_model.EdgeRouterPolicyDetail, error) {
	req := edge_router_policy.NewListEdgeRouterPoliciesParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	filter := filterValue(opts)
	req.Filter = &filter
	limit := opts.limit()
	req.Limit = &limit
	offset := opts.offset()
	req.Offset = &offset
	resp, err := m.client.edge.EdgeRouterPolicy.ListEdgeRouterPolicies(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("edge_router_policy", "find", err)
	}
	return resp.Payload.Data, nil
}

func (m *EdgeRouterPolicyManager) GetByID(ctx context.Context, id string) (*rest_model.EdgeRouterPolicyDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("id", id)})
	if err != nil {
		return nil, err
	}
	return oneResult("edge_router_policy", "get_by_id", id, items)
}

func (m *EdgeRouterPolicyManager) GetByName(ctx context.Context, name string) (*rest_model.EdgeRouterPolicyDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("name", name)})
	if err != nil {
		return nil, err
	}
	return oneResult("edge_router_policy", "get_by_name", name, items)
}
