package persistence

import (
	"context"
	"fmt"
	"time"
)

type WorkgroupInvitationsRepository struct{}

func (r *WorkgroupInvitationsRepository) Create(ctx context.Context, db Queryer, invitation WorkgroupInvitation) (*WorkgroupInvitation, error) {
	if invitation.ID == "" {
		invitation.ID = NewResourceID(PrefixWorkgroupInvitation)
	}
	now := time.Now().UTC()
	invitation.CreatedAt = now
	invitation.UpdatedAt = now
	if invitation.State == "" {
		invitation.State = WorkgroupInvitationStatePending
	}

	const query = `
insert into workgroup_invitations (
    id, workgroup_id, organization_id, state, acknowledged_by_account_id, acknowledged_at, created_at, updated_at
) values (
    $1, $2, $3, $4, $5, $6, $7, $8
)
returning id, workgroup_id, organization_id, state, acknowledged_by_account_id, acknowledged_at, created_at, updated_at`

	var created WorkgroupInvitation
	if err := db.GetContext(
		ctx,
		&created,
		query,
		invitation.ID,
		invitation.WorkgroupID,
		invitation.OrganizationID,
		invitation.State,
		invitation.AcknowledgedByAccountID,
		invitation.AcknowledgedAt,
		invitation.CreatedAt,
		invitation.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create workgroup invitation: %w", err)
	}
	return &created, nil
}

func (r *WorkgroupInvitationsRepository) GetByID(ctx context.Context, db Queryer, id string) (*WorkgroupInvitation, error) {
	const query = `
select id, workgroup_id, organization_id, state, acknowledged_by_account_id, acknowledged_at, created_at, updated_at
from workgroup_invitations
where id = $1`

	var invitation WorkgroupInvitation
	if err := db.GetContext(ctx, &invitation, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workgroup invitation: %w", err)
	}
	return &invitation, nil
}

func (r *WorkgroupInvitationsRepository) GetByWorkgroupAndOrg(ctx context.Context, db Queryer, workgroupID, organizationID string) (*WorkgroupInvitation, error) {
	const query = `
select id, workgroup_id, organization_id, state, acknowledged_by_account_id, acknowledged_at, created_at, updated_at
from workgroup_invitations
where workgroup_id = $1 and organization_id = $2`

	var invitation WorkgroupInvitation
	if err := db.GetContext(ctx, &invitation, query, workgroupID, organizationID); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workgroup invitation by workgroup+org: %w", err)
	}
	return &invitation, nil
}

func (r *WorkgroupInvitationsRepository) ListByWorkgroup(ctx context.Context, db Queryer, workgroupID string) ([]WorkgroupInvitation, error) {
	const query = `
select id, workgroup_id, organization_id, state, acknowledged_by_account_id, acknowledged_at, created_at, updated_at
from workgroup_invitations
where workgroup_id = $1
order by created_at asc`

	var invitations []WorkgroupInvitation
	if err := db.SelectContext(ctx, &invitations, query, workgroupID); err != nil {
		return nil, fmt.Errorf("list workgroup invitations: %w", err)
	}
	return invitations, nil
}

func (r *WorkgroupInvitationsRepository) UpdateState(ctx context.Context, db Queryer, id string, state WorkgroupInvitationState, acknowledgedByAccountID string, acknowledgedAt time.Time) error {
	const query = `
update workgroup_invitations
set state = $2, acknowledged_by_account_id = $3, acknowledged_at = $4, updated_at = $5
where id = $1`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, state, acknowledgedByAccountID, acknowledgedAt, updatedAt)
	if err != nil {
		return fmt.Errorf("update workgroup invitation state: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update workgroup invitation state rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
