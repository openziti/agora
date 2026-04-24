package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListAdvertisements(ctx context.Context, params api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListAdvertisementsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	statusFilter := persistence.AdvertisementStatus("")
	if params.Status.Set {
		statusFilter = persistence.AdvertisementStatus(params.Status.Value)
	}

	ads, err := s.store.Advertisements.ListByAccount(ctx, s.store.DB(), principal.AccountID, statusFilter)
	if err != nil {
		dl.Errorf("list advertisements failed %s: %v", principalLogFields(principal), err)
		return &api.ListAdvertisementsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	result := make(api.ListAdvertisementsResponse, 0, len(ads))
	for i := range ads {
		ad := ads[i]
		result = append(result, *mapAdvertisement(&ad, []string(ad.WorkgroupScopes)))
	}
	return &result, nil
}
