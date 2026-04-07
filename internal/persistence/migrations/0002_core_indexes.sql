-- +migrate Up
create index idx_accounts_organization_id on accounts (organization_id);
create unique index idx_accounts_lower_email_unique on accounts (lower(email));
create unique index idx_accounts_account_token_unique on accounts (account_token);
create index idx_environments_account_id on environments (account_id);
create index idx_environments_organization_id on environments (organization_id);
create index idx_tunnels_environment_id on tunnels (environment_id);
create index idx_tunnels_organization_id on tunnels (organization_id);

-- +migrate Down
drop index if exists idx_tunnels_organization_id;
drop index if exists idx_tunnels_environment_id;
drop index if exists idx_environments_organization_id;
drop index if exists idx_environments_account_id;
drop index if exists idx_accounts_account_token_unique;
drop index if exists idx_accounts_lower_email_unique;
drop index if exists idx_accounts_organization_id;
