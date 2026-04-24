package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type ContractsRepository struct{}

const contractColumns = `id, account_id, organization_id, name, description,
    schema_version, max_duration_seconds, max_envelope_count,
    allowed_message_types, required_workgroup_memberships,
    maturity_requirements, access_mode, created_at, updated_at`

// Create inserts a new contract. Generates ID if empty.
func (r *ContractsRepository) Create(ctx context.Context, db Queryer, c Contract) (*Contract, error) {
	if c.ID == "" {
		c.ID = NewResourceID(PrefixContract)
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.SchemaVersion == 0 {
		c.SchemaVersion = 1
	}
	if c.AccessMode == "" {
		c.AccessMode = ContractAccessModeApprovalRequired
	}
	if c.AllowedMessageTypes == nil {
		c.AllowedMessageTypes = pq.StringArray{}
	}
	if c.RequiredWorkgroupMemberships == nil {
		c.RequiredWorkgroupMemberships = pq.StringArray{}
	}

	query := fmt.Sprintf(`
insert into contracts (%s) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) returning %s`, contractColumns, contractColumns)

	var created Contract
	if err := db.GetContext(
		ctx, &created, query,
		c.ID,
		c.AccountID,
		c.OrganizationID,
		strings.TrimSpace(c.Name),
		c.Description,
		c.SchemaVersion,
		c.MaxDurationSeconds,
		c.MaxEnvelopeCount,
		c.AllowedMessageTypes,
		c.RequiredWorkgroupMemberships,
		c.MaturityRequirements,
		c.AccessMode,
		c.CreatedAt,
		c.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create contract: %w", err)
	}
	return &created, nil
}

func (r *ContractsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Contract, error) {
	query := fmt.Sprintf(`select %s from contracts where id = $1`, contractColumns)
	var c Contract
	if err := db.GetContext(ctx, &c, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get contract: %w", err)
	}
	return &c, nil
}

func (r *ContractsRepository) GetByAccountAndName(ctx context.Context, db Queryer, accountID, name string) (*Contract, error) {
	query := fmt.Sprintf(`
select %s
from contracts
where account_id = $1 and lower(name) = lower($2)`, contractColumns)
	var c Contract
	if err := db.GetContext(ctx, &c, query, accountID, strings.TrimSpace(name)); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get contract by account+name: %w", err)
	}
	return &c, nil
}

func (r *ContractsRepository) ListByAccount(ctx context.Context, db Queryer, accountID string) ([]Contract, error) {
	query := fmt.Sprintf(`
select %s
from contracts
where account_id = $1
order by updated_at desc, id asc`, contractColumns)
	var rows []Contract
	if err := db.SelectContext(ctx, &rows, query, accountID); err != nil {
		return nil, fmt.Errorf("list contracts by account: %w", err)
	}
	return rows, nil
}

func (r *ContractsRepository) Update(ctx context.Context, db Queryer, c Contract) (*Contract, error) {
	now := time.Now().UTC()
	c.UpdatedAt = now
	if c.AllowedMessageTypes == nil {
		c.AllowedMessageTypes = pq.StringArray{}
	}
	if c.RequiredWorkgroupMemberships == nil {
		c.RequiredWorkgroupMemberships = pq.StringArray{}
	}

	query := fmt.Sprintf(`
update contracts set
    name = $2,
    description = $3,
    max_duration_seconds = $4,
    max_envelope_count = $5,
    allowed_message_types = $6,
    required_workgroup_memberships = $7,
    maturity_requirements = $8,
    access_mode = $9,
    updated_at = $10
where id = $1
returning %s`, contractColumns)

	var updated Contract
	if err := db.GetContext(
		ctx, &updated, query,
		c.ID,
		strings.TrimSpace(c.Name),
		c.Description,
		c.MaxDurationSeconds,
		c.MaxEnvelopeCount,
		c.AllowedMessageTypes,
		c.RequiredWorkgroupMemberships,
		c.MaturityRequirements,
		c.AccessMode,
		c.UpdatedAt,
	); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update contract: %w", err)
	}
	return &updated, nil
}

// IsReferencedByActiveAdvertisement reports whether any active
// advertisement references the given contract. Used by Delete to block
// removal while in use.
func (r *ContractsRepository) IsReferencedByActiveAdvertisement(ctx context.Context, db Queryer, contractID string) (bool, error) {
	const query = `select exists(select 1 from advertisements where contract_id = $1 and status = 'active')`
	var exists bool
	if err := db.GetContext(ctx, &exists, query, contractID); err != nil {
		return false, fmt.Errorf("check contract in-use: %w", err)
	}
	return exists, nil
}

// Delete removes a contract. Returns ErrNotFound if the contract does
// not exist. Callers are responsible for checking
// IsReferencedByActiveAdvertisement before calling.
func (r *ContractsRepository) Delete(ctx context.Context, db Queryer, id string) error {
	const query = `delete from contracts where id = $1`
	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete contract: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete contract rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
