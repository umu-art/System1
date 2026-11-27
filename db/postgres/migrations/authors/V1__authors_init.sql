CREATE SCHEMA authors;

CREATE TABLE authors.author (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username      text NOT NULL,
    first_name    text NOT NULL,
    last_name     text,
    email         text NOT NULL,
    password_hash text NOT NULL,
    created       timestamptz NOT NULL DEFAULT now(),
    removed       boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX author_username_key ON authors.author (lower(username)) WHERE NOT removed;
CREATE UNIQUE INDEX author_email_key ON authors.author (lower(email)) WHERE NOT removed;

CREATE TYPE authors.service_status AS ENUM ('active', 'disabled');

CREATE TABLE authors.service (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id   uuid NOT NULL REFERENCES authors.author (id),
    name        text NOT NULL,
    description text,
    status      authors.service_status NOT NULL DEFAULT 'active',
    created     timestamptz NOT NULL DEFAULT now(),
    removed     boolean NOT NULL DEFAULT false
);

CREATE INDEX service_author_id_idx ON authors.service (author_id) WHERE NOT removed;
