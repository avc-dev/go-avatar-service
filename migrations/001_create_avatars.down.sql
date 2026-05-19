DROP TRIGGER IF EXISTS set_avatars_updated_at ON avatars;
DROP FUNCTION IF EXISTS trigger_set_updated_at();
DROP INDEX IF EXISTS idx_avatars_status;
DROP INDEX IF EXISTS idx_avatars_user_id;
DROP TABLE IF EXISTS avatars;
