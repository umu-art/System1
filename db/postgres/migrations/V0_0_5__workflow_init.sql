create schema if not exists workflow;

create table workflow.runs
(
    id               uuid                        not null,
    workflow_id      uuid                        not null,
    service_id       uuid                        not null,
    entry_block_id   text                        not null,
    current_block_id text,
    status           text                        not null,
    snapshot         jsonb                       not null,
    variables        jsonb                       not null,
    state            jsonb                       not null,
    created          timestamp(6) with time zone default now() not null,
    updated          timestamp(6) with time zone,
    constraint pk_runs primary key (id),
    constraint fk_runs_on_service_id foreign key (service_id) references owners.services on delete restrict,
    constraint runs_state_fields_separate check (not (state ?| array['blockId', 'status', 'input', 'state']))
);
