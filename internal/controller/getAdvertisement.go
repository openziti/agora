package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetAdvertisement(ctx context.Context, params api.GetAdvertisementParams) (api.GetAdvertisementRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetAdvertisementUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	ad, visible, err := s.requireVisibleAdvertisement(ctx, principal, params.AdvertisementId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.GetAdvertisementNotFound{Code: "not_found", Message: "advertisement not found"}, nil
		}
		dl.Errorf("get advertisement failed advertisement_id='%s' %s: %v", params.AdvertisementId, principalLogFields(principal), err)
		return &api.GetAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return mapAdvertisement(ad, visible), nil
}
