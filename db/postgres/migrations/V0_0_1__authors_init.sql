create schema if not exists authors;

create table if not exists authors.authors
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
    constraint pk_authors primary key (id)
);

create table if not exists authors.services
(
    id          uuid                        default gen_random_uuid()  not null,
    created     timestamp(6) with time zone default now()              not null,
    updated     timestamp(6) with time zone,
    author_id   uuid                                                   not null,
    name        varchar(1024)                                          not null,
    description text,
    status      varchar(50)                 default 'active'           not null,
    removed     boolean                     default false              not null,
    constraint pk_services primary key (id),
    constraint fk_services_on_author_id foreign key (author_id) references authors.authors on delete cascade,
    constraint services_status_check check (status in ('active', 'disabled'))
);

create unique index if not exists uk_authors_username on authors.authors (lower(username)) where not removed;
create unique index if not exists uk_authors_email on authors.authors (lower(email)) where not removed;
create index if not exists idx_services_author_id on authors.services (author_id);

comment on table authors.authors is 'Авторы — люди, которые создают и ведут сервисы на платформе.';
comment on column authors.authors.id is 'Идентификатор автора.';
comment on column authors.authors.created is 'Когда автор зарегистрировался.';
comment on column authors.authors.updated is 'Когда запись последний раз изменялась.';
comment on column authors.authors.username is 'Логин автора. Уникален без учёта регистра среди неудалённых авторов.';
comment on column authors.authors.first_name is 'Имя автора.';
comment on column authors.authors.last_name is 'Фамилия автора.';
comment on column authors.authors.email is 'Email автора. Уникален без учёта регистра среди неудалённых авторов.';
comment on column authors.authors.password_hash is 'Хэш пароля. Сам пароль не хранится.';
comment on column authors.authors.removed is 'Автор удалён (мягкое удаление).';

comment on table authors.services is 'Сервисы — то, что автор запускает на платформе, например «Дед Мороз».';
comment on column authors.services.id is 'Идентификатор сервиса.';
comment on column authors.services.created is 'Когда сервис создан.';
comment on column authors.services.updated is 'Когда запись последний раз изменялась.';
comment on column authors.services.author_id is 'Автор, которому принадлежит сервис.';
comment on column authors.services.name is 'Название сервиса.';
comment on column authors.services.description is 'Описание сервиса.';
comment on column authors.services.status is 'Состояние сервиса: active — работает, disabled — выключен.';
comment on column authors.services.removed is 'Сервис удалён (мягкое удаление).';
