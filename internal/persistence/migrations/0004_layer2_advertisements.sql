-- +migrate Up
create table advertisements (
    id text primary key,
    organization_id text not null references organizations(id) on delete cascade,
    account_id text not null,
    name text not null,
    description text,
    capabilities jsonb not null default '[]'::jsonb,
    interaction_patterns jsonb not null default '[]'::jsonb,
    workgroup_scopes text[] not null default '{}'::text[],
    schema_version integer not null default 1,
    status text not null,
    retracted_at timestamptz,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint advertisements_name_not_empty check (btrim(name) <> ''),
    constraint advertisements_status_valid check (status in ('active', 'retracted')),
    constraint advertisements_id_format check (id ~ '^adv_[a-z0-9]{12}$'),
    constraint advertisements_account_organization_fk
        foreign key (account_id, organization_id) references accounts(id, organization_id) on delete cascade
);

create index idx_advertisements_account_id on advertisements (account_id);
create index idx_advertisements_organization_id on advertisements (organization_id);
create unique index idx_advertisements_account_name_unique_active on advertisements (account_id, lower(name)) where status = 'active';
create index idx_advertisements_workgroup_scopes_gin on advertisements using gin (workgroup_scopes);
create index idx_advertisements_capabilities_gin on advertisements using gin (capabilities jsonb_path_ops);
create index idx_advertisements_interaction_patterns_gin on advertisements using gin (interaction_patterns jsonb_path_ops);
create index idx_advertisements_status_updated on advertisements (status, updated_at desc, created_at desc, id);

-- +migrate Down
drop index if exists idx_advertisements_status_updated;
drop index if exists idx_advertisements_interaction_patterns_gin;
drop index if exists idx_advertisements_capabilities_gin;
drop index if exists idx_advertisements_workgroup_scopes_gin;
drop index if exists idx_advertisements_account_name_unique_active;
drop index if exists idx_advertisements_organization_id;
drop index if exists idx_advertisements_account_id;
drop table if exists advertisements;
