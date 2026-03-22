#!/usr/bin/env bash
# Backfill room_sentiment_stats from existing llm_classifications data.
# Run once after deploying the room_sentiment_stats schema change.
#
# Usage: ./scripts/backfill_room_sentiment.sh [path/to/gogobee.db]

set -euo pipefail

DB="${1:-data/gogobee.db}"

if [ ! -f "$DB" ]; then
    echo "Database not found: $DB"
    exit 1
fi

sqlite3 "$DB" <<'SQL'
INSERT INTO room_sentiment_stats (room_id, positive, negative, neutral, excited, sarcastic, frustrated, curious, grateful, humorous, supportive, total_score)
SELECT room_id,
       SUM(CASE WHEN sentiment = 'positive' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'negative' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'neutral' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'excited' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'sarcastic' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'frustrated' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'curious' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'grateful' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'humorous' THEN 1 ELSE 0 END),
       SUM(CASE WHEN sentiment = 'supportive' THEN 1 ELSE 0 END),
       SUM(sentiment_score)
FROM llm_classifications
GROUP BY room_id;
SQL

echo "Done. Backfilled room_sentiment_stats from llm_classifications."
