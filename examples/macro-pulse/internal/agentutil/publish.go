// Package agentutil contains small shared helpers used by the Macro
// Pulse demo agents to keep their main.go files focused on the
// agent-specific business logic.
package agentutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/df/dd"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/catalog"
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
	// Contract attaches a contract (by name within the caller's own
	// contracts, or by con_... id) to the advertisement. Optional.
	Contract *ContractSpec
}

// ContractSpec describes a contract to ensure-exists and reference
// from the advertisement. Names are unique within the owning account.
type ContractSpec struct {
	Name                   string
	Description            string
	MaxDurationSeconds     int
	MaxEnvelopeCount       int
	MaxEnvelopeBytes       int
	AllowedMessageTypes    []string
	RequiredWorkgroupNames []string
	MinAccountAgeDays      int
	AccessMode             api.ContractAccessMode
}

type AgentConfig struct {
	Contract   string
	ContractID string
}

// EnsureAdvertisement publishes the advertisement, or — when the
// advertisement name is already in use by the caller — looks up the
// existing record and returns it. This makes provider/tool agent
// startup idempotent against repeated runs.
func EnsureAdvertisement(ctx context.Context, a *agent.Agent, spec AdvertisementSpec) (*catalog.Advertisement, error) {
	if a == nil {
		return nil, errors.New("agent is required")
	}
	client := a.Controller()
	if err := applyAgentConfig(&spec); err != nil {
		return nil, err
	}

	scopes, err := resolveWorkgroupNames(ctx, client, spec.WorkgroupNames)
	if err != nil {
		return nil, err
	}

	contractID := ""
	if spec.Contract != nil {
		contract, err := ensureContract(ctx, client, *spec.Contract)
		if err != nil {
			return nil, err
		}
		contractID = contract.ID
	}

	return catalog.EnsurePublished(ctx, a, catalog.PublishSpec{
		Name:                spec.Name,
		Description:         spec.Description,
		Capabilities:        capabilitiesToCatalog(spec.Capabilities),
		InteractionPatterns: interactionPatternsToCatalog(spec.InteractionPatterns),
		WorkgroupScopeIDs:   scopes,
		TunnelMode:          catalog.TunnelMode(spec.TunnelMode),
		ContractID:          contractID,
	})
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

// ensureContract looks up a contract by name in the caller's own
// contracts list; if absent, creates it. Idempotent against repeated
// agent startup.
func ensureContract(ctx context.Context, client *api.Client, spec ContractSpec) (*api.Contract, error) {
	if spec.Name == "" {
		return nil, errors.New("contract spec requires a name")
	}
	listRes, err := client.ListContracts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list contracts: %w", err)
	}
	listing, ok := listRes.(*api.ListContractsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected list contracts response: %T", listRes)
	}
	for i := range *listing {
		c := (*listing)[i]
		if strings.EqualFold(c.Name, spec.Name) {
			return &c, nil
		}
	}

	var wgIDs []string
	if len(spec.RequiredWorkgroupNames) > 0 {
		var err error
		wgIDs, err = resolveWorkgroupNames(ctx, client, spec.RequiredWorkgroupNames)
		if err != nil {
			return nil, err
		}
	}

	req := &api.CreateContractRequest{Name: spec.Name}
	if spec.Description != "" {
		req.Description.SetTo(spec.Description)
	}
	if spec.MaxDurationSeconds > 0 {
		req.MaxDurationSeconds.SetTo(spec.MaxDurationSeconds)
	}
	if spec.MaxEnvelopeCount > 0 {
		req.MaxEnvelopeCount.SetTo(spec.MaxEnvelopeCount)
	}
	if spec.MaxEnvelopeBytes > 0 {
		req.MaxEnvelopeBytes.SetTo(spec.MaxEnvelopeBytes)
	}
	if len(spec.AllowedMessageTypes) > 0 {
		req.AllowedMessageTypes = append([]string{}, spec.AllowedMessageTypes...)
	}
	if len(wgIDs) > 0 {
		req.RequiredWorkgroupMemberships = wgIDs
	}
	if spec.MinAccountAgeDays > 0 {
		req.MaturityRequirements.SetTo(api.MaturityRequirements{
			MinAccountAgeDays: api.NewOptInt(spec.MinAccountAgeDays),
		})
	}
	if spec.AccessMode != "" {
		req.AccessMode.SetTo(spec.AccessMode)
	}

	res, err := client.CreateContract(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create contract: %w", err)
	}
	switch typed := res.(type) {
	case *api.Contract:
		return typed, nil
	case *api.CreateContractConflict:
		// race: another process created it in the meantime; look up again.
		again, err := client.ListContracts(ctx)
		if err != nil {
			return nil, fmt.Errorf("list contracts after conflict: %w", err)
		}
		listing2, ok := again.(*api.ListContractsResponse)
		if !ok {
			return nil, fmt.Errorf("unexpected list contracts response: %T", again)
		}
		for i := range *listing2 {
			c := (*listing2)[i]
			if strings.EqualFold(c.Name, spec.Name) {
				return &c, nil
			}
		}
		return nil, fmt.Errorf("contract %q conflicted but not found on re-list", spec.Name)
	case *api.CreateContractBadRequest:
		return nil, errors.New("create contract bad_request: " + typed.Message)
	case *api.CreateContractForbidden:
		return nil, errors.New("create contract forbidden: " + typed.Message)
	case *api.CreateContractUnauthorized:
		return nil, errors.New("create contract unauthorized: " + typed.Message)
	case *api.CreateContractInternalServerError:
		return nil, errors.New("create contract internal_error: " + typed.Message)
	default:
		return nil, fmt.Errorf("unexpected create contract response: %T", res)
	}
}

func applyAgentConfig(spec *AdvertisementSpec) error {
	if spec == nil || spec.Contract == nil {
		return nil
	}
	cfg, err := loadAgentConfig()
	if err != nil {
		return err
	}
	if cfg == nil || cfg.Contract == "" {
		return nil
	}
	spec.Contract.Name = cfg.Contract
	return nil
}

func loadAgentConfig() (*AgentConfig, error) {
	root := os.Getenv("AGORA_ENV_ROOT")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".agora")
	}
	path := filepath.Join(root, "agent-config.yaml")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	cfg := &AgentConfig{}
	if err := dd.BindYAMLFile(cfg, path); err != nil {
		return nil, fmt.Errorf("load agent config %q: %w", path, err)
	}
	return cfg, nil
}

func capabilitiesToCatalog(caps []api.AdvertisementCapability) []catalog.Capability {
	out := make([]catalog.Capability, 0, len(caps))
	for _, c := range caps {
		entry := catalog.Capability{Name: c.Name}
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

func interactionPatternsToCatalog(patterns []api.AdvertisementInteractionPattern) []catalog.InteractionPattern {
	out := make([]catalog.InteractionPattern, 0, len(patterns))
	for _, p := range patterns {
		entry := catalog.InteractionPattern{Kind: catalog.InteractionPatternKind(p.Kind)}
		if p.CustomPattern.Set {
			entry.CustomPattern = p.CustomPattern.Value
		}
		out = append(out, entry)
	}
	return out
}
