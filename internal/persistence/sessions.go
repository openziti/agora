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
    tunnel_mode, tunnel_id, state, close_reason, close_detail, proposer_message,
    proposed_at, accepted_at, closed_at`

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
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
) returning %s`, sessionColumns, sessionColumns)

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

// SessionListParams filters session lookups.
type SessionListParams struct {
	// ParticipantAccountID restricts to sessions where the caller is
	// provider, consumer, or either, based on RoleFilter.
	ParticipantAccountID string
	RoleFilter           string // "", "provider", "consumer", "both"
	States               []SessionState
	AdvertisementID      string
}

// List returns sessions matching the filter, ordered by proposed_at desc.
// Unpaginated; MVP list endpoints return the full result set.
func (r *SessionsRepository) List(ctx context.Context, db Queryer, params SessionListParams) ([]Session, error) {
	clauses := []string{}
	args := []any{}

	if params.ParticipantAccountID != "" {
		args = append(args, params.ParticipantAccountID)
		idx := len(args)
		switch params.RoleFilter {
		case "provider":
			clauses = append(clauses, fmt.Sprintf("provider_account_id = $%d", idx))
		case "consumer":
			clauses = append(clauses, fmt.Sprintf("consumer_account_id = $%d", idx))
		default:
			clauses = append(clauses, fmt.Sprintf("(provider_account_id = $%d or consumer_account_id = $%d)", idx, idx))
		}
	}
	if len(params.States) > 0 {
		placeholders := make([]string, len(params.States))
		for i, st := range params.States {
			args = append(args, st)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		clauses = append(clauses, fmt.Sprintf("state in (%s)", strings.Join(placeholders, ", ")))
	}
	if params.AdvertisementID != "" {
		args = append(args, params.AdvertisementID)
		clauses = append(clauses, fmt.Sprintf("advertisement_id = $%d", len(args)))
	}

	where := ""
	if len(clauses) > 0 {
		where = "where " + strings.Join(clauses, " and ")
	}
	query := fmt.Sprintf(`select %s from sessions %s order by proposed_at desc, id`, sessionColumns, where)

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

