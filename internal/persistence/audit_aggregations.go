package persistence

import (
	"context"
	"fmt"
	"sort"
	"time"
)

const environmentStatusTTL = 45 * time.Second

type Bucket struct {
	Start     time.Time
	Envelopes int
	Sessions  int
}

type WorkgroupEnvelopes struct {
	WorkgroupID   string `db:"workgroup_id"`
	WorkgroupName string `db:"workgroup_name"`
	Envelopes     int    `db:"envelopes"`
}

type EnvironmentStatusValue string

const (
	EnvironmentStatusOnline   EnvironmentStatusValue = "online"
	EnvironmentStatusStale    EnvironmentStatusValue = "stale"
	EnvironmentStatusUnknown  EnvironmentStatusValue = "unknown"
	EnvironmentStatusDisabled EnvironmentStatusValue = "disabled"
)

type EnvironmentStatus struct {
	ID              string
	Name            string
	AccountID       string
	Status          EnvironmentStatusValue
	LastHeartbeatAt *time.Time
}

type DashboardCountsResult struct {
	Stats  DashboardStats
	Ribbon DashboardRibbon
}

type DashboardStats struct {
	ActiveSessions        int
	ActiveSessionsDelta7d int
	EnvelopesToday        int
	EnvelopesYesterday    int
	ActiveWorkgroups      int
	ActiveTunnels         int
}

type DashboardRibbon struct {
	WorkgroupCount     int
	AdvertisementCount int
	SessionsToday      int
	EnvironmentCount   int
}

func EnvelopeBuckets(ctx context.Context, db Queryer, orgID string, window, bucket time.Duration) ([]Bucket, error) {
	return EnvelopeBucketsAt(ctx, db, orgID, time.Now().UTC(), window, bucket)
}

func EnvelopeBucketsAt(ctx context.Context, db Queryer, orgID string, now time.Time, window, bucket time.Duration) ([]Bucket, error) {
	now = now.UTC()
	buckets, firstStart, _, err := buildBuckets(now, window, bucket)
	if err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		return buckets, nil
	}

	const query = `
select occurred_at, event_type, coalesce((data->>'count_delta')::bigint, 0) as count_delta
from audit_events
where organization_id = $1
  and event_type in ('envelope.flowed', 'session.proposed')
  and occurred_at >= $2
  and occurred_at < $3
order by occurred_at asc`

	var rows []struct {
		OccurredAt time.Time      `db:"occurred_at"`
		EventType  AuditEventType `db:"event_type"`
		CountDelta int            `db:"count_delta"`
	}
	if err := db.SelectContext(ctx, &rows, query, orgID, firstStart, now); err != nil {
		return nil, fmt.Errorf("audit envelope buckets: %w", err)
	}

	bucketIndex := make(map[time.Time]int, len(buckets))
	for i := range buckets {
		bucketIndex[buckets[i].Start] = i
	}
	for _, row := range rows {
		start := floorToBucket(row.OccurredAt, bucket)
		idx, ok := bucketIndex[start]
		if !ok {
			continue
		}
		switch row.EventType {
		case AuditEventEnvelopeFlowed:
			buckets[idx].Envelopes += row.CountDelta
		case AuditEventSessionProposed:
			buckets[idx].Sessions++
		}
	}
	return buckets, nil
}

func EnvelopesByWorkgroup(ctx context.Context, db Queryer, orgID string, window time.Duration, limit int) ([]WorkgroupEnvelopes, error) {
	return EnvelopesByWorkgroupAt(ctx, db, orgID, time.Now().UTC(), window, limit)
}

func EnvelopesByWorkgroupAt(ctx context.Context, db Queryer, orgID string, now time.Time, window time.Duration, limit int) ([]WorkgroupEnvelopes, error) {
	if window <= 0 {
		return nil, fmt.Errorf("window must be positive")
	}
	if limit <= 0 {
		return []WorkgroupEnvelopes{}, nil
	}
	now = now.UTC()
	windowStart := now.Add(-window)

	const query = `
select
	ae.workgroup_id as workgroup_id,
	coalesce(w.name, ae.workgroup_id) as workgroup_name,
	coalesce(sum(coalesce((ae.data->>'count_delta')::bigint, 0)), 0) as envelopes
from audit_events ae
left join workgroups w on w.id = ae.workgroup_id
where ae.organization_id = $1
  and ae.event_type = 'envelope.flowed'
  and ae.workgroup_id is not null
  and ae.occurred_at >= $2
  and ae.occurred_at < $3
group by ae.workgroup_id, w.name
order by envelopes desc, ae.workgroup_id asc
limit $4`

	var rows []WorkgroupEnvelopes
	if err := db.SelectContext(ctx, &rows, query, orgID, windowStart, now, limit); err != nil {
		return nil, fmt.Errorf("audit envelopes by workgroup: %w", err)
	}
	return rows, nil
}

func EnvelopesByVisibleWorkgroups(ctx context.Context, db Queryer, orgID, accountID string, window time.Duration) ([]WorkgroupEnvelopes, error) {
	return EnvelopesByVisibleWorkgroupsAt(ctx, db, orgID, accountID, time.Now().UTC(), window)
}

func EnvelopesByVisibleWorkgroupsAt(ctx context.Context, db Queryer, orgID, accountID string, now time.Time, window time.Duration) ([]WorkgroupEnvelopes, error) {
	if window <= 0 {
		return nil, fmt.Errorf("window must be positive")
	}
	now = now.UTC()
	windowStart := now.Add(-window)

	const query = `
with envelope_counts as (
	select
		workgroup_id,
		coalesce(sum(coalesce((data->>'count_delta')::bigint, 0)), 0) as envelopes
	from audit_events
	where organization_id = $1
	  and event_type = 'envelope.flowed'
	  and workgroup_id is not null
	  and occurred_at >= $3
	  and occurred_at < $4
	group by workgroup_id
)
select
	wg.id as workgroup_id,
	wg.name as workgroup_name,
	coalesce(ec.envelopes, 0) as envelopes
from workgroup_memberships wm
inner join workgroups wg on wg.id = wm.workgroup_id and not wg.deleted
left join envelope_counts ec on ec.workgroup_id = wg.id
where wm.organization_id = $1
  and wm.account_id = $2
  and not wm.deleted
order by wg.name asc, wg.id asc`

	var rows []WorkgroupEnvelopes
	if err := db.SelectContext(ctx, &rows, query, orgID, accountID, windowStart, now); err != nil {
		return nil, fmt.Errorf("audit envelopes by visible workgroups: %w", err)
	}
	return rows, nil
}

func EnvironmentStatuses(ctx context.Context, db Queryer, orgID string) ([]EnvironmentStatus, error) {
	return EnvironmentStatusesAt(ctx, db, orgID, time.Now().UTC())
}

func EnvironmentStatusesAt(ctx context.Context, db Queryer, orgID string, now time.Time) ([]EnvironmentStatus, error) {
	const query = `
select
	id,
	coalesce(nullif(host, ''), nullif(description, ''), id) as name,
	account_id,
	state,
	last_seen_at
from environments
where organization_id = $1 and not deleted`

	var rows []struct {
		ID              string           `db:"id"`
		Name            string           `db:"name"`
		AccountID       string           `db:"account_id"`
		State           EnvironmentState `db:"state"`
		LastHeartbeatAt *time.Time       `db:"last_seen_at"`
	}
	if err := db.SelectContext(ctx, &rows, query, orgID); err != nil {
		return nil, fmt.Errorf("environment statuses: %w", err)
	}

	result := make([]EnvironmentStatus, 0, len(rows))
	for _, row := range rows {
		result = append(result, EnvironmentStatus{
			ID:              row.ID,
			Name:            row.Name,
			AccountID:       row.AccountID,
			Status:          deriveEnvironmentStatus(now, row.State, row.LastHeartbeatAt),
			LastHeartbeatAt: row.LastHeartbeatAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := environmentStatusRank(result[i].Status), environmentStatusRank(result[j].Status)
		if left != right {
			return left < right
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func DashboardCounts(ctx context.Context, db Queryer, orgID, accountID string) (*DashboardCountsResult, error) {
	return DashboardCountsAt(ctx, db, orgID, accountID, time.Now().UTC())
}

func DashboardCountsAt(ctx context.Context, db Queryer, orgID, accountID string, now time.Time) (*DashboardCountsResult, error) {
	now = now.UTC()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
	twentyFourHoursAgo := now.Add(-24 * time.Hour)
	fortyEightHoursAgo := now.Add(-48 * time.Hour)

	activeSessions, err := queryInt(ctx, db, `
select count(*)
from sessions
where (provider_organization_id = $1 or consumer_organization_id = $1)
  and state in ('proposed', 'accepting', 'active', 'closing')`, orgID)
	if err != nil {
		return nil, err
	}
	historicalSessions, err := queryInt(ctx, db, `
select count(*)
from sessions
where (provider_organization_id = $1 or consumer_organization_id = $1)
  and proposed_at <= $2
  and (closed_at is null or closed_at > $2)`, orgID, sevenDaysAgo)
	if err != nil {
		return nil, err
	}
	envelopesToday, err := queryInt(ctx, db, `
select coalesce(sum(coalesce((data->>'count_delta')::bigint, 0)), 0)
from audit_events
where organization_id = $1
  and event_type = 'envelope.flowed'
  and occurred_at >= $2
  and occurred_at < $3`, orgID, twentyFourHoursAgo, now)
	if err != nil {
		return nil, err
	}
	envelopesYesterday, err := queryInt(ctx, db, `
select coalesce(sum(coalesce((data->>'count_delta')::bigint, 0)), 0)
from audit_events
where organization_id = $1
  and event_type = 'envelope.flowed'
  and occurred_at >= $2
  and occurred_at < $3`, orgID, fortyEightHoursAgo, twentyFourHoursAgo)
	if err != nil {
		return nil, err
	}
	activeWorkgroups, err := queryInt(ctx, db, `
select count(distinct workgroup_id)
from workgroup_memberships
where account_id = $1 and not deleted`, accountID)
	if err != nil {
		return nil, err
	}
	activeTunnels, err := queryInt(ctx, db, `
select
	(select count(*) from tunnel_attachments where organization_id = $1 and state = 'active' and not deleted) +
	(select count(*) from tunnel_serves where organization_id = $1 and state = 'active' and not deleted)`, orgID)
	if err != nil {
		return nil, err
	}
	advertisementCount, err := queryInt(ctx, db, `
select count(*)
from advertisements
where organization_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	sessionsToday, err := queryInt(ctx, db, `
select count(*)
from audit_events
where organization_id = $1
  and event_type = 'session.proposed'
  and occurred_at >= $2
  and occurred_at < $3`, orgID, twentyFourHoursAgo, now)
	if err != nil {
		return nil, err
	}
	environmentCount, err := queryInt(ctx, db, `
select count(*)
from environments
where organization_id = $1 and state <> 'disabled' and not deleted`, orgID)
	if err != nil {
		return nil, err
	}

	return &DashboardCountsResult{
		Stats: DashboardStats{
			ActiveSessions:        activeSessions,
			ActiveSessionsDelta7d: activeSessions - historicalSessions,
			EnvelopesToday:        envelopesToday,
			EnvelopesYesterday:    envelopesYesterday,
			ActiveWorkgroups:      activeWorkgroups,
			ActiveTunnels:         activeTunnels,
		},
		Ribbon: DashboardRibbon{
			WorkgroupCount:     activeWorkgroups,
			AdvertisementCount: advertisementCount,
			SessionsToday:      sessionsToday,
			EnvironmentCount:   environmentCount,
		},
	}, nil
}

func queryInt(ctx context.Context, db Queryer, query string, args ...any) (int, error) {
	var value int
	if err := db.GetContext(ctx, &value, query, args...); err != nil {
		return 0, fmt.Errorf("audit aggregation count: %w", err)
	}
	return value, nil
}

func buildBuckets(now time.Time, window, bucket time.Duration) ([]Bucket, time.Time, time.Time, error) {
	if window <= 0 {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("window must be positive")
	}
	if bucket <= 0 {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("bucket must be positive")
	}
	if bucket >= window {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("bucket must be smaller than window")
	}

	windowStart := now.Add(-window)
	firstStart := floorToBucket(windowStart, bucket)
	if firstStart.Before(windowStart) {
		firstStart = firstStart.Add(bucket)
	}
	lastStart := floorToBucket(now.Add(-time.Nanosecond), bucket)
	if lastStart.Before(firstStart) {
		return []Bucket{}, firstStart, lastStart, nil
	}

	count := int(lastStart.Sub(firstStart)/bucket) + 1
	buckets := make([]Bucket, 0, count)
	for start := firstStart; !start.After(lastStart); start = start.Add(bucket) {
		buckets = append(buckets, Bucket{Start: start})
	}
	return buckets, firstStart, lastStart, nil
}

func floorToBucket(t time.Time, bucket time.Duration) time.Time {
	utc := t.UTC()
	bucketNanos := int64(bucket)
	nanos := utc.UnixNano()
	return time.Unix(0, (nanos/bucketNanos)*bucketNanos).UTC()
}

func deriveEnvironmentStatus(now time.Time, state EnvironmentState, lastHeartbeatAt *time.Time) EnvironmentStatusValue {
	if state == EnvironmentStateDisabled {
		return EnvironmentStatusDisabled
	}
	if lastHeartbeatAt == nil {
		return EnvironmentStatusUnknown
	}
	if now.UTC().Sub(lastHeartbeatAt.UTC()) <= environmentStatusTTL {
		return EnvironmentStatusOnline
	}
	return EnvironmentStatusStale
}

func environmentStatusRank(status EnvironmentStatusValue) int {
	switch status {
	case EnvironmentStatusOnline:
		return 0
	case EnvironmentStatusStale:
		return 1
	case EnvironmentStatusUnknown:
		return 2
	case EnvironmentStatusDisabled:
		return 3
	default:
		return 4
	}
}
