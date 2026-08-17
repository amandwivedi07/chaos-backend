DROP INDEX IF EXISTS idx_conversations_group;
ALTER TABLE conversations DROP COLUMN IF EXISTS direct;
ALTER TABLE conversations DROP COLUMN IF EXISTS group_id;
DROP TABLE IF EXISTS group_memory;
DROP TABLE IF EXISTS group_members;
DROP TABLE IF EXISTS groups;
