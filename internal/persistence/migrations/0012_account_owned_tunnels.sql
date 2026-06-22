-- +migrate Up
-- account-owned tunnels: ownership moves from the creating environment to the account. a standalone
-- tunnel has a NULL environment_id (account-owned, re-hostable by any of the account's environments);
-- a layer 2 session tunnel keeps environment_id set. the null/set split is the standalone-vs-session
-- discriminator.
alter table tunnels alter column environment_id drop not null;

-- enforce the org -> account -> environment boundary in the schema: a tunnel that references an
-- environment must reference one owned by the tunnel's own account, not merely one in the same
-- organization. this needs a matching unique target on environments.
alter table environments add constraint environments_id_account_organization_unique
    unique (id, account_id, organization_id);

-- replace the org-scoped ON DELETE CASCADE env FK (which deleted tunnels when their environment was
-- deleted) with an account-scoped NO ACTION FK. a standalone tunnel has NULL environment_id and
-- (MATCH SIMPLE) is not checked by this FK at all; a session tunnel keeps environment_id set and is
-- torn down by the retire handler before the environment row is deleted, so the constraint never
-- blocks retire. NO ACTION is cascade-safe because tunnels are still independently cascade-deleted via
-- tunnels_account_organization_fk / the organization FK when an account or org is removed.
alter table tunnels drop constraint tunnels_environment_organization_fk;
alter table tunnels add constraint tunnels_environment_account_organization_fk
    foreign key (environment_id, account_id, organization_id)
    references environments(id, account_id, organization_id);

-- +migrate Down
alter table tunnels drop constraint if exists tunnels_environment_account_organization_fk;
-- restoring the org-scoped cascade FK requires every tunnel to reference an environment; standalone
-- (account-owned) tunnels with NULL environment_id cannot be reversed in place and must be removed
-- first by the operator (lab-only, recreate rather than migrate).
alter table tunnels add constraint tunnels_environment_organization_fk
    foreign key (environment_id, organization_id) references environments(id, organization_id) on delete cascade;
alter table environments drop constraint if exists environments_id_account_organization_unique;
alter table tunnels alter column environment_id set not null;
