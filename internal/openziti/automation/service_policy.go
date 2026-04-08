package automation

import (
	"context"

	"github.com/go-openapi/runtime"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_model"
)

type ServicePolicyOperations interface {
	Create(context.Context, *ServicePolicyOptions) (string, error)
	Delete(context.Context, string) error
	Find(context.Context, *FilterOptions) ([]*rest_model.ServicePolicyDetail, error)
	GetByID(context.Context, string) (*rest_model.ServicePolicyDetail, error)
	GetByName(context.Context, string) (*rest_model.ServicePolicyDetail, error)
}

type ServicePolicyManagerOperations interface {
	ServicePolicyOperations
	CreateBind(context.Context, *ServicePolicyOptions) (string, error)
	CreateDial(context.Context, *ServicePolicyOptions) (string, error)
}

type ServicePolicyManager struct {
	client *Client
}

type ServicePolicyOptions struct {
	BaseOptions
	IdentityRoles []string
	ServiceRoles  []string
	PolicyType    rest_model.DialBind
	Semantic      rest_model.Semantic
}

func NewServicePolicyManager(client *Client) *ServicePolicyManager {
	return &ServicePolicyManager{client: client}
}

func (m *ServicePolicyManager) Create(ctx context.Context, opts *ServicePolicyOptions) (string, error) {
	req := service_policy.NewCreateServicePolicyParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	req.Policy = &rest_model.ServicePolicyCreate{
		IdentityRoles:     opts.IdentityRoles,
		Name:              &opts.Name,
		PostureCheckRoles: []string{},
		Semantic:          &opts.Semantic,
		ServiceRoles:      opts.ServiceRoles,
		Tags:              opts.tags().ToRESTModel(),
		Type:              &opts.PolicyType,
	}
	resp, err := m.client.edge.ServicePolicy.CreateServicePolicy(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return "", wrapError("service_policy", "create", err)
	}
	return resp.Payload.Data.ID, nil
}

func (m *ServicePolicyManager) Delete(ctx context.Context, id string) error {
	req := service_policy.NewDeleteServicePolicyParamsWithContext(ctx)
	req.SetTimeout(m.client.cfg.OperationTimeout)
	req.ID = id
	_, err := m.client.edge.ServicePolicy.DeleteServicePolicy(req, runtime.ClientAuthInfoWriter(nil))
	return wrapError("service_policy", "delete", err)
}

func (m *ServicePolicyManager) Find(ctx context.Context, opts *FilterOptions) ([]*rest_model.ServicePolicyDetail, error) {
	req := service_policy.NewListServicePoliciesParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	filter := filterValue(opts)
	req.Filter = &filter
	limit := opts.limit()
	req.Limit = &limit
	offset := opts.offset()
	req.Offset = &offset
	resp, err := m.client.edge.ServicePolicy.ListServicePolicies(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("service_policy", "find", err)
	}
	return resp.Payload.Data, nil
}

func (m *ServicePolicyManager) GetByID(ctx context.Context, id string) (*rest_model.ServicePolicyDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("id", id)})
	if err != nil {
		return nil, err
	}
	return oneResult("service_policy", "get_by_id", id, items)
}

func (m *ServicePolicyManager) GetByName(ctx context.Context, name string) (*rest_model.ServicePolicyDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("name", name)})
	if err != nil {
		return nil, err
	}
	return oneResult("service_policy", "get_by_name", name, items)
}

func (m *ServicePolicyManager) CreateBind(ctx context.Context, opts *ServicePolicyOptions) (string, error) {
	opts.PolicyType = rest_model.DialBindBind
	return m.Create(ctx, opts)
}

func (m *ServicePolicyManager) CreateDial(ctx context.Context, opts *ServicePolicyOptions) (string, error) {
	opts.PolicyType = rest_model.DialBindDial
	return m.Create(ctx, opts)
}
