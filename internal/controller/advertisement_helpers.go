package controller

import (
	"context"
	"errors"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

var (
	errUnknownWorkgroup       = errors.New("unknown workgroup")
	errNotAWorkgroupMember    = errors.New("not a workgroup member")
	errAdvertisementRetracted = errors.New("advertisement retracted")
)

// requireOwnedAdvertisement returns the advertisement when the caller
// is the owner. Returns persistence.ErrNotFound otherwise (404-leak
// prevention; non-owners learn nothing about whether the advertisement
// exists).
func (s *Service) requireOwnedAdvertisement(ctx context.Context, principal *accountPrincipal, advID string) (*persistence.Advertisement, error) {
	ad, err := s.store.Advertisements.GetByID(ctx, s.store.DB(), advID)
	if err != nil {
		return nil, err
	}
	if ad.AccountID != principal.AccountID || ad.OrganizationID != principal.OrganizationID {
		return nil, persistence.ErrNotFound
	}
	return ad, nil
}

// requireVisibleAdvertisement returns the advertisement plus the
// caller's visible workgroup-scope intersection if the caller is the
// owner or shares any workgroup. Retracted advertisements are visible
// only to the owner. Returns persistence.ErrNotFound otherwise.
func (s *Service) requireVisibleAdvertisement(ctx context.Context, principal *accountPrincipal, advID string) (*persistence.Advertisement, []string, error) {
	ad, err := s.store.Advertisements.GetByID(ctx, s.store.DB(), advID)
	if err != nil {
		return nil, nil, err
	}
	isOwner := ad.AccountID == principal.AccountID
	if ad.Status == persistence.AdvertisementStatusRetracted && !isOwner {
		return nil, nil, persistence.ErrNotFound
	}
	if isOwner {
		return ad, []string(ad.WorkgroupScopes), nil
	}
	memberships, err := s.store.WorkgroupMemberships.ListByAccount(ctx, s.store.DB(), principal.AccountID)
	if err != nil {
		return nil, nil, err
	}
	memberSet := make(map[string]struct{}, len(memberships))
	for _, m := range memberships {
		memberSet[m.WorkgroupID] = struct{}{}
	}
	intersection := make([]string, 0, len(ad.WorkgroupScopes))
	for _, scope := range ad.WorkgroupScopes {
		if _, ok := memberSet[scope]; ok {
			intersection = append(intersection, scope)
		}
	}
	if len(intersection) == 0 {
		return nil, nil, persistence.ErrNotFound
	}
	return ad, intersection, nil
}

// validateWorkgroupScopes verifies every supplied workgroup ID exists
// and the caller holds a non-deleted membership in it. Returns
// errUnknownWorkgroup or errNotAWorkgroupMember on failure.
func (s *Service) validateWorkgroupScopes(ctx context.Context, principal *accountPrincipal, scopes []string) error {
	for _, scope := range scopes {
		wg, err := s.store.Workgroups.GetByID(ctx, s.store.DB(), scope)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return errUnknownWorkgroup
			}
			return err
		}
		_ = wg
		if _, err := s.store.WorkgroupMemberships.GetByWorkgroupAndAccount(ctx, s.store.DB(), scope, principal.AccountID); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return errNotAWorkgroupMember
			}
			return err
		}
	}
	return nil
}

// loadCallerWorkgroupIDs returns the IDs of workgroups the caller is
// a non-deleted member of. Used by catalog search and visibility
// checks.
func (s *Service) loadCallerWorkgroupIDs(ctx context.Context, principal *accountPrincipal) ([]string, error) {
	memberships, err := s.store.WorkgroupMemberships.ListByAccount(ctx, s.store.DB(), principal.AccountID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.WorkgroupID)
	}
	return ids, nil
}

func mapAdvertisement(ad *persistence.Advertisement, visibleScopes []string) *api.Advertisement {
	result := &api.Advertisement{
		ID:                  ad.ID,
		OrganizationId:      ad.OrganizationID,
		AccountId:           ad.AccountID,
		Name:                ad.Name,
		WorkgroupScopes:     append([]string(nil), visibleScopes...),
		SchemaVersion:       ad.SchemaVersion,
		Status:              api.AdvertisementStatus(ad.Status),
		CreatedAt:           ad.CreatedAt,
		UpdatedAt:           ad.UpdatedAt,
		Capabilities:        mapCapabilities(ad.Capabilities),
		InteractionPatterns: mapInteractionPatterns(ad.InteractionPatterns),
	}
	if ad.Description != nil {
		result.Description.SetTo(*ad.Description)
	}
	if ad.RetractedAt != nil {
		result.RetractedAt.SetTo(*ad.RetractedAt)
	}
	return result
}

func mapCapabilities(caps persistence.CapabilitiesJSON) []api.AdvertisementCapability {
	out := make([]api.AdvertisementCapability, 0, len(caps))
	for _, c := range caps {
		ac := api.AdvertisementCapability{Name: c.Name}
		if c.Description != "" {
			ac.Description.SetTo(c.Description)
		}
		if len(c.Metadata) > 0 {
			ac.Metadata.SetTo(api.AdvertisementCapabilityMetadata(c.Metadata))
		}
		out = append(out, ac)
	}
	return out
}

func mapInteractionPatterns(patterns persistence.InteractionPatternsJSON) []api.AdvertisementInteractionPattern {
	out := make([]api.AdvertisementInteractionPattern, 0, len(patterns))
	for _, p := range patterns {
		ip := api.AdvertisementInteractionPattern{Kind: api.AdvertisementInteractionPatternKind(p.Kind)}
		if p.CustomPattern != "" {
			ip.CustomPattern.SetTo(p.CustomPattern)
		}
		out = append(out, ip)
	}
	return out
}

func capabilitiesFromAPI(caps []api.AdvertisementCapability) persistence.CapabilitiesJSON {
	out := make(persistence.CapabilitiesJSON, 0, len(caps))
	for _, c := range caps {
		entry := persistence.Capability{Name: c.Name}
		if c.Description.Set {
			entry.Description = c.Description.Value
		}
		if c.Metadata.Set {
			entry.Metadata = map[string]string(c.Metadata.Value)
		}
		out = append(out, entry)
	}
	return out
}

func interactionPatternsFromAPI(patterns []api.AdvertisementInteractionPattern) persistence.InteractionPatternsJSON {
	out := make(persistence.InteractionPatternsJSON, 0, len(patterns))
	for _, p := range patterns {
		entry := persistence.InteractionPattern{Kind: persistence.InteractionPatternKind(p.Kind)}
		if p.CustomPattern.Set {
			entry.CustomPattern = p.CustomPattern.Value
		}
		out = append(out, entry)
	}
	return out
}

func validateCapabilities(caps []api.AdvertisementCapability) error {
	if len(caps) == 0 {
		return errors.New("capabilities must contain at least one entry")
	}
	for i, c := range caps {
		if strings.TrimSpace(c.Name) == "" {
			return errors.New("capability name must not be empty")
		}
		_ = i
	}
	return nil
}

func validateInteractionPatterns(patterns []api.AdvertisementInteractionPattern) error {
	if len(patterns) == 0 {
		return errors.New("interactionPatterns must contain at least one entry")
	}
	for _, p := range patterns {
		if p.Kind == api.AdvertisementInteractionPatternKindCustom {
			if !p.CustomPattern.Set || strings.TrimSpace(p.CustomPattern.Value) == "" {
				return errors.New("interaction pattern with kind=custom requires a non-empty customPattern")
			}
		}
	}
	return nil
}
