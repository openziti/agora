package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type SessionsRepository struct{}

const sessionColumns = `id, advertisement_id, workgroup_id,
    provider_account_id, provider_organization_id,
    consumer_account_id, consumer_organization_id,
    tunnel_mode, tunnel_id, contract_snapshot, envelope_count, state, close_reason, close_detail, proposer_message,
    proposed_at, accepted_at, closed_at`

const sessionListColumns = `s.id, s.advertisement_id, s.workgroup_id,
    s.provider_account_id, s.provider_organization_id,
    s.consumer_account_id, s.consumer_organization_id,
    coalesce(ad.name, s.advertisement_id) as advertisement_name,
    coalesce(wg.name, s.workgroup_id) as workgroup_name,
    coalesce(po.name, s.provider_organization_id) as provider_organization_name,
    coalesce(co.name, s.consumer_organization_id) as consumer_organization_name,
    pa.email as provider_account_email,
    ca.email as consumer_account_email,
    s.tunnel_mode, s.tunnel_id, s.contract_snapshot, s.envelope_count, s.state, s.close_reason, s.close_detail, s.proposer_message,
    s.proposed_at, s.accepted_at, s.closed_at`

// Create inserts a new session in the proposed state. ID is generated if empty.
func (r *SessionsRepository) Create(ctx context.Context, db Queryer, sess Session) (*Session, error) {
	if sess.ID == "" {
		sess.ID = NewResourceID(PrefixSession)
	}
	if sess.State == "" {
		sess.State = SessionStateProposed
	}
	if sess.ProposedAt.IsZero() {
		sess.ProposedAt = time.Now().UTC()
	}

	query := fmt.Sprintf(`
insert into sessions (%s) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
) returning %s`, sessionColumns, sessionColumns)

	snapshotArg := any(nil)
	if len(sess.ContractSnapshotJSON) > 0 {
		snapshotArg = []byte(sess.ContractSnapshotJSON)
	}

	var created Session
	if err := db.GetContext(
		ctx, &created, query,
		sess.ID,
		sess.AdvertisementID,
		sess.WorkgroupID,
		sess.ProviderAccountID,
		sess.ProviderOrganizationID,
		sess.ConsumerAccountID,
		sess.ConsumerOrganizationID,
		sess.TunnelMode,
		sess.TunnelID,
		snapshotArg,
		sess.EnvelopeCount,
		sess.State,
		sess.CloseReason,
		sess.CloseDetail,
		sess.ProposerMessage,
		sess.ProposedAt,
		sess.AcceptedAt,
		sess.ClosedAt,
	); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &created, nil
}

// WriteContractSnapshot persists the frozen contract snapshot onto an
// existing session row. Intended to be called during the accept flow
// alongside MarkActive, inside the same transaction.
func (r *SessionsRepository) WriteContractSnapshot(ctx context.Context, db Queryer, sessionID string, snapshotJSON []byte) error {
	if len(snapshotJSON) == 0 {
		return nil
	}
	const query = `update sessions set contract_snapshot = $2 where id = $1`
	if _, err := db.ExecContext(ctx, query, sessionID, snapshotJSON); err != nil {
		return fmt.Errorf("write contract snapshot: %w", err)
	}
	return nil
}

// RecordEnvelopeCount stores the reported envelope count on the session
// row using max-seen semantics: the stored value only increases. This
// protects against reordered heartbeats and idempotent retries.
// Returns ErrNotFound when the session does not exist.
func (r *SessionsRepository) RecordEnvelopeCount(ctx context.Context, db Queryer, sessionID string, count int) error {
	const query = `
update sessions
set envelope_count = greatest(coalesce(envelope_count, 0), $2)
where id = $1`
	result, err := db.ExecContext(ctx, query, sessionID, count)
	if err != nil {
		return fmt.Errorf("record envelope count: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record envelope count rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordEnvelopeCountDelta stores the reported envelope count with
// max-seen semantics and returns the delta from the previously stored
// high-water mark. The row is selected FOR UPDATE so concurrent reports
// for the same session linearize before the audit event is emitted.
func (r *SessionsRepository) RecordEnvelopeCountDelta(ctx context.Context, db Queryer, sessionID string, count int) (*Session, int, error) {
	current, err := r.GetByIDForUpdate(ctx, db, sessionID)
	if err != nil {
		return nil, 0, err
	}
	previousMax := 0
	if current.EnvelopeCount != nil {
		previousMax = *current.EnvelopeCount
	}
	countDelta := count - previousMax
	if countDelta < 0 {
		countDelta = 0
	}

	query := fmt.Sprintf(`
update sessions
set envelope_count = greatest(coalesce(envelope_count, 0), $2)
where id = $1
returning %s`, sessionColumns)

	var updated Session
	if err := db.GetContext(ctx, &updated, query, sessionID, count); err != nil {
		if isNotFound(err) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("record envelope count delta: %w", err)
	}
	return &updated, countDelta, nil
}

// ListActiveWithDurationCap returns every active session whose
// snapshot contains a positive max_duration_seconds value. The reaper
// uses this to evaluate duration bounds on a periodic tick.
func (r *SessionsRepository) ListActiveWithDurationCap(ctx context.Context, db Queryer) ([]Session, error) {
	query := fmt.Sprintf(`
select %s
from sessions
where state = 'active'
  and contract_snapshot is not null
  and coalesce((contract_snapshot->>'maxDurationSeconds')::int, 0) > 0`, sessionColumns)

	var rows []Session
	if err := db.SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("list active sessions with duration cap: %w", err)
	}
	return rows, nil
}

func (r *SessionsRepository) HasActiveTunnelForAccount(ctx context.Context, db Queryer, accountID string) (bool, error) {
	const query = `
select count(1)
from sessions
where state = 'active'
  and tunnel_id is not null
  and (provider_account_id = $1 or consumer_account_id = $1)`

	var count int
	if err := db.GetContext(ctx, &count, query, accountID); err != nil {
		return false, fmt.Errorf("check active sessions for account: %w", err)
	}
	return count > 0, nil
}

func (r *SessionsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Session, error) {
	query := fmt.Sprintf(`select %s from sessions where id = $1`, sessionColumns)
	var sess Session
	if err := db.GetContext(ctx, &sess, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &sess, nil
}

func (r *SessionsRepository) GetByIDWithDisplay(ctx context.Context, db Queryer, id string) (*Session, error) {
	query := fmt.Sprintf(`
select %s
from sessions s
left join advertisements ad on ad.id = s.advertisement_id
left join workgroups wg on wg.id = s.workgroup_id
left join organizations po on po.id = s.provider_organization_id
left join organizations co on co.id = s.consumer_organization_id
left join accounts pa on pa.id = s.provider_account_id and pa.organization_id = s.provider_organization_id
left join accounts ca on ca.id = s.consumer_account_id and ca.organization_id = s.consumer_organization_id
where s.id = $1`, sessionListColumns)

	var sess Session
	if err := db.GetContext(ctx, &sess, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get session with display: %w", err)
	}
	return &sess, nil
}

func (r *SessionsRepository) GetByIDForUpdate(ctx context.Context, db Queryer, id string) (*Session, error) {
	query := fmt.Sprintf(`select %s from sessions where id = $1 for update`, sessionColumns)
	var sess Session
	if err := db.GetContext(ctx, &sess, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get session for update: %w", err)
	}
	return &sess, nil
}

func (r *SessionsRepository) GetByTunnelID(ctx context.Context, db Queryer, tunnelID string) (*Session, error) {
	query := fmt.Sprintf(`select %s from sessions where tunnel_id = $1 order by proposed_at desc limit 1`, sessionColumns)
	var sess Session
	if err := db.GetContext(ctx, &sess, query, tunnelID); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get session by tunnel: %w", err)
	}
	return &sess, nil
}

// SessionListParams filters session lookups.
type SessionListParams struct {
	// ParticipantAccountID restricts to sessions where the caller is
	// provider, consumer, or either, based on RoleFilter.
	ParticipantAccountID string
	RoleFilter           string // "", "provider", "consumer", "both"
	States               []SessionState
	AdvertisementID      string
	Limit                int
	OrderBy              SessionListOrder
}

type SessionListOrder string

const (
	SessionListOrderProposedAtDesc SessionListOrder = "proposedAtDesc"
	SessionListOrderClosedAtDesc   SessionListOrder = "closedAtDesc"
)

// List returns sessions matching the filter. The zero OrderBy value preserves
// the historical proposed_at desc ordering.
func (r *SessionsRepository) List(ctx context.Context, db Queryer, params SessionListParams) ([]Session, error) {
	clauses := []string{}
	args := []any{}

	if params.ParticipantAccountID != "" {
		args = append(args, params.ParticipantAccountID)
		idx := len(args)
		switch params.RoleFilter {
		case "provider":
			clauses = append(clauses, fmt.Sprintf("s.provider_account_id = $%d", idx))
		case "consumer":
			clauses = append(clauses, fmt.Sprintf("s.consumer_account_id = $%d", idx))
		default:
			clauses = append(clauses, fmt.Sprintf("(s.provider_account_id = $%d or s.consumer_account_id = $%d)", idx, idx))
		}
	}
	if len(params.States) > 0 {
		placeholders := make([]string, len(params.States))
		for i, st := range params.States {
			args = append(args, st)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		clauses = append(clauses, fmt.Sprintf("s.state in (%s)", strings.Join(placeholders, ", ")))
	}
	if params.AdvertisementID != "" {
		args = append(args, params.AdvertisementID)
		clauses = append(clauses, fmt.Sprintf("s.advertisement_id = $%d", len(args)))
	}

	orderBy := "s.proposed_at desc, s.id"
	switch params.OrderBy {
	case "", SessionListOrderProposedAtDesc:
	case SessionListOrderClosedAtDesc:
		clauses = append(clauses, "s.closed_at is not null")
		orderBy = "s.closed_at desc nulls last, s.id"
	default:
		return nil, fmt.Errorf("unknown session list order '%s'", params.OrderBy)
	}

	where := ""
	if len(clauses) > 0 {
		where = "where " + strings.Join(clauses, " and ")
	}
	query := fmt.Sprintf(`
select %s
from sessions s
left join advertisements ad on ad.id = s.advertisement_id
left join workgroups wg on wg.id = s.workgroup_id
left join organizations po on po.id = s.provider_organization_id
left join organizations co on co.id = s.consumer_organization_id
left join accounts pa on pa.id = s.provider_account_id and pa.organization_id = s.provider_organization_id
left join accounts ca on ca.id = s.consumer_account_id and ca.organization_id = s.consumer_organization_id
%s
order by %s`, sessionListColumns, where, orderBy)
	if params.Limit > 0 {
		args = append(args, params.Limit)
		query += fmt.Sprintf("\nlimit $%d", len(args))
	}

	var rows []Session
	if err := db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return rows, nil
}

// MarkAccepting transitions a proposed session to accepting.
func (r *SessionsRepository) MarkAccepting(ctx context.Context, db Queryer, id string) (*Session, error) {
	return r.stateTransition(ctx, db, id,
		`update sessions set state = 'accepting' where id = $1 and state = 'proposed'`)
}

// MarkActive transitions an accepting session to active with the given tunnel.
func (r *SessionsRepository) MarkActive(ctx context.Context, db Queryer, id, tunnelID string) (*Session, error) {
	now := time.Now().UTC()
	query := fmt.Sprintf(`
update sessions set state = 'active', tunnel_id = $2, accepted_at = $3
where id = $1 and state = 'accepting'
returning %s`, sessionColumns)
	var sess Session
	if err := db.GetContext(ctx, &sess, query, id, tunnelID, now); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("mark session active: %w", err)
	}
	return &sess, nil
}

// MarkRejected closes a proposed session with reason=rejected.
// Idempotent: returns the current row if already closed.
func (r *SessionsRepository) MarkRejected(ctx context.Context, db Queryer, id, detail string) (*Session, error) {
	return r.closeWithReason(ctx, db, id, SessionCloseReasonRejected, detail, []SessionState{SessionStateProposed})
}

// MarkClosed transitions a session to closed with the given reason.
// Idempotent: returns the existing record if already closed.
func (r *SessionsRepository) MarkClosed(ctx context.Context, db Queryer, id string, reason SessionCloseReason, detail string) (*Session, error) {
	return r.closeWithReason(ctx, db, id, reason, detail, []SessionState{
		SessionStateProposed, SessionStateAccepting, SessionStateActive, SessionStateClosing,
	})
}

func (r *SessionsRepository) stateTransition(ctx context.Context, db Queryer, id, stmt string) (*Session, error) {
	result, err := db.ExecContext(ctx, stmt, id)
	if err != nil {
		return nil, fmt.Errorf("session state transition: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("session state transition rows affected: %w", err)
	}
	if rows == 0 {
		existing, err := r.GetByID(ctx, db, id)
		if err != nil {
			return nil, err
		}
		return existing, nil
	}
	return r.GetByID(ctx, db, id)
}

func (r *SessionsRepository) closeWithReason(ctx context.Context, db Queryer, id string, reason SessionCloseReason, detail string, fromStates []SessionState) (*Session, error) {
	placeholders := make([]string, len(fromStates))
	args := []any{id, string(reason)}
	for i, s := range fromStates {
		args = append(args, string(s))
		placeholders[i] = fmt.Sprintf("$%d", len(args))
	}
	var detailArg any
	if strings.TrimSpace(detail) == "" {
		detailArg = nil
	} else {
		detailArg = detail
	}
	args = append(args, detailArg)
	detailIdx := len(args)
	now := time.Now().UTC()
	args = append(args, now)
	nowIdx := len(args)

	query := fmt.Sprintf(`
update sessions
set state = 'closed', close_reason = $2, close_detail = $%d, closed_at = $%d
where id = $1 and state in (%s)`, detailIdx, nowIdx, strings.Join(placeholders, ", "))

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("close session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("close session rows affected: %w", err)
	}
	if rows == 0 {
		existing, err := r.GetByID(ctx, db, id)
		if err != nil {
			return nil, err
		}
		if existing.State == SessionStateClosed {
			return existing, nil
		}
		return nil, ErrNotFound
	}
	return r.GetByID(ctx, db, id)
}
