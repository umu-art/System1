create schema if not exists dialogs;

create table if not exists dialogs.clients
(
    id           uuid                        default gen_random_uuid()  not null,
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
    id         uuid                        default gen_random_uuid()  not null,
    created    timestamp(6) with time zone default now()              not null,
    updated    timestamp(6) with time zone,
    service_id uuid                                                   not null,
    client_id  uuid                                                   not null,
    blocked    boolean                     default false              not null,
    restricted boolean                     default false              not null,
    constraint pk_service_clients primary key (id),
    constraint fk_service_clients_on_service_id foreign key (service_id) references owners.services on delete restrict,
    constraint fk_service_clients_on_client_id foreign key (client_id) references dialogs.clients on delete cascade,
    constraint uk_service_clients_service_id_client_id unique (service_id, client_id)
);

create table if not exists dialogs.dialogs
(
    id              uuid                        default gen_random_uuid()  not null,
    created         timestamp(6) with time zone default now()              not null,
    updated         timestamp(6) with time zone,
    service_id      uuid                                                   not null,
    type            varchar(50)                                            not null,
    status          varchar(50)                 default 'active'           not null,
    last_message_at timestamp(6) with time zone,
    removed         boolean                     default false              not null,
    constraint pk_dialogs primary key (id),
    constraint fk_dialogs_on_service_id foreign key (service_id) references owners.services on delete restrict
);

create table if not exists dialogs.dialog_clients
(
    id        uuid                        default gen_random_uuid()  not null,
    created   timestamp(6) with time zone default now()              not null,
    updated   timestamp(6) with time zone,
    dialog_id uuid                                                   not null,
    client_id uuid                                                   not null,
    removed   boolean                     default false              not null,
    constraint pk_dialog_clients primary key (id),
    constraint fk_dialog_clients_on_dialog_id foreign key (dialog_id) references dialogs.dialogs on delete cascade,
    constraint fk_dialog_clients_on_client_id foreign key (client_id) references dialogs.clients on delete cascade,
    constraint uk_dialog_clients_dialog_id_client_id unique (dialog_id, client_id)
);

create table if not exists dialogs.messages
(
    id                uuid                        default gen_random_uuid()  not null,
    created           timestamp(6) with time zone default now()              not null,
    updated           timestamp(6) with time zone,
    dialog_id         uuid                                                   not null,
    client_id         uuid,
    direction         varchar(50)                                            not null,
    text              text                                                   not null,
    external_id       varchar(255),
    sent_at           timestamp(6) with time zone,
    delivery_status   varchar(50),
    processing_status varchar(50),
    taken_at          timestamp(6) with time zone,
    constraint pk_messages primary key (id),
    constraint fk_messages_on_dialog_id foreign key (dialog_id) references dialogs.dialogs on delete cascade,
    constraint fk_messages_on_dialog_id_client_id foreign key (dialog_id, client_id) references dialogs.dialog_clients (dialog_id, client_id) on delete cascade,
    constraint messages_client_id_check check ((direction = 'inbound') = (client_id is not null)),
    constraint messages_delivery_status_check check ((direction = 'outbound') = (delivery_status is not null)),
    constraint messages_processing_status_check check ((direction = 'inbound') = (processing_status is not null))
);

create index if not exists idx_service_clients_client_id on dialogs.service_clients (client_id);
create index if not exists idx_service_clients_service_id_created on dialogs.service_clients (service_id, created, client_id);
create index if not exists idx_dialogs_service_id_last_message_at on dialogs.dialogs (service_id, last_message_at desc nulls last) where not removed;
create index if not exists idx_dialogs_service_id_status on dialogs.dialogs (service_id, status) where not removed;
create index if not exists idx_dialog_clients_client_id on dialogs.dialog_clients (client_id);
create index if not exists idx_messages_dialog_id_created on dialogs.messages (dialog_id, created desc);
create index if not exists idx_messages_processing_status_created on dialogs.messages (processing_status, created) where processing_status in ('pending', 'processing');
create unique index if not exists uk_messages_dialog_id_external_id on dialogs.messages (dialog_id, external_id) where direction = 'inbound' and external_id is not null;

comment on table dialogs.clients is 'Клиенты — люди, которые пишут ботам сервисов. Клиент один на всю систему, в сервисы он входит через service_clients.';
comment on column dialogs.clients.id is 'Идентификатор клиента.';
comment on column dialogs.clients.created is 'Когда клиент создан.';
comment on column dialogs.clients.updated is 'Когда запись последний раз изменялась.';
comment on column dialogs.clients.display_name is 'Отображаемое имя из профиля на платформе.';
comment on column dialogs.clients.language is 'Язык из профиля на платформе.';
comment on column dialogs.clients.phone is 'Телефон, если платформа его передала.';
comment on column dialogs.clients.email is 'Email, если платформа его передала.';

comment on table dialogs.service_clients is 'Клиент в сервисе владельца: связь клиента с сервисом и его статус в этом сервисе.';
comment on column dialogs.service_clients.id is 'Идентификатор связи клиента с сервисом.';
comment on column dialogs.service_clients.created is 'Когда клиент появился в сервисе.';
comment on column dialogs.service_clients.updated is 'Когда запись последний раз изменялась.';
comment on column dialogs.service_clients.service_id is 'Сервис владельца.';
comment on column dialogs.service_clients.client_id is 'Клиент.';
comment on column dialogs.service_clients.blocked is 'Владелец заблокировал клиента в сервисе.';
comment on column dialogs.service_clients.restricted is 'Тариф владельца не позволяет обслуживать клиента.';

comment on table dialogs.dialogs is 'Диалоги клиентов с сервисами. Участники диалога — в dialog_clients.';
comment on column dialogs.dialogs.id is 'Идентификатор диалога.';
comment on column dialogs.dialogs.created is 'Когда диалог создан.';
comment on column dialogs.dialogs.updated is 'Когда запись последний раз изменялась.';
comment on column dialogs.dialogs.service_id is 'Сервис, которому принадлежит диалог.';
comment on column dialogs.dialogs.type is 'Тип диалога.';
comment on column dialogs.dialogs.status is 'Состояние диалога.';
comment on column dialogs.dialogs.last_message_at is 'Время последнего сообщения в диалоге.';
comment on column dialogs.dialogs.removed is 'Диалог удалён владельцем (мягкое удаление).';

comment on table dialogs.dialog_clients is 'Участники диалога.';
comment on column dialogs.dialog_clients.id is 'Идентификатор участника диалога.';
comment on column dialogs.dialog_clients.created is 'Когда клиент стал участником диалога.';
comment on column dialogs.dialog_clients.updated is 'Когда запись последний раз изменялась.';
comment on column dialogs.dialog_clients.dialog_id is 'Диалог.';
comment on column dialogs.dialog_clients.client_id is 'Клиент.';
comment on column dialogs.dialog_clients.removed is 'Клиент вышел из диалога (мягкое удаление).';

comment on table dialogs.messages is 'Сообщения диалогов: и сообщения клиентов, и ответы сервиса.';
comment on column dialogs.messages.id is 'Идентификатор сообщения.';
comment on column dialogs.messages.created is 'Когда сообщение сохранено в системе.';
comment on column dialogs.messages.updated is 'Когда запись последний раз изменялась.';
comment on column dialogs.messages.dialog_id is 'Диалог, к которому относится сообщение.';
comment on column dialogs.messages.client_id is 'Участник диалога, который отправил сообщение. Есть только у входящих сообщений.';
comment on column dialogs.messages.direction is 'Направление сообщения: от клиента или клиенту.';
comment on column dialogs.messages.text is 'Текст сообщения.';
comment on column dialogs.messages.external_id is 'Идентификатор сообщения на платформе. У ответа появляется после успешной отправки.';
comment on column dialogs.messages.sent_at is 'Когда сообщение отправлено на платформе.';
comment on column dialogs.messages.delivery_status is 'Состояние отправки ответа на платформу. Есть только у ответов.';
comment on column dialogs.messages.processing_status is 'Состояние обработки сообщения сценарным движком. Есть только у входящих сообщений.';
comment on column dialogs.messages.taken_at is 'Когда сценарный движок взял сообщение в работу. По нему находят сообщения, которые слишком долго в обработке.';
