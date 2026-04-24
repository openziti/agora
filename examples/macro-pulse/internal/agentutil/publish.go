// Package agentutil contains small shared helpers used by the Macro
// Pulse demo agents to keep their main.go files focused on the
// agent-specific business logic.
package agentutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openziti/agora/internal/api"
)

// AdvertisementSpec describes an advertisement an agent wants to
// publish. Workgroups are referenced by name and resolved via
// ListWorkgroups before publish.
type AdvertisementSpec struct {
	Name                string
	Description         string
	Capabilities        []api.AdvertisementCapability
	InteractionPatterns []api.AdvertisementInteractionPattern
	WorkgroupNames      []string
	TunnelMode          api.AdvertisementTunnelMode // optional; defaults to "tcp" on the controller
}

// EnsureAdvertisement publishes the advertisement, or — when the
// advertisement name is already in use by the caller — looks up the
// existing record and returns it. This makes provider/tool agent
// startup idempotent against repeated runs.
func EnsureAdvertisement(ctx context.Context, client *api.Client, spec AdvertisementSpec) (*api.Advertisement, error) {
	scopes, err := resolveWorkgroupNames(ctx, client, spec.WorkgroupNames)
	if err != nil {
		return nil, err
	}

	req := &api.PublishAdvertisementRequest{
		Name:                spec.Name,
		Capabilities:        spec.Capabilities,
		InteractionPatterns: spec.InteractionPatterns,
		WorkgroupScopes:     scopes,
	}
	if spec.Description != "" {
		req.Description.SetTo(spec.Description)
	}
	if spec.TunnelMode != "" {
		req.TunnelMode.SetTo(spec.TunnelMode)
	}

	res, err := client.PublishAdvertisement(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("publish advertisement: %w", err)
	}
	switch typed := res.(type) {
	case *api.Advertisement:
		return typed, nil
	case *api.PublishAdvertisementConflict:
		// Look up the existing advertisement we own with this name.
		existing, lookupErr := findAdvertisementByName(ctx, client, spec.Name)
		if lookupErr != nil {
			return nil, lookupErr
		}
		return existing, nil
	case *api.PublishAdvertisementBadRequest:
		return nil, errors.New("publish advertisement bad_request: " + typed.Message)
	case *api.PublishAdvertisementForbidden:
		return nil, errors.New("publish advertisement forbidden: " + typed.Message)
	case *api.PublishAdvertisementUnauthorized:
		return nil, errors.New("publish advertisement unauthorized: " + typed.Message)
	case *api.PublishAdvertisementInternalServerError:
		return nil, errors.New("publish advertisement internal_error: " + typed.Message)
	default:
		return nil, fmt.Errorf("unexpected publish advertisement response: %T", res)
	}
}

func resolveWorkgroupNames(ctx context.Context, client *api.Client, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, errors.New("no workgroup names supplied")
	}
	res, err := client.ListWorkgroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workgroups: %w", err)
	}
	listing, ok := res.(*api.ListWorkgroupsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected list workgroups response: %T", res)
	}
	byName := make(map[string]string, len(*listing))
	for _, wg := range *listing {
		byName[strings.ToLower(wg.Name)] = wg.ID
	}
	resolved := make([]string, 0, len(names))
	for _, name := range names {
		id, ok := byName[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("workgroup %q not visible to this agent (is its account a member?)", name)
		}
		resolved = append(resolved, id)
	}
	return resolved, nil
}

func findAdvertisementByName(ctx context.Context, client *api.Client, name string) (*api.Advertisement, error) {
	res, err := client.ListAdvertisements(ctx, api.ListAdvertisementsParams{})
	if err != nil {
		return nil, fmt.Errorf("list advertisements: %w", err)
	}
	listing, ok := res.(*api.ListAdvertisementsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected list advertisements response: %T", res)
	}
	for i := range *listing {
		ad := (*listing)[i]
		if strings.EqualFold(ad.Name, name) {
			return &ad, nil
		}
	}
	return nil, fmt.Errorf("advertisement %q not found in caller's own list", name)
}
