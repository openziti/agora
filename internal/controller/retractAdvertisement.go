package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) RetractAdvertisement(ctx context.Context, params api.RetractAdvertisementParams) (api.RetractAdvertisementRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.RetractAdvertisementUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	ad, err := s.requireOwnedAdvertisement(ctx, principal, params.AdvertisementId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.RetractAdvertisementNotFound{Code: "not_found", Message: "advertisement not found"}, nil
		}
		return &api.RetractAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	if err := s.store.Advertisements.MarkRetracted(ctx, s.store.DB(), ad.ID); err != nil {
		dl.Errorf("retract advertisement failed id='%s' %s: %v", ad.ID, principalLogFields(principal), err)
		return &api.RetractAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("retracted advertisement id='%s' %s", ad.ID, principalLogFields(principal))
	return &api.RetractAdvertisementNoContent{}, nil
}
