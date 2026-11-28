create schema if not exists workflow_validator;

create table workflow_validator.workflows
(
    id         uuid                        default gen_random_uuid() not null,
    created    timestamp(6) with time zone default now()             not null,
    updated    timestamp(6) with time zone,
    service_id uuid                                                  not null,
    constraint pk_workflows primary key (id),
    constraint fk_workflows_on_service_id foreign key (service_id) references owners.services on delete restrict
);

create index idx_workflows_service_id on workflow_validator.workflows (service_id);

create table workflow_validator.blocks
(
    workflow_id uuid  not null,
    id          text  not null,
    type        text  not null,
    metadata    jsonb not null,
    constraint pk_blocks primary key (workflow_id, id),
    constraint fk_blocks_on_workflow_id foreign key (workflow_id) references workflow_validator.workflows on delete cascade
);
