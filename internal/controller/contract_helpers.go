package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

var (
	errUnknownContract       = errors.New("unknown contract")
	errContractInUse         = errors.New("contract in use")
	errContractViolation     = errors.New("contract violation")
	errContractViolationMemb = errors.New("contract violation: memberships")
	errContractViolationMat  = errors.New("contract violation: maturity")
)

// requireOwnedContract returns the contract when the caller is the owner.
// Returns persistence.ErrNotFound for non-owners (404-leak prevention).
func (s *Service) requireOwnedContract(ctx context.Context, principal *accountPrincipal, contractID string) (*persistence.Contract, error) {
	c, err := s.store.Contracts.GetByID(ctx, s.store.DB(), contractID)
	if err != nil {
		return nil, err
	}
	if c.AccountID != principal.AccountID || c.OrganizationID != principal.OrganizationID {
		return nil, persistence.ErrNotFound
	}
	return c, nil
}

// requireVisibleContract returns the contract if the caller is the
// owner or shares a workgroup with at least one active advertisement
// that references it. Returns ErrNotFound otherwise.
func (s *Service) requireVisibleContract(ctx context.Context, principal *accountPrincipal, contractID string) (*persistence.Contract, error) {
	c, err := s.store.Contracts.GetByID(ctx, s.store.DB(), contractID)
	if err != nil {
		return nil, err
	}
	if c.AccountID == principal.AccountID {
		return c, nil
	}
	// Non-owner: visible iff at least one active advertisement referencing
	// this contract has a workgroup the caller holds.
	memberships, err := s.loadCallerWorkgroupIDs(ctx, principal)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, persistence.ErrNotFound
	}
	// Search active advertisements that reference this contract and share
	// at least one workgroup scope with the caller's memberships.
	const q = `
select 1 from advertisements
where contract_id = $1 and status = 'active' and workgroup_scopes && $2
limit 1`
	var one int
	if err := s.store.DB().GetContext(ctx, &one, q, contractID, pq.StringArray(memberships)); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

// validateContractOwnership verifies the contract exists and is owned
// by the caller — used by advertisement publish/update when a contractId
// is supplied.
func (s *Service) validateContractOwnership(ctx context.Context, principal *accountPrincipal, contractID string) error {
	if contractID == "" {
		return nil
	}
	c, err := s.store.Contracts.GetByID(ctx, s.store.DB(), contractID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return errUnknownContract
		}
		return err
	}
	if c.AccountID != principal.AccountID {
		return errUnknownContract
	}
	return nil
}

// evaluateContractAdmission checks consumer-side admission terms on a
// contract snapshot against the proposed session. Returns a sentinel
// error per the specific violation dimension; callers map to 403 codes.
func (s *Service) evaluateContractAdmission(ctx context.Context, consumer *persistence.Account, contract *persistence.Contract) error {
	// required_workgroup_memberships
	if len(contract.RequiredWorkgroupMemberships) > 0 {
		memberships, err := s.store.WorkgroupMemberships.ListByAccount(ctx, s.store.DB(), consumer.ID)
		if err != nil {
			return err
		}
		have := make(map[string]struct{}, len(memberships))
		for _, m := range memberships {
			have[m.WorkgroupID] = struct{}{}
		}
		for _, required := range contract.RequiredWorkgroupMemberships {
			if _, ok := have[required]; !ok {
				return errContractViolationMemb
			}
		}
	}
	// maturity_requirements.min_account_age_days
	minAge := contract.MaturityRequirements.MinAccountAgeDays
	if minAge > 0 {
		ageThreshold := time.Now().UTC().Add(-time.Duration(minAge) * 24 * time.Hour)
		if consumer.CreatedAt.After(ageThreshold) {
			return errContractViolationMat
		}
	}
	return nil
}

// snapshotContract produces the frozen jsonb payload written to
// sessions.contract_snapshot at accept time.
func snapshotContract(c *persistence.Contract) ([]byte, error) {
	snap := persistence.ContractSnapshot{
		ContractID:                   c.ID,
		Name:                         c.Name,
		SchemaVersion:                c.SchemaVersion,
		MaxDurationSeconds:           c.MaxDurationSeconds,
		MaxEnvelopeCount:             c.MaxEnvelopeCount,
		MaxEnvelopeBytes:             c.MaxEnvelopeBytes,
		AllowedMessageTypes:          []string(c.AllowedMessageTypes),
		RequiredWorkgroupMemberships: []string(c.RequiredWorkgroupMemberships),
		MaturityRequirements:         persistence.MaturityRequirements(c.MaturityRequirements),
		AccessMode:                   c.AccessMode,
		SnapshottedAt:                time.Now().UTC(),
	}
	if c.Description != nil {
		snap.Description = *c.Description
	}
	return json.Marshal(snap)
}

func mapContract(c *persistence.Contract) *api.Contract {
	out := &api.Contract{
		ID:                           c.ID,
		AccountId:                    c.AccountID,
		OrganizationId:               c.OrganizationID,
		Name:                         c.Name,
		SchemaVersion:                c.SchemaVersion,
		MaxDurationSeconds:           c.MaxDurationSeconds,
		MaxEnvelopeCount:             c.MaxEnvelopeCount,
		MaxEnvelopeBytes:             c.MaxEnvelopeBytes,
		AllowedMessageTypes:          append([]string{}, []string(c.AllowedMessageTypes)...),
		RequiredWorkgroupMemberships: append([]string{}, []string(c.RequiredWorkgroupMemberships)...),
		AccessMode:                   api.ContractAccessMode(c.AccessMode),
		CreatedAt:                    c.CreatedAt,
		UpdatedAt:                    c.UpdatedAt,
	}
	if c.Description != nil {
		out.Description.SetTo(*c.Description)
	}
	if c.MaturityRequirements.MinAccountAgeDays > 0 {
		out.MaturityRequirements.SetTo(api.MaturityRequirements{
			MinAccountAgeDays: api.NewOptInt(c.MaturityRequirements.MinAccountAgeDays),
		})
	}
	return out
}

// mapSessionSnapshot decodes the jsonb bytes into the API shape.
func mapSessionSnapshot(raw []byte) (api.OptContractSnapshot, error) {
	out := api.OptContractSnapshot{}
	if len(raw) == 0 {
		return out, nil
	}
	var snap persistence.ContractSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return out, fmt.Errorf("decode contract snapshot: %w", err)
	}
	apiSnap := api.ContractSnapshot{
		ContractId:                   snap.ContractID,
		Name:                         snap.Name,
		SchemaVersion:                snap.SchemaVersion,
		MaxDurationSeconds:           snap.MaxDurationSeconds,
		MaxEnvelopeCount:             snap.MaxEnvelopeCount,
		MaxEnvelopeBytes:             snap.MaxEnvelopeBytes,
		AllowedMessageTypes:          append([]string{}, snap.AllowedMessageTypes...),
		RequiredWorkgroupMemberships: append([]string{}, snap.RequiredWorkgroupMemberships...),
		AccessMode:                   api.ContractAccessMode(snap.AccessMode),
		SnapshottedAt:                snap.SnapshottedAt,
	}
	if snap.Description != "" {
		apiSnap.Description.SetTo(snap.Description)
	}
	if snap.MaturityRequirements.MinAccountAgeDays > 0 {
		apiSnap.MaturityRequirements.SetTo(api.MaturityRequirements{
			MinAccountAgeDays: api.NewOptInt(snap.MaturityRequirements.MinAccountAgeDays),
		})
	}
	out.SetTo(apiSnap)
	return out, nil
}

func sanitizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
