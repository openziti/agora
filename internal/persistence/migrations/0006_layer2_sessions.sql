-- +migrate Up
create table sessions (
    id text primary key,
    advertisement_id text not null references advertisements(id) on delete restrict,
    workgroup_id text not null references workgroups(id) on delete restrict,
    provider_account_id text not null,
    provider_organization_id text not null references organizations(id) on delete cascade,
    consumer_account_id text not null,
    consumer_organization_id text not null references organizations(id) on delete cascade,
    tunnel_mode text not null,
    tunnel_id text references tunnels(id) on delete set null,
    contract_snapshot jsonb,
    state text not null,
    close_reason text,
    close_detail text,
    proposer_message text,
    proposed_at timestamptz not null default current_timestamp,
    accepted_at timestamptz,
    closed_at timestamptz,
    constraint sessions_id_format check (id ~ '^ses_[a-z0-9]{12}$'),
    constraint sessions_tunnel_mode_valid check (tunnel_mode in ('http', 'tcp', 'udp')),
    constraint sessions_state_valid check (state in ('proposed', 'accepting', 'active', 'closing', 'closed')),
    constraint sessions_close_reason_valid check (
        close_reason is null or close_reason in (
            'rejected', 'consumer_close', 'provider_close', 'contract_violation',
            'tunnel_failed', 'admin_close', 'workgroup_deleted', 'environment_disabled'
        )
    ),
    constraint sessions_provider_account_fk
        foreign key (provider_account_id, provider_organization_id) references accounts(id, organization_id) on delete cascade,
    constraint sessions_consumer_account_fk
        foreign key (consumer_account_id, consumer_organization_id) references accounts(id, organization_id) on delete cascade
);

create index idx_sessions_provider on sessions (provider_account_id, proposed_at desc, id);
create index idx_sessions_consumer on sessions (consumer_account_id, proposed_at desc, id);
create index idx_sessions_advertisement on sessions (advertisement_id, state, proposed_at desc, id);
create index idx_sessions_state on sessions (state, proposed_at desc, id);
create index idx_sessions_workgroup on sessions (workgroup_id);
create index idx_sessions_tunnel on sessions (tunnel_id);

-- +migrate Down
drop index if exists idx_sessions_tunnel;
drop index if exists idx_sessions_workgroup;
drop index if exists idx_sessions_state;
drop index if exists idx_sessions_advertisement;
drop index if exists idx_sessions_consumer;
drop index if exists idx_sessions_provider;
drop table if exists sessions;
