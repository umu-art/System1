-- name: GetServiceClient :one
select c.id as client_id,
       sc.service_id,
       c.display_name,
       c.language,
       c.phone,
       c.email,
       sc.blocked,
       sc.restricted,
       sc.created
from dialogs.service_clients sc
         join dialogs.clients c on c.id = sc.client_id
where sc.client_id = @client_id
  and sc.service_id = @service_id;
