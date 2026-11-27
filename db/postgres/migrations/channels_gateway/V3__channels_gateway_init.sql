CREATE SCHEMA channels_gateway;

CREATE TYPE channels_gateway.channel_platform AS ENUM ('telegram', 'whatsapp', 'vk', 'max');
CREATE TYPE channels_gateway.channel_status AS ENUM ('active', 'disabled');

CREATE TABLE channels_gateway.channel (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id      uuid NOT NULL REFERENCES authors.service (id),
    platform        channels_gateway.channel_platform NOT NULL,
    token           bytea,
    external_bot_id text NOT NULL,
    bot_username    text NOT NULL,
    status          channels_gateway.channel_status NOT NULL DEFAULT 'active',
    created         timestamptz NOT NULL DEFAULT now(),
    removed         boolean NOT NULL DEFAULT false,
    CONSTRAINT channel_token_check CHECK (removed = (token IS NULL))
);

CREATE UNIQUE INDEX channel_platform_external_bot_id_key ON channels_gateway.channel (platform, external_bot_id) WHERE NOT removed;
CREATE INDEX channel_service_created_idx ON channels_gateway.channel (service_id, created DESC) WHERE NOT removed;

CREATE TABLE channels_gateway.channel_binding (
    channel_id       uuid NOT NULL REFERENCES channels_gateway.channel (id),
    external_chat_id text NOT NULL,
    client_id        uuid REFERENCES dialogs.client (id),
    dialog_id        uuid REFERENCES dialogs.dialog (id),
    created          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, external_chat_id)
);

CREATE INDEX channel_binding_dialog_id_idx ON channels_gateway.channel_binding (dialog_id);
