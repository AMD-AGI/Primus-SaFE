-- Patch: Change skill_embeddings vector dimension from 1536 to 1024
-- This supports BGE-M3 model which uses 1024 dimensions

-- Drop existing data (embeddings need to be regenerated with new model anyway)
TRUNCATE TABLE skill_embeddings;

-- Alter column to use 1024 dimensions
ALTER TABLE skill_embeddings 
ALTER COLUMN embedding TYPE vector(1024);
