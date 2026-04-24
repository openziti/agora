-- +migrate Up
create table contracts (
    id text primary key,
    account_id text not null,
    organization_id text not null references organizations(id) on delete cascade,
    name text not null,
    description text,
    schema_version integer not null default 1,
    max_duration_seconds integer not null default 0,
    max_envelope_count integer not null default 0,
    allowed_message_types text[] not null default '{}'::text[],
    required_workgroup_memberships text[] not null default '{}'::text[],
    maturity_requirements jsonb not null default '{}'::jsonb,
    access_mode text not null,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint contracts_name_not_empty check (btrim(name) <> ''),
    constraint contracts_id_format check (id ~ '^con_[a-z0-9]{12}$'),
    constraint contracts_access_mode_valid check (access_mode in ('open', 'approval_required')),
    constraint contracts_max_duration_nonneg check (max_duration_seconds >= 0),
    constraint contracts_max_envelope_nonneg check (max_envelope_count >= 0),
    constraint contracts_account_organization_fk
        foreign key (account_id, organization_id) references accounts(id, organization_id) on delete cascade
);

create index idx_contracts_account_id on contracts (account_id);
create index idx_contracts_organization_id on contracts (organization_id);
create unique index idx_contracts_account_name_unique on contracts (account_id, lower(name));

alter table advertisements add column contract_id text references contracts(id) on delete set null;
create index idx_advertisements_contract_id on advertisements (contract_id) where contract_id is not null;

-- +migrate Down
drop index if exists idx_advertisements_contract_id;
alter table advertisements drop column if exists contract_id;
drop index if exists idx_contracts_account_name_unique;
drop index if exists idx_contracts_organization_id;
drop index if exists idx_contracts_account_id;
drop table if exists contracts;
