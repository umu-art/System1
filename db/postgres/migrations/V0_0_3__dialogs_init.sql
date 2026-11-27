create schema if not exists dialogs;

create table if not exists dialogs.clients
(
    id           uuid                        default uuid_generate_v4() not null,
    created      timestamp(6) with time zone default now()              not null,
    updated      timestamp(6) with time zone,
    display_name varchar(255)                                           not null,
    language     varchar(10),
    phone        varchar(50),
    email        varchar(255),
    constraint pk_clients primary key (id)
);

create table if not exists dialogs.service_clients
(
    id         uuid                        default uuid_generate_v4() not null,
    created    timestamp(6) with time zone default now()              not null,
    updated    timestamp(6) with time zone,
    service_id uuid                                                   not null,
    client_id  uuid                                                   not null,
    blocked    boolean                     default false              not null,
    restricted boolean                     default false              not null,
    constraint pk_service_clients primary key (id),
    constraint fk_service_clients_on_service_id foreign key (service_id) references authors.services on delete restrict,
    constraint fk_service_clients_on_client_id foreign key (client_id) references dialogs.clients on delete cascade,
    constraint uk_service_clients_service_id_client_id unique (service_id, client_id)
);

create table if not exists dialogs.dialogs
(
    id              uuid                        default uuid_generate_v4() not null,
    created         timestamp(6) with time zone default now()              not null,
    updated         timestamp(6) with time zone,
    service_id      uuid                                                   not null,
    client_id       uuid                                                   not null,
    type            varchar(50)                                            not null,
    status          varchar(50)                 default 'active'           not null,
    last_message_at timestamp(6) with time zone,
    removed         boolean                     default false              not null,
    constraint pk_dialogs primary key (id),
    constraint fk_dialogs_on_service_id_client_id foreign key (service_id, client_id) references dialogs.service_clients (service_id, client_id) on delete cascade,
    constraint dialogs_type_check check (type in ('private', 'group')),
    constraint dialogs_status_check check (status in ('active', 'client_blocked_bot'))
);

create table if not exists dialogs.messages
(
    id              uuid                        default uuid_generate_v4() not null,
    created         timestamp(6) with time zone default now()              not null,
    updated         timestamp(6) with time zone,
    dialog_id       uuid                                                   not null,
    client_id       uuid,
    direction       varchar(50)                                            not null,
    text            text                                                   not null,
    external_id     varchar(255),
    sent_at         timestamp(6) with time zone,
    delivery_status varchar(50),
    constraint pk_messages primary key (id),
    constraint fk_messages_on_dialog_id foreign key (dialog_id) references dialogs.dialogs on delete cascade,
    constraint fk_messages_on_client_id foreign key (client_id) references dialogs.clients on delete cascade,
    constraint messages_direction_check check (direction in ('inbound', 'outbound')),
    constraint messages_client_id_check check ((direction = 'inbound') = (client_id is not null)),
    constraint messages_delivery_status_check check ((direction = 'outbound') = (delivery_status is not null) and delivery_status in ('pending', 'sent', 'failed'))
);

create index if not exists idx_service_clients_client_id on dialogs.service_clients (client_id);
create index if not exists idx_service_clients_service_id_created on dialogs.service_clients (service_id, created, client_id);
create index if not exists idx_dialogs_service_id_client_id on dialogs.dialogs (service_id, client_id);
create index if not exists idx_dialogs_service_id_last_message_at on dialogs.dialogs (service_id, last_message_at desc nulls last) where not removed;
create index if not exists idx_dialogs_service_id_status on dialogs.dialogs (service_id, status) where not removed;
create index if not exists idx_messages_dialog_id_created on dialogs.messages (dialog_id, created desc);
create index if not exists idx_messages_client_id on dialogs.messages (client_id);
create unique index if not exists uk_messages_dialog_id_external_id on dialogs.messages (dialog_id, external_id) where direction = 'inbound' and external_id is not null;
