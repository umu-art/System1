CREATE SCHEMA dialogs;

CREATE TYPE dialogs.dialog_type AS ENUM ('private', 'group');
CREATE TYPE dialogs.dialog_status AS ENUM ('active', 'client_blocked_bot');
CREATE TYPE dialogs.message_direction AS ENUM ('inbound', 'outbound');
CREATE TYPE dialogs.message_delivery_status AS ENUM ('pending', 'sent', 'failed');

CREATE TABLE dialogs.client (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name text NOT NULL,
    language     text,
    phone        text,
    email        text,
    created      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE dialogs.service_client (
    service_id uuid NOT NULL REFERENCES authors.service (id),
    client_id  uuid NOT NULL REFERENCES dialogs.client (id),
    blocked    boolean NOT NULL DEFAULT false,
    restricted boolean NOT NULL DEFAULT false,
    created    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, client_id)
);

CREATE INDEX service_client_service_created_idx ON dialogs.service_client (service_id, created, client_id);

CREATE TABLE dialogs.dialog (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id      uuid NOT NULL,
    client_id       uuid NOT NULL,
    type            dialogs.dialog_type NOT NULL,
    status          dialogs.dialog_status NOT NULL DEFAULT 'active',
    last_message_at timestamptz,
    created         timestamptz NOT NULL DEFAULT now(),
    removed         boolean NOT NULL DEFAULT false,
    FOREIGN KEY (service_id, client_id) REFERENCES dialogs.service_client (service_id, client_id)
);

CREATE INDEX dialog_service_last_message_idx ON dialogs.dialog (service_id, last_message_at DESC NULLS LAST) WHERE NOT removed;
CREATE INDEX dialog_service_client_idx ON dialogs.dialog (service_id, client_id) WHERE NOT removed;

CREATE TABLE dialogs.message (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    dialog_id       uuid NOT NULL REFERENCES dialogs.dialog (id),
    client_id       uuid REFERENCES dialogs.client (id),
    direction       dialogs.message_direction NOT NULL,
    text            text NOT NULL,
    external_id     text,
    sent_at         timestamptz,
    delivery_status dialogs.message_delivery_status,
    created         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT message_sender_check CHECK ((direction = 'inbound') = (client_id IS NOT NULL)),
    CONSTRAINT message_delivery_status_check CHECK ((direction = 'outbound') = (delivery_status IS NOT NULL))
);

CREATE INDEX message_dialog_created_idx ON dialogs.message (dialog_id, created DESC);
CREATE UNIQUE INDEX message_inbound_external_id_key ON dialogs.message (dialog_id, external_id)
    WHERE direction = 'inbound' AND external_id IS NOT NULL;
