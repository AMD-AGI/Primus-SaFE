-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- Add job tracking fields to dataplane_install_tasks

ALTER TABLE dataplane_install_tasks
ADD COLUMN IF NOT EXISTS job_name VARCHAR(255) DEFAULT '',
ADD COLUMN IF NOT EXISTS job_namespace VARCHAR(255) DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_install_tasks_job ON dataplane_install_tasks(job_name, job_namespace);

-- +goose Down
DROP INDEX IF EXISTS idx_install_tasks_job;
ALTER TABLE dataplane_install_tasks
DROP COLUMN IF EXISTS job_name,
DROP COLUMN IF EXISTS job_namespace;
