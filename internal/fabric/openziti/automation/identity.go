package automation

import (
	"context"
	"encoding/json"

	"github.com/go-openapi/runtime"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti/enroll"
)

type IdentityOperations interface {
	Create(context.Context, *IdentityOptions) (string, error)
	Delete(context.Context, string) error
	DeleteWithFilter(context.Context, string) error
	Find(context.Context, *FilterOptions) ([]*rest_model.IdentityDetail, error)
	GetByID(context.Context, string) (*rest_model.IdentityDetail, error)
	GetByName(context.Context, string) (*rest_model.IdentityDetail, error)
	Enroll(context.Context, string) ([]byte, error)
}

type IdentityManager struct {
	client *Client
}

type IdentityOptions struct {
	BaseOptions
	Type           rest_model.IdentityType
	IsAdmin        bool
	RoleAttributes []string
}

func NewIdentityManager(client *Client) *IdentityManager {
	return &IdentityManager{client: client}
}

func (m *IdentityManager) Create(ctx context.Context, opts *IdentityOptions) (string, error) {
	req := identity.NewCreateIdentityParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	req.Identity = &rest_model.IdentityCreate{
		Enrollment:     &rest_model.IdentityCreateEnrollment{Ott: true},
		IsAdmin:        &opts.IsAdmin,
		Name:           &opts.Name,
		RoleAttributes: (*rest_model.Attributes)(&opts.RoleAttributes),
		Tags:           opts.tags().ToRESTModel(),
		Type:           &opts.Type,
	}

	resp, err := m.client.edge.Identity.CreateIdentity(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return "", wrapError("identity", "create", err)
	}
	return resp.Payload.Data.ID, nil
}

func (m *IdentityManager) Delete(ctx context.Context, id string) error {
	req := identity.NewDeleteIdentityParamsWithContext(ctx)
	req.SetTimeout(m.client.cfg.OperationTimeout)
	req.ID = id
	_, err := m.client.edge.Identity.DeleteIdentity(req, runtime.ClientAuthInfoWriter(nil))
	return wrapError("identity", "delete", err)
}

func (m *IdentityManager) Find(ctx context.Context, opts *FilterOptions) ([]*rest_model.IdentityDetail, error) {
	req := identity.NewListIdentitiesParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	filter := filterValue(opts)
	req.Filter = &filter
	limit := opts.limit()
	req.Limit = &limit
	offset := opts.offset()
	req.Offset = &offset

	resp, err := m.client.edge.Identity.ListIdentities(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("identity", "find", err)
	}
	return resp.Payload.Data, nil
}

func (m *IdentityManager) GetByID(ctx context.Context, id string) (*rest_model.IdentityDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("id", id)})
	if err != nil {
		return nil, err
	}
	return oneResult("identity", "get_by_id", id, items)
}

func (m *IdentityManager) GetByName(ctx context.Context, name string) (*rest_model.IdentityDetail, error) {
	items, err := m.Find(ctx, &FilterOptions{Filter: BuildFilter("name", name)})
	if err != nil {
		return nil, err
	}
	return oneResult("identity", "get_by_name", name, items)
}

func (m *IdentityManager) DeleteWithFilter(ctx context.Context, filter string) error {
	return deleteWithFilter(ctx, m, filter, func(item *rest_model.IdentityDetail) string {
		if item.ID == nil {
			return ""
		}
		return *item.ID
	})
}

func (m *IdentityManager) Enroll(ctx context.Context, id string) ([]byte, error) {
	req := identity.NewDetailIdentityParamsWithContext(ctx)
	req.SetTimeout(m.client.cfg.OperationTimeout)
	req.ID = id
	resp, err := m.client.edge.Identity.DetailIdentity(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("identity", "detail", err)
	}

	token, _, err := enroll.ParseToken(resp.GetPayload().Data.Enrollment.Ott.JWT)
	if err != nil {
		return nil, wrapError("identity", "enroll", err)
	}

	config, err := enroll.Enroll(enroll.EnrollmentFlags{
		Token:  token,
		KeyAlg: "RSA",
	})
	if err != nil {
		return nil, wrapError("identity", "enroll", err)
	}

	data, err := json.Marshal(config)
	if err != nil {
		return nil, wrapError("identity", "enroll", err)
	}
	return data, nil
}
