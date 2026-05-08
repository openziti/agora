package catalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
)

var (
	workgroupIDPattern = regexp.MustCompile(`^wg_[a-z0-9]{12}$`)
	contractIDPattern  = regexp.MustCompile(`^con_[a-z0-9]{12}$`)
)

type controller interface {
	PublishAdvertisement(context.Context, *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error)
	ListAdvertisements(context.Context, api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error)
	RetractAdvertisement(context.Context, api.RetractAdvertisementParams) (api.RetractAdvertisementRes, error)
}

// EnsurePublished publishes spec or returns the existing same-name advertisement.
//
// The operation is idempotent for repeated agent startup: if the
// controller reports that this account already owns an active
// advertisement with spec.Name, EnsurePublished looks that record up
// and returns it unchanged. It does not reconcile drift between spec
// and the existing record.
func EnsurePublished(ctx context.Context, a *agent.Agent, spec PublishSpec) (*Advertisement, error) {
	if a == nil {
		return nil, badRequest("agent is required")
	}
	return ensurePublished(ctx, a.Controller(), spec)
}

func ensurePublished(ctx context.Context, c controller, spec PublishSpec) (*Advertisement, error) {
	if c == nil {
		return nil, badRequest("controller client is required")
	}
	spec = normalizeSpec(spec)
	if err := validateSpec(spec); err != nil {
		return nil, err
	}

	res, err := c.PublishAdvertisement(ctx, toAPIPublishRequest(spec))
	if err != nil {
		return nil, fmt.Errorf("publish advertisement: %v: %w", err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.Advertisement:
		return fromAPIAdvertisement(typed), nil
	case *api.PublishAdvertisementConflict:
		existing, lookupErr := lookupOwnedByName(ctx, c, spec.Name, StatusActive)
		if lookupErr != nil {
			if isNotFound(lookupErr) {
				return nil, fmt.Errorf("publish advertisement '%s' conflicted but not found on re-list: %w", spec.Name, ErrConflict)
			}
			return nil, lookupErr
		}
		return existing, nil
	case *api.PublishAdvertisementBadRequest:
		return nil, typedError("publish advertisement", typed.Message, ErrBadRequest)
	case *api.PublishAdvertisementUnauthorized:
		return nil, typedError("publish advertisement", typed.Message, ErrUnauthorized)
	case *api.PublishAdvertisementForbidden:
		return nil, typedError("publish advertisement", typed.Message, ErrForbidden)
	case *api.PublishAdvertisementInternalServerError:
		return nil, typedError("publish advertisement", typed.Message, ErrTransient)
	default:
		return nil, fmt.Errorf("publish advertisement: unexpected response %T: %w", res, ErrTransient)
	}
}

// Retract removes a previously published advertisement owned by the caller.
//
// Retract treats both a successful controller retraction and an already
// missing advertisement as terminal success, so shutdown paths can call
// it without tracking whether another actor already retracted the record.
func Retract(ctx context.Context, a *agent.Agent, advertisementID string) error {
	if a == nil {
		return badRequest("agent is required")
	}
	return retract(ctx, a.Controller(), advertisementID)
}

func retract(ctx context.Context, c controller, advertisementID string) error {
	if c == nil {
		return badRequest("controller client is required")
	}
	if strings.TrimSpace(advertisementID) == "" {
		return badRequest("advertisement id is required")
	}

	res, err := c.RetractAdvertisement(ctx, api.RetractAdvertisementParams{AdvertisementId: advertisementID})
	if err != nil {
		return fmt.Errorf("retract advertisement: %v: %w", err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.RetractAdvertisementNoContent, *api.RetractAdvertisementNotFound:
		return nil
	case *api.RetractAdvertisementUnauthorized:
		return typedError("retract advertisement", typed.Message, ErrUnauthorized)
	case *api.RetractAdvertisementForbidden:
		return typedError("retract advertisement", typed.Message, ErrForbidden)
	case *api.RetractAdvertisementInternalServerError:
		return typedError("retract advertisement", typed.Message, ErrTransient)
	default:
		return fmt.Errorf("retract advertisement: unexpected response %T: %w", res, ErrTransient)
	}
}

// Lookup returns the caller-owned advertisement with name.
func Lookup(ctx context.Context, a *agent.Agent, name string) (*Advertisement, error) {
	if a == nil {
		return nil, badRequest("agent is required")
	}
	return lookup(ctx, a.Controller(), name)
}

func lookup(ctx context.Context, c controller, name string) (*Advertisement, error) {
	if c == nil {
		return nil, badRequest("controller client is required")
	}
	return lookupOwnedByName(ctx, c, name, "")
}

func lookupOwnedByName(ctx context.Context, c controller, name string, status AdvertisementStatus) (*Advertisement, error) {
	if strings.TrimSpace(name) == "" {
		return nil, badRequest("advertisement name is required")
	}
	ads, err := list(ctx, c, status)
	if err != nil {
		return nil, err
	}
	for i := range ads {
		if strings.EqualFold(ads[i].Name, name) {
			return &ads[i], nil
		}
	}
	return nil, fmt.Errorf("advertisement '%s' not found: %w", name, ErrNotFound)
}

// List returns caller-owned advertisements, optionally filtered by status.
//
// Pass StatusActive or StatusRetracted to filter by status. Pass the
// empty string to return every caller-owned advertisement regardless
// of status.
func List(ctx context.Context, a *agent.Agent, status AdvertisementStatus) ([]Advertisement, error) {
	if a == nil {
		return nil, badRequest("agent is required")
	}
	return list(ctx, a.Controller(), status)
}

func list(ctx context.Context, c controller, status AdvertisementStatus) ([]Advertisement, error) {
	if c == nil {
		return nil, badRequest("controller client is required")
	}
	if status != "" && status != StatusActive && status != StatusRetracted {
		return nil, badRequest("invalid advertisement status")
	}

	params := api.ListAdvertisementsParams{}
	if status != "" {
		params.Status.SetTo(api.AdvertisementStatus(status))
	}
	res, err := c.ListAdvertisements(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list advertisements: %v: %w", err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.ListAdvertisementsResponse:
		out := make([]Advertisement, 0, len(*typed))
		for i := range *typed {
			out = append(out, *fromAPIAdvertisement(&(*typed)[i]))
		}
		return out, nil
	case *api.ListAdvertisementsUnauthorized:
		return nil, typedError("list advertisements", typed.Message, ErrUnauthorized)
	case *api.ListAdvertisementsInternalServerError:
		return nil, typedError("list advertisements", typed.Message, ErrTransient)
	default:
		return nil, fmt.Errorf("list advertisements: unexpected response %T: %w", res, ErrTransient)
	}
}

func normalizeSpec(spec PublishSpec) PublishSpec {
	if len(spec.InteractionPatterns) == 0 {
		spec.InteractionPatterns = []InteractionPattern{{Kind: InteractionRequestResponse}}
	}
	return spec
}

func validateSpec(spec PublishSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return badRequest("advertisement name is required")
	}
	if len(spec.Capabilities) == 0 {
		return badRequest("capabilities must contain at least one entry")
	}
	for _, capability := range spec.Capabilities {
		if strings.TrimSpace(capability.Name) == "" {
			return badRequest("capability name is required")
		}
	}
	if len(spec.InteractionPatterns) == 0 {
		return badRequest("interaction patterns must contain at least one entry")
	}
	for _, pattern := range spec.InteractionPatterns {
		switch pattern.Kind {
		case InteractionRequestResponse, InteractionStream, InteractionBroadcast:
		case InteractionCustom:
			if strings.TrimSpace(pattern.CustomPattern) == "" {
				return badRequest("custom interaction pattern requires customPattern")
			}
		default:
			return badRequest("invalid interaction pattern")
		}
	}
	if len(spec.WorkgroupScopeIDs) == 0 {
		return badRequest("workgroup scope ids must contain at least one entry")
	}
	for _, scope := range spec.WorkgroupScopeIDs {
		if !workgroupIDPattern.MatchString(scope) {
			return badRequest("invalid workgroup scope id")
		}
	}
	switch spec.TunnelMode {
	case "", TunnelTCP, TunnelHTTP, TunnelUDP:
	default:
		return badRequest("invalid tunnel mode")
	}
	if spec.ContractID != "" && !contractIDPattern.MatchString(spec.ContractID) {
		return badRequest("invalid contract id")
	}
	return nil
}

func toAPIPublishRequest(spec PublishSpec) *api.PublishAdvertisementRequest {
	req := &api.PublishAdvertisementRequest{
		Name:                strings.TrimSpace(spec.Name),
		Capabilities:        toAPICapabilities(spec.Capabilities),
		InteractionPatterns: toAPIInteractionPatterns(spec.InteractionPatterns),
		WorkgroupScopes:     append([]string(nil), spec.WorkgroupScopeIDs...),
	}
	if spec.Description != "" {
		req.Description.SetTo(spec.Description)
	}
	if spec.TunnelMode != "" {
		req.TunnelMode.SetTo(api.AdvertisementTunnelMode(spec.TunnelMode))
	}
	if spec.ContractID != "" {
		req.ContractId.SetTo(spec.ContractID)
	}
	return req
}

func toAPICapabilities(caps []Capability) []api.AdvertisementCapability {
	out := make([]api.AdvertisementCapability, 0, len(caps))
	for _, c := range caps {
		entry := api.AdvertisementCapability{Name: strings.TrimSpace(c.Name)}
		if c.Description != "" {
			entry.Description.SetTo(c.Description)
		}
		if len(c.Metadata) > 0 {
			entry.Metadata.SetTo(api.AdvertisementCapabilityMetadata(cloneStringMap(c.Metadata)))
		}
		out = append(out, entry)
	}
	return out
}

func toAPIInteractionPatterns(patterns []InteractionPattern) []api.AdvertisementInteractionPattern {
	out := make([]api.AdvertisementInteractionPattern, 0, len(patterns))
	for _, pattern := range patterns {
		entry := api.AdvertisementInteractionPattern{Kind: api.AdvertisementInteractionPatternKind(pattern.Kind)}
		if pattern.CustomPattern != "" {
			entry.CustomPattern.SetTo(pattern.CustomPattern)
		}
		out = append(out, entry)
	}
	return out
}

func fromAPIAdvertisement(ad *api.Advertisement) *Advertisement {
	if ad == nil {
		return nil
	}
	out := &Advertisement{
		ID:                  ad.ID,
		OrganizationID:      ad.OrganizationId,
		OrganizationName:    ad.OrganizationName,
		AccountID:           ad.AccountId,
		Name:                ad.Name,
		Capabilities:        fromAPICapabilities(ad.Capabilities),
		InteractionPatterns: fromAPIInteractionPatterns(ad.InteractionPatterns),
		WorkgroupScopeIDs:   append([]string(nil), ad.WorkgroupScopes...),
		SchemaVersion:       ad.SchemaVersion,
		Status:              AdvertisementStatus(ad.Status),
		CreatedAt:           ad.CreatedAt,
		UpdatedAt:           ad.UpdatedAt,
	}
	if ad.Description.Set {
		out.Description = ad.Description.Value
	}
	if ad.TunnelMode.Set {
		out.TunnelMode = TunnelMode(ad.TunnelMode.Value)
	}
	if ad.ContractId.Set {
		out.ContractID = ad.ContractId.Value
	}
	if ad.RetractedAt.Set {
		out.RetractedAt = ad.RetractedAt.Value
	}
	return out
}

func fromAPICapabilities(caps []api.AdvertisementCapability) []Capability {
	out := make([]Capability, 0, len(caps))
	for _, c := range caps {
		entry := Capability{Name: c.Name}
		if c.Description.Set {
			entry.Description = c.Description.Value
		}
		if c.Metadata.Set {
			entry.Metadata = cloneStringMap(map[string]string(c.Metadata.Value))
		}
		out = append(out, entry)
	}
	return out
}

func fromAPIInteractionPatterns(patterns []api.AdvertisementInteractionPattern) []InteractionPattern {
	out := make([]InteractionPattern, 0, len(patterns))
	for _, pattern := range patterns {
		entry := InteractionPattern{Kind: InteractionPatternKind(pattern.Kind)}
		if pattern.CustomPattern.Set {
			entry.CustomPattern = pattern.CustomPattern.Value
		}
		out = append(out, entry)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func badRequest(message string) error {
	return typedError("catalog", message, ErrBadRequest)
}

func typedError(operation, message string, sentinel error) error {
	if message == "" {
		return fmt.Errorf("%s: %w", operation, sentinel)
	}
	return fmt.Errorf("%s: %s: %w", operation, message, sentinel)
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
