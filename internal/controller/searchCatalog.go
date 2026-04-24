package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) SearchCatalog(ctx context.Context, params api.SearchCatalogParams) (api.SearchCatalogRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.SearchCatalogUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	callerWorkgroupIDs, err := s.loadCallerWorkgroupIDs(ctx, principal)
	if err != nil {
		dl.Errorf("catalog search workgroup-membership lookup failed %s: %v", principalLogFields(principal), err)
		return &api.SearchCatalogInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	var cursor *persistence.SearchCursor
	if params.Cursor.Set && params.Cursor.Value != "" {
		decoded, err := persistence.DecodeSearchCursor(params.Cursor.Value)
		if err != nil {
			return &api.SearchCatalogBadRequest{Code: "invalid_request", Message: "cursor is no longer valid; restart the search"}, nil
		}
		cursor = decoded
	}

	limit := 0
	if params.Limit.Set {
		limit = params.Limit.Value
	}

	kinds := make([]persistence.InteractionPatternKind, 0, len(params.InteractionPattern))
	for _, k := range params.InteractionPattern {
		kinds = append(kinds, persistence.InteractionPatternKind(k))
	}

	searchParams := persistence.SearchParams{
		CallerAccountID:         principal.AccountID,
		CallerWorkgroupIDs:      callerWorkgroupIDs,
		WorkgroupFilter:         append([]string(nil), params.Workgroup...),
		CapabilityKeyword:       optString(params.Capability),
		InteractionPatternKinds: kinds,
		OwnerOrganizationID:     optString(params.OwnerOrganizationId),
		Cursor:                  cursor,
		Limit:                   limit,
	}

	ads, nextCursor, err := s.store.Advertisements.Search(ctx, s.store.DB(), searchParams)
	if err != nil {
		dl.Errorf("catalog search failed %s: %v", principalLogFields(principal), err)
		return &api.SearchCatalogInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	memberSet := make(map[string]struct{}, len(callerWorkgroupIDs))
	for _, id := range callerWorkgroupIDs {
		memberSet[id] = struct{}{}
	}

	resp := &api.CatalogSearchResponse{Items: make([]api.Advertisement, 0, len(ads))}
	for i := range ads {
		ad := ads[i]
		visible := visibleScopesForCaller(&ad, principal, memberSet)
		resp.Items = append(resp.Items, *mapAdvertisement(&ad, visible))
	}
	if nextCursor != "" {
		resp.NextCursor.SetTo(nextCursor)
	}
	return resp, nil
}

func visibleScopesForCaller(ad *persistence.Advertisement, principal *accountPrincipal, memberSet map[string]struct{}) []string {
	if ad.AccountID == principal.AccountID {
		return []string(ad.WorkgroupScopes)
	}
	intersection := make([]string, 0, len(ad.WorkgroupScopes))
	for _, scope := range ad.WorkgroupScopes {
		if _, ok := memberSet[scope]; ok {
			intersection = append(intersection, scope)
		}
	}
	return intersection
}

func optString(v api.OptString) string {
	if v.Set {
		return v.Value
	}
	return ""
}
