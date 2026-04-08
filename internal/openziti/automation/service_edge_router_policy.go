package automation

import (
	"context"

	"github.com/go-openapi/runtime"
	"github.com/openziti/edge-api/rest_management_api_client/service_edge_router_policy"
	"github.com/openziti/edge-api/rest_model"
)

type ServiceEdgeRouterPolicyOperations interface {
	Create(context.Context, *ServiceEdgeRouterPolicyOptions) (string, error)
	Delete(context.Context, string) error
	Find(context.Context, *FilterOptions) ([]*rest_model.ServiceEdgeRouterPolicyDetail, error)
	GetByID(context.Context, string) (*rest_model.ServiceEdgeRouterPolicyDetail, error)
	GetByName(context.Context, string) (*rest_model.ServiceEdgeRouterPolicyDetail, error)
}

type ServiceEdgeRouterPolicyManager struct {
	client *Client
}

type ServiceEdgeRouterPolicyOptions struct {
	BaseOptions
	ServiceRoles    []string
	EdgeRouterRoles []string
	Semantic        rest_model.Semantic
}

func NewServiceEdgeRouterPolicyManager(client *Client) *ServiceEdgeRouterPolicyManager {
	return &ServiceEdgeRouterPolicyManager{client: client}
}

func (m *ServiceEdgeRouterPolicyManager) Create(ctx context.Context, opts *ServiceEdgeRouterPolicyOptions) (string, error) {
	req := service_edge_router_policy.NewCreateServiceEdgeRouterPolicyParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	req.Policy = &rest_model.ServiceEdgeRouterPolicyCreate{
		EdgeRouterRoles: opts.EdgeRouterRoles,
		Name:            &opts.Name,
		Semantic:        &opts.Semantic,
		ServiceRoles:    opts.ServiceRoles,
		Tags:            opts.tags().ToRESTModel(),
	}
	resp, err := m.client.edge.ServiceEdgeRouterPolicy.CreateServiceEdgeRouterPolicy(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return "", wrapError("service_edge_router_policy", "create", err)
	}
	return resp.Payload.Data.ID, nil
}

func (m *ServiceEdgeRouterPolicyManager) Delete(ctx context.Context, id string) error {
	req := service_edge_router_policy.NewDeleteServiceEdgeRouterPolicyParamsWithContext(ctx)
	req.SetTimeout(m.client.cfg.OperationTimeout)
	req.ID = id
	_, err := m.client.edge.ServiceEdgeRouterPolicy.DeleteServiceEdgeRouterPolicy(req, runtime.ClientAuthInfoWriter(nil))
	return wrapError("service_edge_router_policy", "delete", err)
}

func (m *ServiceEdgeRouterPolicyManager) Find(ctx context.Context, opts *FilterOptions) ([]*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	req := service_edge_router_policy.NewListServiceEdgeRouterPoliciesParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	filter := filterValue(opts)
	req.Filter = &filter
	limit := opts.limit()
	req.Limit = &limit
	offset := opts.offset()
	req.Offset = &offset
	resp, err := m.client.edge.ServiceEdgeRouterPolicy.ListServiceEdgeRouterPolicies(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("service_edge_router_policy", "find", err)
	}
	return resp.Payload.Data, nil
}

func (m *ServiceEdgeRouterPolicyManager) GetByID(ctx context.Context, id string) (*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("id", id)})
	if err != nil {
		return nil, err
	}
	return oneResult("service_edge_router_policy", "get_by_id", id, items)
}

func (m *ServiceEdgeRouterPolicyManager) GetByName(ctx context.Context, name string) (*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("name", name)})
	if err != nil {
		return nil, err
	}
	return oneResult("service_edge_router_policy", "get_by_name", name, items)
}
