-- Add expanded sentiment columns to sentiment_stats
ALTER TABLE sentiment_stats ADD COLUMN excited INTEGER DEFAULT 0;
ALTER TABLE sentiment_stats ADD COLUMN sarcastic INTEGER DEFAULT 0;
ALTER TABLE sentiment_stats ADD COLUMN frustrated INTEGER DEFAULT 0;
ALTER TABLE sentiment_stats ADD COLUMN curious INTEGER DEFAULT 0;
ALTER TABLE sentiment_stats ADD COLUMN grateful INTEGER DEFAULT 0;
ALTER TABLE sentiment_stats ADD COLUMN humorous INTEGER DEFAULT 0;
ALTER TABLE sentiment_stats ADD COLUMN supportive INTEGER DEFAULT 0;

-- Create generic API cache table
CREATE TABLE IF NOT EXISTS api_cache (
	cache_key TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
);
