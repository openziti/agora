-- +migrate Up
alter table contracts add column max_envelope_bytes integer not null default 0;
alter table contracts add constraint contracts_max_envelope_bytes_nonneg check (max_envelope_bytes >= 0);

alter table sessions add column envelope_count integer;
alter table sessions add constraint sessions_envelope_count_nonneg check (envelope_count is null or envelope_count >= 0);

-- +migrate Down
alter table sessions drop constraint if exists sessions_envelope_count_nonneg;
alter table sessions drop column if exists envelope_count;
alter table contracts drop constraint if exists contracts_max_envelope_bytes_nonneg;
alter table contracts drop column if exists max_envelope_bytes;
