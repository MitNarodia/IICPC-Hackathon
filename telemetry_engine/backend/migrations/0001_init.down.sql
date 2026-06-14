-- Track 3 schema, migration 0001 (down). Drops everything 0001_init.up.sql
-- created, in reverse order. Dropping a hypertable is just DROP TABLE.

BEGIN;

DROP INDEX IF EXISTS idx_scores_rank;
DROP INDEX IF EXISTS idx_window_agg_lookup;

DROP TABLE IF EXISTS scores;
DROP TABLE IF EXISTS validation_results;
DROP TABLE IF EXISTS rolling_stats;
DROP TABLE IF EXISTS window_aggregates;
DROP TABLE IF EXISTS submissions_meta;
DROP TABLE IF EXISTS runs;

COMMIT;
