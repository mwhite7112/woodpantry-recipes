-- name: CreateIngestionJob :one
INSERT INTO ingestion_jobs (type, raw_input)
VALUES ($1, $2)
RETURNING id, type, raw_input, status, staged_data, created_at;

-- name: GetIngestionJob :one
SELECT id, type, raw_input, status, staged_data, created_at
FROM ingestion_jobs WHERE id = $1;

-- name: UpdateIngestionJobStatus :one
UPDATE ingestion_jobs
SET status = $2
WHERE id = $1
RETURNING id, type, raw_input, status, staged_data, created_at;

-- name: UpdateIngestionJobStaged :one
UPDATE ingestion_jobs
SET status = 'staged', staged_data = $2
WHERE id = $1
RETURNING id, type, raw_input, status, staged_data, created_at;
