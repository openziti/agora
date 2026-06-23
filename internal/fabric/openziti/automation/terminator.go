package automation

import (
	"context"

	"github.com/go-openapi/runtime"
	"github.com/openziti/edge-api/rest_management_api_client/terminator"
	"github.com/openziti/edge-api/rest_model"
)

// TerminatorOperations exposes the fabric terminator list/delete primitives takeover and environment
// retirement need. It mirrors the shape of ServicePolicyManager; terminators are the live binding a
// host has on a service, so deleting them is how the controller evicts a host from the fabric.
type TerminatorOperations interface {
	Find(context.Context, *FilterOptions) ([]*rest_model.TerminatorDetail, error)
	Delete(context.Context, string) error
	DeleteWithFilter(context.Context, string) error
}

type TerminatorManager struct {
	client *Client
}

func NewTerminatorManager(client *Client) *TerminatorManager {
	return &TerminatorManager{client: client}
}

func (m *TerminatorManager) Find(ctx context.Context, opts *FilterOptions) ([]*rest_model.TerminatorDetail, error) {
	req := terminator.NewListTerminatorsParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	filter := filterValue(opts)
	req.Filter = &filter
	limit := opts.limit()
	req.Limit = &limit
	offset := opts.offset()
	req.Offset = &offset
	resp, err := m.client.edge.Terminator.ListTerminators(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("terminator", "find", err)
	}
	return resp.Payload.Data, nil
}

func (m *TerminatorManager) Delete(ctx context.Context, id string) error {
	req := terminator.NewDeleteTerminatorParamsWithContext(ctx)
	req.SetTimeout(m.client.cfg.OperationTimeout)
	req.ID = id
	_, err := m.client.edge.Terminator.DeleteTerminator(req, runtime.ClientAuthInfoWriter(nil))
	return wrapError("terminator", "delete", err)
}

func (m *TerminatorManager) DeleteWithFilter(ctx context.Context, filter string) error {
	return deleteWithFilter(ctx, m, filter, func(item *rest_model.TerminatorDetail) string {
		if item.ID == nil {
			return ""
		}
		return *item.ID
	})
}
