create schema if not exists channels_gateway;

create table if not exists channels_gateway.channels
(
    id              uuid                        default uuid_generate_v4() not null,
    created         timestamp(6) with time zone default now()              not null,
    updated         timestamp(6) with time zone,
    service_id      uuid                                                   not null,
    platform        varchar(50)                                            not null,
    token           bytea,
    external_bot_id varchar(255)                                           not null,
    bot_username    varchar(255)                                           not null,
    status          varchar(50)                 default 'active'           not null,
    removed         boolean                     default false              not null,
    constraint pk_channels primary key (id),
    constraint fk_channels_on_service_id foreign key (service_id) references authors.services on delete restrict,
    constraint channels_platform_check check (platform in ('telegram', 'whatsapp', 'vk', 'max')),
    constraint channels_status_check check (status in ('active', 'disabled')),
    constraint channels_token_check check (removed = (token is null))
);

create table if not exists channels_gateway.channel_bindings
(
    id               uuid                        default uuid_generate_v4() not null,
    created          timestamp(6) with time zone default now()              not null,
    updated          timestamp(6) with time zone,
    channel_id       uuid                                                   not null,
    external_chat_id varchar(255)                                           not null,
    client_id        uuid,
    dialog_id        uuid,
    constraint pk_channel_bindings primary key (id),
    constraint fk_channel_bindings_on_channel_id foreign key (channel_id) references channels_gateway.channels on delete cascade,
    constraint fk_channel_bindings_on_client_id foreign key (client_id) references dialogs.clients on delete restrict,
    constraint fk_channel_bindings_on_dialog_id foreign key (dialog_id) references dialogs.dialogs on delete restrict,
    constraint uk_channel_bindings_channel_id_external_chat_id unique (channel_id, external_chat_id)
);

create unique index if not exists uk_channels_platform_external_bot_id on channels_gateway.channels (platform, external_bot_id) where not removed;
create index if not exists idx_channels_service_id_created on channels_gateway.channels (service_id, created desc);
create index if not exists idx_channel_bindings_client_id on channels_gateway.channel_bindings (client_id);
create index if not exists idx_channel_bindings_dialog_id on channels_gateway.channel_bindings (dialog_id);
