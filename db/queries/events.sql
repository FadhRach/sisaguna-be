-- name: Notify :exec
SELECT pg_notify('sg_events', @payload::text);
