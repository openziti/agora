-- +migrate Up
create table workgroups (
    id text primary key,
    owner_organization_id text not null references organizations(id) on delete cascade,
    name text not null,
    description text,
    scope text not null,
    state text not null,
    deleted boolean not null default false,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint workgroups_name_not_empty check (btrim(name) <> ''),
    constraint workgroups_scope_valid check (scope in ('intra-org', 'inter-org')),
    constraint workgroups_state_valid check (state in ('pending', 'active', 'declined')),
    constraint workgroups_id_format check (id ~ '^wg_[a-z0-9]{12}$')
);

create table workgroup_invitations (
    id text primary key,
    workgroup_id text not null references workgroups(id) on delete cascade,
    organization_id text not null references organizations(id) on delete cascade,
    state text not null,
    acknowledged_by_account_id text,
    acknowledged_at timestamptz,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint workgroup_invitations_state_valid check (state in ('pending', 'accepted', 'declined')),
    constraint workgroup_invitations_id_format check (id ~ '^wgi_[a-z0-9]{12}$'),
    constraint workgroup_invitations_workgroup_org_unique unique (workgroup_id, organization_id)
);

create table workgroup_memberships (
    id text primary key,
    workgroup_id text not null references workgroups(id) on delete cascade,
    organization_id text not null references organizations(id) on delete cascade,
    account_id text not null,
    role text not null,
    joined_at timestamptz not null default current_timestamp,
    deleted boolean not null default false,
    created_at timestamptz not null default current_timestamp,
    updated_at timestamptz not null default current_timestamp,
    constraint workgroup_memberships_role_valid check (role in ('member', 'admin')),
    constraint workgroup_memberships_id_format check (id ~ '^wgm_[a-z0-9]{12}$'),
    constraint workgroup_memberships_account_organization_fk
        foreign key (account_id, organization_id) references accounts(id, organization_id) on delete cascade
);

create index idx_workgroups_owner_organization_id on workgroups (owner_organization_id);
create index idx_workgroups_state on workgroups (state) where not deleted;
create unique index idx_workgroups_owner_org_name_unique on workgroups (owner_organization_id, lower(name)) where not deleted and state <> 'declined';

create index idx_workgroup_invitations_workgroup_id on workgroup_invitations (workgroup_id);
create index idx_workgroup_invitations_organization_id on workgroup_invitations (organization_id);
create index idx_workgroup_invitations_state on workgroup_invitations (state);

create index idx_workgroup_memberships_workgroup_id on workgroup_memberships (workgroup_id);
create index idx_workgroup_memberships_account_id on workgroup_memberships (account_id);
create index idx_workgroup_memberships_organization_id on workgroup_memberships (organization_id);
create unique index idx_workgroup_memberships_workgroup_account_unique_active on workgroup_memberships (workgroup_id, account_id) where not deleted;

-- +migrate Down
drop index if exists idx_workgroup_memberships_workgroup_account_unique_active;
drop index if exists idx_workgroup_memberships_organization_id;
drop index if exists idx_workgroup_memberships_account_id;
drop index if exists idx_workgroup_memberships_workgroup_id;
drop index if exists idx_workgroup_invitations_state;
drop index if exists idx_workgroup_invitations_organization_id;
drop index if exists idx_workgroup_invitations_workgroup_id;
drop index if exists idx_workgroups_owner_org_name_unique;
drop index if exists idx_workgroups_state;
drop index if exists idx_workgroups_owner_organization_id;
drop table if exists workgroup_memberships;
drop table if exists workgroup_invitations;
drop table if exists workgroups;
