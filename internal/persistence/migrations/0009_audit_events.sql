-- +migrate Up
create table audit_events (
    id bigserial primary key,
    occurred_at timestamptz not null,
    event_type text not null,
    organization_id text not null,
    account_id text,
    workgroup_id text,
    session_id text,
    advertisement_id text,
    contract_id text,
    envelope_id text,
    data jsonb not null default '{}'::jsonb
);

create index idx_audit_events_org_occurred on audit_events (organization_id, occurred_at desc);
create index idx_audit_events_org_type_occurred on audit_events (organization_id, event_type, occurred_at desc);
create index idx_audit_events_account_occurred on audit_events (account_id, occurred_at desc);
create index idx_audit_events_session_occurred on audit_events (session_id, occurred_at desc);
create index idx_audit_events_workgroup_occurred on audit_events (workgroup_id, occurred_at desc);

-- +migrate Down
drop index if exists idx_audit_events_workgroup_occurred;
drop index if exists idx_audit_events_session_occurred;
drop index if exists idx_audit_events_account_occurred;
drop index if exists idx_audit_events_org_type_occurred;
drop index if exists idx_audit_events_org_occurred;
drop table if exists audit_events;
