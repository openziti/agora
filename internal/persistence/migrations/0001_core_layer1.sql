-- +migrate Up
create table organizations (
    id uuid primary key,
    name text not null unique,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint organizations_name_not_empty check (btrim(name) <> '')
);

create table accounts (
    id uuid primary key,
    organization_id uuid not null references organizations(id) on delete cascade,
    email text not null,
    display_name text,
    role text not null,
    status text not null,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint accounts_email_not_empty check (btrim(email) <> ''),
    constraint accounts_role_valid check (role in ('admin', 'member')),
    constraint accounts_status_valid check (status in ('active', 'disabled')),
    constraint accounts_id_organization_unique unique (id, organization_id),
    constraint accounts_org_email_unique unique (organization_id, email)
);

create table environments (
    id uuid primary key,
    organization_id uuid not null references organizations(id) on delete cascade,
    account_id uuid not null,
    description text,
    host text,
    ziti_identity_id text not null unique,
    state text not null,
    last_seen_at timestamptz,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint environments_ziti_identity_not_empty check (btrim(ziti_identity_id) <> ''),
    constraint environments_state_valid check (state in ('enabled', 'disabled')),
    constraint environments_id_organization_unique unique (id, organization_id),
    constraint environments_account_organization_fk
        foreign key (account_id, organization_id) references accounts(id, organization_id) on delete cascade
);

create table tunnels (
    id uuid primary key,
    organization_id uuid not null references organizations(id) on delete cascade,
    environment_id uuid not null,
    name text not null,
    backend_address text not null,
    ziti_service_id text unique,
    state text not null,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint tunnels_name_not_empty check (btrim(name) <> ''),
    constraint tunnels_backend_address_not_empty check (btrim(backend_address) <> ''),
    constraint tunnels_state_valid check (state in ('active', 'disabled')),
    constraint tunnels_environment_organization_fk
        foreign key (environment_id, organization_id) references environments(id, organization_id) on delete cascade,
    constraint tunnels_org_name_unique unique (organization_id, name)
);

-- +migrate Down
drop table if exists tunnels;
drop table if exists environments;
drop table if exists accounts;
drop table if exists organizations;
