create schema if not exists channels_gateway;

create table if not exists channels_gateway.channels
(
    id              uuid                        default gen_random_uuid()  not null,
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
    constraint channels_token_check check (removed = (token is null))
);

create table if not exists channels_gateway.channel_bindings
(
    id               uuid                        default gen_random_uuid()  not null,
    created          timestamp(6) with time zone default now()              not null,
    updated          timestamp(6) with time zone,
    channel_id       uuid                                                   not null,
    external_chat_id varchar(255)                                           not null,
    external_user_id varchar(255)                                           not null,
    client_id        uuid,
    dialog_id        uuid,
    constraint pk_channel_bindings primary key (id),
    constraint fk_channel_bindings_on_channel_id foreign key (channel_id) references channels_gateway.channels on delete cascade,
    constraint fk_channel_bindings_on_client_id foreign key (client_id) references dialogs.clients on delete restrict,
    constraint fk_channel_bindings_on_dialog_id foreign key (dialog_id) references dialogs.dialogs on delete restrict,
    constraint uk_channel_bindings_channel_id_chat_id_user_id unique (channel_id, external_chat_id, external_user_id)
);

create unique index if not exists uk_channels_platform_external_bot_id on channels_gateway.channels (platform, external_bot_id) where not removed;
create index if not exists idx_channels_service_id_created on channels_gateway.channels (service_id, created desc);
create index if not exists idx_channel_bindings_channel_id_external_user_id on channels_gateway.channel_bindings (channel_id, external_user_id);
create index if not exists idx_channel_bindings_dialog_id on channels_gateway.channel_bindings (dialog_id);

comment on table channels_gateway.channels is 'Каналы — бот сервиса на конкретной платформе.';
comment on column channels_gateway.channels.id is 'Идентификатор канала.';
comment on column channels_gateway.channels.created is 'Когда канал создан.';
comment on column channels_gateway.channels.updated is 'Когда запись последний раз изменялась.';
comment on column channels_gateway.channels.service_id is 'Сервис, которому принадлежит канал.';
comment on column channels_gateway.channels.platform is 'Платформа бота: telegram, whatsapp, vk, max.';
comment on column channels_gateway.channels.token is 'Токен бота в зашифрованном виде.';
comment on column channels_gateway.channels.external_bot_id is 'Идентификатор бота на платформе (в Telegram — id из getMe). Не меняется при смене токена.';
comment on column channels_gateway.channels.bot_username is 'Имя бота на платформе, по нему клиенты находят бота.';
comment on column channels_gateway.channels.status is 'Состояние канала: active — работает, disabled — выключен.';
comment on column channels_gateway.channels.removed is 'Канал удалён (мягкое удаление).';

comment on table channels_gateway.channel_bindings is 'Связь пользователя в чате на платформе с клиентом и диалогом. В личном чате chat ID и user ID обычно совпадают, в групповом на каждого участника своя строка.';
comment on column channels_gateway.channel_bindings.id is 'Идентификатор связи.';
comment on column channels_gateway.channel_bindings.created is 'Когда шлюз впервые увидел этого пользователя в этом чате.';
comment on column channels_gateway.channel_bindings.updated is 'Когда запись последний раз изменялась.';
comment on column channels_gateway.channel_bindings.channel_id is 'Канал, в котором идёт чат.';
comment on column channels_gateway.channel_bindings.external_chat_id is 'Идентификатор чата на платформе (в Telegram — chat.id).';
comment on column channels_gateway.channel_bindings.external_user_id is 'Идентификатор пользователя на платформе (в Telegram — from.id).';
comment on column channels_gateway.channel_bindings.client_id is 'Клиент этого пользователя. Пусто, пока клиент ещё не создан.';
comment on column channels_gateway.channel_bindings.dialog_id is 'Текущий диалог этого чата. Пусто, пока диалог не создан или после того, как автор его удалил.';
