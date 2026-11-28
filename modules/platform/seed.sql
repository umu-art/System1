-- Пароль тестового владельца: test-password
INSERT INTO owners.owners (id, username, first_name, last_name, email, password_hash)
VALUES ('00000000-0000-0000-0000-000000000001', 'test_owner', 'Тест', 'Владелец', 'test-owner@example.com', '$2a$10$FmBp9d11KUSeDd/40PguzO3RCRnNvW9AYWAUCEwu7SGigKYKfPpJO')
ON CONFLICT (id) DO NOTHING;

INSERT INTO owners.services (id, owner_id, name, description)
VALUES ('00000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001', 'Тестовый сервис', 'Сервис для локальной проверки')
ON CONFLICT (id) DO NOTHING;
