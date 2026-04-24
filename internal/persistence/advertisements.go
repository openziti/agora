package persistence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type AdvertisementsRepository struct{}

// SearchParams holds the inputs to a catalog search query.
type SearchParams struct {
	CallerAccountID         string
	CallerWorkgroupIDs      []string
	WorkgroupFilter         []string
	CapabilityKeyword       string
	InteractionPatternKinds []InteractionPatternKind
	OwnerOrganizationID     string
	Cursor                  *SearchCursor
	Limit                   int
}

// SearchCursor encodes the position of the last returned advertisement
// in the deterministic sort order (updated_at desc, created_at desc, id asc).
type SearchCursor struct {
	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

const searchDefaultLimit = 50
const searchMaxLimit = 200

// EncodeSearchCursor returns the base64-encoded JSON cursor for the
// supplied position. Returns "" for nil.
func EncodeSearchCursor(c *SearchCursor) (string, error) {
	if c == nil {
		return "", nil
	}
	bytes, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// DecodeSearchCursor parses a base64-encoded JSON cursor.
func DecodeSearchCursor(raw string) (*SearchCursor, error) {
	if raw == "" {
		return nil, nil
	}
	bytes, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}
	var c SearchCursor
	if err := json.Unmarshal(bytes, &c); err != nil {
		return nil, fmt.Errorf("unmarshal cursor: %w", err)
	}
	return &c, nil
}

func (r *AdvertisementsRepository) Create(ctx context.Context, db Queryer, ad Advertisement) (*Advertisement, error) {
	if ad.ID == "" {
		ad.ID = NewResourceID(PrefixAdvertisement)
	}
	now := time.Now().UTC()
	ad.CreatedAt = now
	ad.UpdatedAt = now
	if ad.Status == "" {
		ad.Status = AdvertisementStatusActive
	}
	if ad.SchemaVersion == 0 {
		ad.SchemaVersion = 1
	}
	if ad.Capabilities == nil {
		ad.Capabilities = CapabilitiesJSON{}
	}
	if ad.InteractionPatterns == nil {
		ad.InteractionPatterns = InteractionPatternsJSON{}
	}
	if ad.WorkgroupScopes == nil {
		ad.WorkgroupScopes = pq.StringArray{}
	}
	if ad.TunnelMode == "" {
		ad.TunnelMode = TunnelModeTCP
	}

	const query = `
insert into advertisements (
    id, organization_id, account_id, name, description, capabilities, interaction_patterns,
    workgroup_scopes, tunnel_mode, contract_id, schema_version, status, retracted_at, created_at, updated_at
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
returning id, organization_id, account_id, name, description, capabilities, interaction_patterns,
    workgroup_scopes, tunnel_mode, contract_id, schema_version, status, retracted_at, created_at, updated_at`

	var created Advertisement
	if err := db.GetContext(
		ctx,
		&created,
		query,
		ad.ID,
		ad.OrganizationID,
		ad.AccountID,
		strings.TrimSpace(ad.Name),
		ad.Description,
		ad.Capabilities,
		ad.InteractionPatterns,
		ad.WorkgroupScopes,
		ad.TunnelMode,
		ad.ContractID,
		ad.SchemaVersion,
		ad.Status,
		ad.RetractedAt,
		ad.CreatedAt,
		ad.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create advertisement: %w", err)
	}
	return &created, nil
}

func (r *AdvertisementsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Advertisement, error) {
	const query = `
select id, organization_id, account_id, name, description, capabilities, interaction_patterns,
    workgroup_scopes, tunnel_mode, contract_id, schema_version, status, retracted_at, created_at, updated_at
from advertisements
where id = $1`

	var ad Advertisement
	if err := db.GetContext(ctx, &ad, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get advertisement: %w", err)
	}
	return &ad, nil
}

func (r *AdvertisementsRepository) GetByAccountAndName(ctx context.Context, db Queryer, accountID, name string) (*Advertisement, error) {
	const query = `
select id, organization_id, account_id, name, description, capabilities, interaction_patterns,
    workgroup_scopes, tunnel_mode, contract_id, schema_version, status, retracted_at, created_at, updated_at
from advertisements
where account_id = $1 and lower(name) = lower($2) and status = 'active'`

	var ad Advertisement
	if err := db.GetContext(ctx, &ad, query, accountID, strings.TrimSpace(name)); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get advertisement by account+name: %w", err)
	}
	return &ad, nil
}

func (r *AdvertisementsRepository) ListByAccount(ctx context.Context, db Queryer, accountID string, statusFilter AdvertisementStatus) ([]Advertisement, error) {
	clauses := []string{"account_id = $1"}
	args := []any{accountID}
	if statusFilter != "" {
		args = append(args, statusFilter)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	query := fmt.Sprintf(`
select id, organization_id, account_id, name, description, capabilities, interaction_patterns,
    workgroup_scopes, tunnel_mode, contract_id, schema_version, status, retracted_at, created_at, updated_at
from advertisements
where %s
order by updated_at desc, created_at desc, id asc`, strings.Join(clauses, " and "))

	var ads []Advertisement
	if err := db.SelectContext(ctx, &ads, query, args...); err != nil {
		return nil, fmt.Errorf("list advertisements by account: %w", err)
	}
	return ads, nil
}

// Search executes the catalog query: visibility-enforced + filtered.
// Returns up to params.Limit results (defaulting to searchDefaultLimit,
// capped at searchMaxLimit) and a non-empty next-cursor string when
// further pages are available.
func (r *AdvertisementsRepository) Search(ctx context.Context, db Queryer, params SearchParams) ([]Advertisement, string, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = searchDefaultLimit
	}
	if limit > searchMaxLimit {
		limit = searchMaxLimit
	}

	clauses := []string{"status = 'active'"}
	args := []any{}

	// Visibility predicate: caller is owner OR shares a workgroup.
	args = append(args, params.CallerAccountID)
	callerArgIdx := len(args)
	args = append(args, pq.StringArray(params.CallerWorkgroupIDs))
	wgArgIdx := len(args)
	clauses = append(clauses, fmt.Sprintf("(account_id = $%d or workgroup_scopes && $%d)", callerArgIdx, wgArgIdx))

	if len(params.WorkgroupFilter) > 0 {
		args = append(args, pq.StringArray(params.WorkgroupFilter))
		clauses = append(clauses, fmt.Sprintf("workgroup_scopes && $%d", len(args)))
	}
	if params.OwnerOrganizationID != "" {
		args = append(args, params.OwnerOrganizationID)
		clauses = append(clauses, fmt.Sprintf("organization_id = $%d", len(args)))
	}
	if params.CapabilityKeyword != "" {
		// EXISTS subquery over the jsonb array, matching capability.name (case-insensitive substring).
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(params.CapabilityKeyword))+"%")
		clauses = append(clauses, fmt.Sprintf(`exists (
            select 1
            from jsonb_array_elements(capabilities) cap
            where lower(cap->>'name') like $%d
        )`, len(args)))
	}
	if len(params.InteractionPatternKinds) > 0 {
		// EXISTS subquery matching any interaction_pattern.kind in the supplied set.
		kinds := make([]string, len(params.InteractionPatternKinds))
		for i, k := range params.InteractionPatternKinds {
			kinds[i] = string(k)
		}
		args = append(args, pq.StringArray(kinds))
		clauses = append(clauses, fmt.Sprintf(`exists (
            select 1
            from jsonb_array_elements(interaction_patterns) ip
            where (ip->>'kind') = any($%d)
        )`, len(args)))
	}
	if params.Cursor != nil {
		// Keyset predicate for descending updated_at, descending created_at, ascending id.
		args = append(args, params.Cursor.UpdatedAt)
		updIdx := len(args)
		args = append(args, params.Cursor.CreatedAt)
		crIdx := len(args)
		args = append(args, params.Cursor.ID)
		idIdx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(updated_at < $%d or (updated_at = $%d and (created_at < $%d or (created_at = $%d and id > $%d))))",
			updIdx, updIdx, crIdx, crIdx, idIdx,
		))
	}

	args = append(args, limit+1) // fetch one extra to detect if more pages exist
	limitIdx := len(args)

	query := fmt.Sprintf(`
select id, organization_id, account_id, name, description, capabilities, interaction_patterns,
    workgroup_scopes, tunnel_mode, contract_id, schema_version, status, retracted_at, created_at, updated_at
from advertisements
where %s
order by updated_at desc, created_at desc, id asc
limit $%d`, strings.Join(clauses, " and "), limitIdx)

	var rows []Advertisement
	if err := db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, "", fmt.Errorf("search advertisements: %w", err)
	}

	var nextCursor string
	if len(rows) > limit {
		// Trim the extra row and emit a cursor pointing at the last returned record.
		last := rows[limit-1]
		c := &SearchCursor{UpdatedAt: last.UpdatedAt, CreatedAt: last.CreatedAt, ID: last.ID}
		encoded, err := EncodeSearchCursor(c)
		if err != nil {
			return nil, "", err
		}
		nextCursor = encoded
		rows = rows[:limit]
	}
	return rows, nextCursor, nil
}

func (r *AdvertisementsRepository) Update(ctx context.Context, db Queryer, ad Advertisement) (*Advertisement, error) {
	now := time.Now().UTC()
	ad.UpdatedAt = now
	if ad.Capabilities == nil {
		ad.Capabilities = CapabilitiesJSON{}
	}
	if ad.InteractionPatterns == nil {
		ad.InteractionPatterns = InteractionPatternsJSON{}
	}
	if ad.WorkgroupScopes == nil {
		ad.WorkgroupScopes = pq.StringArray{}
	}
	if ad.TunnelMode == "" {
		ad.TunnelMode = TunnelModeTCP
	}

	const query = `
update advertisements
set name = $2,
    description = $3,
    capabilities = $4,
    interaction_patterns = $5,
    workgroup_scopes = $6,
    tunnel_mode = $7,
    contract_id = $8,
    updated_at = $9
where id = $1
returning id, organization_id, account_id, name, description, capabilities, interaction_patterns,
    workgroup_scopes, tunnel_mode, contract_id, schema_version, status, retracted_at, created_at, updated_at`

	var updated Advertisement
	if err := db.GetContext(
		ctx,
		&updated,
		query,
		ad.ID,
		strings.TrimSpace(ad.Name),
		ad.Description,
		ad.Capabilities,
		ad.InteractionPatterns,
		ad.WorkgroupScopes,
		ad.TunnelMode,
		ad.ContractID,
		ad.UpdatedAt,
	); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update advertisement: %w", err)
	}
	return &updated, nil
}

// MarkRetracted transitions the advertisement to the retracted state.
// Idempotent: returns nil if the advertisement is already retracted.
func (r *AdvertisementsRepository) MarkRetracted(ctx context.Context, db Queryer, id string) error {
	const query = `
update advertisements
set status = 'retracted', retracted_at = $2, updated_at = $2
where id = $1 and status = 'active'`

	now := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, now)
	if err != nil {
		return fmt.Errorf("mark advertisement retracted: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark advertisement retracted rows affected: %w", err)
	}
	if rows == 0 {
		// Either not found or already retracted; verify via lookup.
		ad, err := r.GetByID(ctx, db, id)
		if err != nil {
			return err
		}
		if ad.Status == AdvertisementStatusRetracted {
			return nil
		}
		return ErrNotFound
	}
	return nil
}
