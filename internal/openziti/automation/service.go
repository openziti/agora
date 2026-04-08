package automation

import (
	"context"

	"github.com/go-openapi/runtime"
	edge_service "github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_model"
)

type ServiceOperations interface {
	Create(context.Context, *ServiceOptions) (string, error)
	Delete(context.Context, string) error
	Find(context.Context, *FilterOptions) ([]*rest_model.ServiceDetail, error)
	GetByID(context.Context, string) (*rest_model.ServiceDetail, error)
	GetByName(context.Context, string) (*rest_model.ServiceDetail, error)
}

type ServiceManager struct {
	client *Client
}

type ServiceOptions struct {
	BaseOptions
	EncryptionRequired bool
	TerminatorStrategy string
	RoleAttributes     []string
	MaxIdleTimeMillis  *int64
}

func NewServiceManager(client *Client) *ServiceManager {
	return &ServiceManager{client: client}
}

func (m *ServiceManager) Create(ctx context.Context, opts *ServiceOptions) (string, error) {
	req := edge_service.NewCreateServiceParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	service := &rest_model.ServiceCreate{
		EncryptionRequired: &opts.EncryptionRequired,
		Name:               &opts.Name,
		Tags:               opts.tags().ToRESTModel(),
	}
	if len(opts.RoleAttributes) > 0 {
		service.RoleAttributes = opts.RoleAttributes
	}
	if opts.TerminatorStrategy != "" {
		service.TerminatorStrategy = opts.TerminatorStrategy
	}
	if opts.MaxIdleTimeMillis != nil {
		service.MaxIdleTimeMillis = *opts.MaxIdleTimeMillis
	}
	req.Service = service
	resp, err := m.client.edge.Service.CreateService(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return "", wrapError("service", "create", err)
	}
	return resp.Payload.Data.ID, nil
}

func (m *ServiceManager) Delete(ctx context.Context, id string) error {
	req := edge_service.NewDeleteServiceParamsWithContext(ctx)
	req.SetTimeout(m.client.cfg.OperationTimeout)
	req.ID = id
	_, err := m.client.edge.Service.DeleteService(req, runtime.ClientAuthInfoWriter(nil))
	return wrapError("service", "delete", err)
}

func (m *ServiceManager) Find(ctx context.Context, opts *FilterOptions) ([]*rest_model.ServiceDetail, error) {
	req := edge_service.NewListServicesParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	filter := filterValue(opts)
	req.Filter = &filter
	limit := opts.limit()
	req.Limit = &limit
	offset := opts.offset()
	req.Offset = &offset
	resp, err := m.client.edge.Service.ListServices(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("service", "find", err)
	}
	return resp.Payload.Data, nil
}

func (m *ServiceManager) GetByID(ctx context.Context, id string) (*rest_model.ServiceDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("id", id)})
	if err != nil {
		return nil, err
	}
	return oneResult("service", "get_by_id", id, items)
}

func (m *ServiceManager) GetByName(ctx context.Context, name string) (*rest_model.ServiceDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("name", name)})
	if err != nil {
		return nil, err
	}
	return oneResult("service", "get_by_name", name, items)
}
