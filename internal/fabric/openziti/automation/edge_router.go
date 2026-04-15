package automation

import (
	"context"

	"github.com/go-openapi/runtime"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_model"
)

type EdgeRouterOperations interface {
	Find(context.Context, *FilterOptions) ([]*rest_model.EdgeRouterDetail, error)
}

type EdgeRouterManager struct {
	client *Client
}

func NewEdgeRouterManager(client *Client) *EdgeRouterManager {
	return &EdgeRouterManager{client: client}
}

func (m *EdgeRouterManager) Find(ctx context.Context, opts *FilterOptions) ([]*rest_model.EdgeRouterDetail, error) {
	req := edge_router.NewListEdgeRoutersParamsWithContext(ctx)
	req.SetTimeout(opts.timeout(m.client.cfg.RequestTimeout))
	filter := filterValue(opts)
	req.Filter = &filter
	limit := opts.limit()
	req.Limit = &limit
	offset := opts.offset()
	req.Offset = &offset
	resp, err := m.client.edge.EdgeRouter.ListEdgeRouters(req, runtime.ClientAuthInfoWriter(nil))
	if err != nil {
		return nil, wrapError("edge_router", "find", err)
	}
	return resp.Payload.Data, nil
}
