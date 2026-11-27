create schema if not exists owners;

create table if not exists owners.owners
(
    id            uuid                        default gen_random_uuid()  not null,
    created       timestamp(6) with time zone default now()              not null,
    updated       timestamp(6) with time zone,
    username      varchar(255)                                           not null,
    first_name    varchar(255)                                           not null,
    last_name     varchar(255)                                           not null,
    email         varchar(255)                                           not null,
    password_hash varchar(128)                                           not null,
    removed       boolean                     default false              not null,
    constraint pk_owners primary key (id)
);

create table if not exists owners.services
(
    id          uuid                        default gen_random_uuid()  not null,
    created     timestamp(6) with time zone default now()              not null,
    updated     timestamp(6) with time zone,
    owner_id   uuid                                                   not null,
    name        varchar(1024)                                          not null,
    description text,
    status      varchar(50)                 default 'active'           not null,
    removed     boolean                     default false              not null,
    constraint pk_services primary key (id),
    constraint fk_services_on_owner_id foreign key (owner_id) references owners.owners on delete cascade
);

create unique index if not exists uk_owners_username on owners.owners (lower(username)) where not removed;
create unique index if not exists uk_owners_email on owners.owners (lower(email)) where not removed;
create index if not exists idx_services_owner_id on owners.services (owner_id);

comment on table owners.owners is 'Владельцы — люди, которые создают и ведут сервисы на платформе.';
comment on column owners.owners.id is 'Идентификатор владельца.';
comment on column owners.owners.created is 'Когда владелец зарегистрировался.';
comment on column owners.owners.updated is 'Когда запись последний раз изменялась.';
comment on column owners.owners.username is 'Логин владельца. Уникален без учёта регистра среди неудалённых владельцев.';
comment on column owners.owners.first_name is 'Имя владельца.';
comment on column owners.owners.last_name is 'Фамилия владельца.';
comment on column owners.owners.email is 'Email владельца. Уникален без учёта регистра среди неудалённых владельцев.';
comment on column owners.owners.password_hash is 'Хэш пароля. Сам пароль не хранится.';
comment on column owners.owners.removed is 'Владелец удалён (мягкое удаление).';

comment on table owners.services is 'Сервисы — то, что владелец запускает на платформе.';
comment on column owners.services.id is 'Идентификатор сервиса.';
comment on column owners.services.created is 'Когда сервис создан.';
comment on column owners.services.updated is 'Когда запись последний раз изменялась.';
comment on column owners.services.owner_id is 'Владелец, которому принадлежит сервис.';
comment on column owners.services.name is 'Название сервиса.';
comment on column owners.services.description is 'Описание сервиса.';
comment on column owners.services.status is 'Состояние сервиса.';
comment on column owners.services.removed is 'Сервис удалён (мягкое удаление).';
