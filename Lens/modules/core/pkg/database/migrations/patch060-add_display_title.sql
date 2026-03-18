-- Add display_title column to github_workflow_run_summaries
ALTER TABLE github_workflow_run_summaries ADD COLUMN IF NOT EXISTS display_title TEXT;
COMMENT ON COLUMN github_workflow_run_summaries.display_title IS 'The display title of the workflow run (commit message, PR title, etc.)';

-- Add html_url column to github_workflow_jobs if not exists
ALTER TABLE github_workflow_jobs ADD COLUMN IF NOT EXISTS html_url TEXT;
COMMENT ON COLUMN github_workflow_jobs.html_url IS 'GitHub URL for the job';
