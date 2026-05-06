package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListSessions(ctx context.Context, params api.ListSessionsParams) (api.ListSessionsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListSessionsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	role := "both"
	if params.Role.Set {
		role = string(params.Role.Value)
	}
	if role != "provider" && role != "consumer" && role != "both" {
		return &api.ListSessionsBadRequest{Code: "invalid_request", Message: "role must be provider, consumer, or both"}, nil
	}

	states := make([]persistence.SessionState, 0, len(params.State))
	for _, st := range params.State {
		states = append(states, persistence.SessionState(st))
	}

	lp := persistence.SessionListParams{
		ParticipantAccountID: principal.AccountID,
		RoleFilter:           role,
		States:               states,
	}
	if params.AdvertisementId.Set {
		lp.AdvertisementID = params.AdvertisementId.Value
	}
	if params.Sort.Set {
		switch params.Sort.Value {
		case api.ListSessionsSortProposedAtDesc:
			lp.OrderBy = persistence.SessionListOrderProposedAtDesc
		case api.ListSessionsSortClosedAtDesc:
			lp.OrderBy = persistence.SessionListOrderClosedAtDesc
		default:
			return &api.ListSessionsBadRequest{Code: "invalid_request", Message: "sort must be proposedAtDesc or closedAtDesc"}, nil
		}
	}
	if params.Limit.Set {
		if params.Limit.Value > 200 {
			return &api.ListSessionsBadRequest{Code: "invalid_request", Message: "limit must be 200 or less"}, nil
		}
		lp.Limit = params.Limit.Value
	}

	rows, err := s.store.Sessions.List(ctx, s.store.DB(), lp)
	if err != nil {
		dl.Errorf("list sessions failed %s: %v", principalLogFields(principal), err)
		return &api.ListSessionsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	resp := make(api.ListSessionsResponse, 0, len(rows))
	for i := range rows {
		resp = append(resp, *mapSession(&rows[i], principal.OrganizationID))
	}
	return &resp, nil
}
