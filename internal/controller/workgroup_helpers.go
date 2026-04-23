package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

// requireVisibleWorkgroup returns the workgroup if the caller can see
// it under the account-token surface, or persistence.ErrNotFound if
// not. Visibility for the account-token surface is membership-based
// (any non-deleted membership the caller holds on the workgroup
// satisfies). Pending and active workgroups follow the same rule;
// declined workgroups are never visible through this surface.
func (s *Service) requireVisibleWorkgroup(ctx context.Context, principal *accountPrincipal, workgroupID string) (*persistence.Workgroup, *persistence.WorkgroupMembership, error) {
	wg, err := s.store.Workgroups.GetByID(ctx, s.store.DB(), workgroupID)
	if err != nil {
		return nil, nil, err
	}
	if wg.State == persistence.WorkgroupStateDeclined {
		return nil, nil, persistence.ErrNotFound
	}
	membership, err := s.store.WorkgroupMemberships.GetByWorkgroupAndAccount(ctx, s.store.DB(), wg.ID, principal.AccountID)
	if err != nil {
		return nil, nil, persistence.ErrNotFound
	}
	return wg, membership, nil
}

// requireWorkgroupAdmin returns the workgroup and the caller's
// membership if the caller is a workgroup admin. Returns
// persistence.ErrNotFound (404 leak prevention) when the caller is
// not a member or the workgroup is not visible. Returns errForbidden
// when the caller is a member but lacks admin role.
func (s *Service) requireWorkgroupAdmin(ctx context.Context, principal *accountPrincipal, workgroupID string) (*persistence.Workgroup, *persistence.WorkgroupMembership, error) {
	wg, membership, err := s.requireVisibleWorkgroup(ctx, principal, workgroupID)
	if err != nil {
		return nil, nil, err
	}
	if membership.Role != persistence.WorkgroupMembershipRoleAdmin {
		return nil, nil, errForbidden
	}
	return wg, membership, nil
}

var errForbidden = fmt.Errorf("forbidden")

// aggregateWorkgroupState computes a workgroup's state from its
// invitations: any-declined → declined, all-accepted → active,
// otherwise pending. Pure function.
func aggregateWorkgroupState(invitations []persistence.WorkgroupInvitation) persistence.WorkgroupState {
	if len(invitations) == 0 {
		return persistence.WorkgroupStateActive
	}
	allAccepted := true
	for _, inv := range invitations {
		if inv.State == persistence.WorkgroupInvitationStateDeclined {
			return persistence.WorkgroupStateDeclined
		}
		if inv.State != persistence.WorkgroupInvitationStateAccepted {
			allAccepted = false
		}
	}
	if allAccepted {
		return persistence.WorkgroupStateActive
	}
	return persistence.WorkgroupStatePending
}

// recomputeWorkgroupState loads invitations within the supplied
// transaction, derives the aggregate state, and updates the workgroup
// row if it changed. Returns the new state.
func (s *Service) recomputeWorkgroupState(ctx context.Context, tx persistence.Queryer, wg *persistence.Workgroup) (persistence.WorkgroupState, error) {
	invitations, err := s.store.WorkgroupInvitations.ListByWorkgroup(ctx, tx, wg.ID)
	if err != nil {
		return "", err
	}
	newState := aggregateWorkgroupState(invitations)
	if newState == wg.State {
		return wg.State, nil
	}
	if err := s.store.Workgroups.UpdateState(ctx, tx, wg.ID, newState); err != nil {
		return "", err
	}
	return newState, nil
}

// participatingOrgIDs derives the inter-org participant list for a
// workgroup from its invitations. Returns an empty slice for
// intra-org workgroups.
func (s *Service) participatingOrgIDs(ctx context.Context, tx persistence.Queryer, wg *persistence.Workgroup) ([]string, error) {
	if wg.Scope != persistence.WorkgroupScopeInterOrg {
		return []string{}, nil
	}
	invitations, err := s.store.WorkgroupInvitations.ListByWorkgroup(ctx, tx, wg.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(invitations))
	for _, inv := range invitations {
		ids = append(ids, inv.OrganizationID)
	}
	return ids, nil
}

func mapWorkgroup(wg *persistence.Workgroup, participatingOrgIDs []string) *api.Workgroup {
	result := &api.Workgroup{
		ID:                           wg.ID,
		OwnerOrganizationId:          wg.OwnerOrganizationID,
		Name:                         wg.Name,
		Scope:                        api.WorkgroupScope(wg.Scope),
		State:                        api.WorkgroupState(wg.State),
		ParticipatingOrganizationIds: participatingOrgIDs,
		CreatedAt:                    wg.CreatedAt,
		UpdatedAt:                    wg.UpdatedAt,
	}
	if wg.Description != nil {
		result.Description.SetTo(*wg.Description)
	}
	return result
}

func mapWorkgroupInvitation(inv *persistence.WorkgroupInvitation) *api.WorkgroupInvitation {
	result := &api.WorkgroupInvitation{
		ID:             inv.ID,
		WorkgroupId:    inv.WorkgroupID,
		OrganizationId: inv.OrganizationID,
		State:          api.WorkgroupInvitationState(inv.State),
		CreatedAt:      inv.CreatedAt,
		UpdatedAt:      inv.UpdatedAt,
	}
	if inv.AcknowledgedByAccountID != nil {
		result.AcknowledgedByAccountId.SetTo(*inv.AcknowledgedByAccountID)
	}
	if inv.AcknowledgedAt != nil {
		result.AcknowledgedAt.SetTo(*inv.AcknowledgedAt)
	}
	return result
}

func mapWorkgroupMembership(m *persistence.WorkgroupMembership, email string) *api.WorkgroupMembership {
	result := &api.WorkgroupMembership{
		ID:             m.ID,
		WorkgroupId:    m.WorkgroupID,
		AccountId:      m.AccountID,
		OrganizationId: m.OrganizationID,
		Role:           api.WorkgroupMembershipRole(m.Role),
		JoinedAt:       m.JoinedAt,
	}
	if email != "" {
		result.Email.SetTo(email)
	}
	return result
}

// loadAccountEmailsForOrg returns a map of accountID → email for the
// given account IDs that belong to the supplied organization. Used
// for membership-list redaction (only same-org accounts get emails).
func (s *Service) loadAccountEmailsForOrg(ctx context.Context, db persistence.Queryer, organizationID string, accountIDs []string) (map[string]string, error) {
	emails := make(map[string]string, len(accountIDs))
	for _, id := range accountIDs {
		acct, err := s.store.Accounts.GetByID(ctx, db, id)
		if err != nil {
			continue
		}
		if acct.OrganizationID != organizationID {
			continue
		}
		emails[id] = acct.Email
	}
	return emails, nil
}

func normalizedWorkgroupName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
