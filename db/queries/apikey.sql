-- name: GetAPIKeyByHash :one
SELECT id, key_hash, name, scopes
FROM api_keys
WHERE key_hash = $1 AND active = TRUE;

-- name: UpsertAPIKey :exec
INSERT INTO api_keys (id, key_hash, name, scopes, active)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
    key_hash = EXCLUDED.key_hash,
    name = EXCLUDED.name,
    scopes = EXCLUDED.scopes,
    active = EXCLUDED.active;
