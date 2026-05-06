package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetDashboardActivity(ctx context.Context, params api.GetDashboardActivityParams) (api.GetDashboardActivityRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetDashboardActivityUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	window, err := dashboardWindowDuration(params.Window.Or(api.DashboardWindow24h))
	if err != nil {
		return &api.GetDashboardActivityBadRequest{Code: "invalid_request", Message: err.Error()}, nil
	}
	bucket, err := dashboardBucketDuration(params.Bucket.Or(api.DashboardBucket1h))
	if err != nil {
		return &api.GetDashboardActivityBadRequest{Code: "invalid_request", Message: err.Error()}, nil
	}
	if bucket >= window {
		return &api.GetDashboardActivityBadRequest{Code: "invalid_request", Message: "bucket must be smaller than window"}, nil
	}

	buckets, err := persistence.EnvelopeBuckets(ctx, s.store.DB(), principal.OrganizationID, window, bucket)
	if err != nil {
		dl.Errorf("get dashboard activity buckets failed %s: %v", principalLogFields(principal), err)
		return &api.GetDashboardActivityInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	byWorkgroup, err := persistence.EnvelopesByWorkgroup(ctx, s.store.DB(), principal.OrganizationID, window, dashboardTopWorkgroupsLimit)
	if err != nil {
		dl.Errorf("get dashboard activity workgroup breakdown failed %s: %v", principalLogFields(principal), err)
		return &api.GetDashboardActivityInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.DashboardActivityResponse{
		Buckets:     mapDashboardBuckets(buckets),
		ByWorkgroup: mapDashboardWorkgroupActivity(byWorkgroup),
	}, nil
}
