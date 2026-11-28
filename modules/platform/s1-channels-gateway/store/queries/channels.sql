-- name: GetChannel :one
select id,
       created,
       updated,
       service_id,
       platform,
       external_bot_id,
       bot_username,
       status
from channels_gateway.channels
where id = @id
  and not removed;
