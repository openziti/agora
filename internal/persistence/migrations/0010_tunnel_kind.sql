-- +migrate Up
alter table tunnels add column kind text not null default 'proxy';
alter table tunnels add constraint tunnels_kind_valid check (kind in ('proxy', 'direct'));
alter table tunnels alter column backend_target drop not null;
alter table tunnels drop constraint tunnels_backend_target_not_empty;
alter table tunnels add constraint tunnels_backend_target_kind check (
    (kind = 'proxy' and backend_target is not null and btrim(backend_target) <> '') or
    (kind = 'direct' and backend_target is null)
);

-- +migrate Down
alter table tunnels drop constraint if exists tunnels_backend_target_kind;
update tunnels set backend_target = 'direct' where backend_target is null;
alter table tunnels alter column backend_target set not null;
alter table tunnels add constraint tunnels_backend_target_not_empty check (btrim(backend_target) <> '');
alter table tunnels drop constraint if exists tunnels_kind_valid;
alter table tunnels drop column if exists kind;
