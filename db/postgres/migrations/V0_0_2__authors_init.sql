create schema if not exists authors;

create table if not exists authors.authors
(
    id            uuid                        default uuid_generate_v4() not null,
    created       timestamp(6) with time zone default now()              not null,
    updated       timestamp(6) with time zone,
    username      varchar(255)                                           not null,
    first_name    varchar(255)                                           not null,
    last_name     varchar(255),
    email         varchar(255)                                           not null,
    password_hash varchar(128)                                           not null,
    removed       boolean                     default false              not null,
    constraint pk_authors primary key (id)
);

create table if not exists authors.services
(
    id          uuid                        default uuid_generate_v4() not null,
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
