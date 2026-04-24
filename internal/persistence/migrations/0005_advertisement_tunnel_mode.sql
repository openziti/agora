-- +migrate Up
alter table advertisements add column tunnel_mode text not null default 'tcp';
alter table advertisements add constraint advertisements_tunnel_mode_valid check (tunnel_mode in ('http', 'tcp', 'udp'));

-- +migrate Down
alter table advertisements drop constraint if exists advertisements_tunnel_mode_valid;
alter table advertisements drop column if exists tunnel_mode;
