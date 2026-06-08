-- +migrate Up
alter table tunnel_attachments add column kind text not null default 'proxy';
alter table tunnel_attachments add constraint tunnel_attachments_kind_valid check (kind in ('proxy', 'dialer'));
alter table tunnel_attachments alter column listen_address drop not null;
alter table tunnel_attachments drop constraint tunnel_attachments_listen_address_not_empty;
alter table tunnel_attachments add constraint tunnel_attachments_listen_address_kind check (
    (kind = 'proxy' and listen_address is not null and btrim(listen_address) <> '') or
    (kind = 'dialer' and listen_address is null)
);
create unique index idx_tunnel_attachments_dialer_unique
    on tunnel_attachments (environment_id, tunnel_id)
    where kind = 'dialer' and state = 'active' and not deleted and dial_policy_id is not null;

-- +migrate Down
drop index if exists idx_tunnel_attachments_dialer_unique;
alter table tunnel_attachments drop constraint if exists tunnel_attachments_listen_address_kind;
update tunnel_attachments set listen_address = 'direct-dialer' where listen_address is null;
alter table tunnel_attachments alter column listen_address set not null;
alter table tunnel_attachments add constraint tunnel_attachments_listen_address_not_empty check (btrim(listen_address) <> '');
alter table tunnel_attachments drop constraint if exists tunnel_attachments_kind_valid;
alter table tunnel_attachments drop column if exists kind;
