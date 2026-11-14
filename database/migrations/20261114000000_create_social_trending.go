package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261114000000CreateSocialTrending adds the weekly social trending ranking (SDD 14 §12.13): the 10 most active
// reviews and the 10 most active items of the last 7 days, written by the job social.compute_trending on Sunday at
// 03:00 (America/Sao_Paulo) and read by /trending. Rows of a week are replaced as a set, so a retry rewrites the
// same rows; `week` is the Sunday date in that zone. `kind = 'item'` rows have no review_id. Foreign keys cascade, so
// a deleted review or item leaves a gap in the ranks instead of a dangling row; hidden reviews and banned authors
// are filtered again at read time. No column holds user data, so the account purge needs no entry; the prune of
// trending snapshots (04:45) also deletes weeks older than 12.
//
// Serves: the latest-week lookup (the primary key prefix) and the per-kind read ordered by rank (the primary key).
func m20261114000000CreateSocialTrending() schema.Migration {
	return postgresMigration("20261114000000_create_social_trending",
		[]string{
			`CREATE TABLE social_trending (
  week date NOT NULL,
  kind text NOT NULL CHECK (kind IN ('item', 'review')),
  rank smallint NOT NULL CHECK (rank BETWEEN 1 AND 20),
  item_id uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  review_id uuid REFERENCES reviews(id) ON DELETE CASCADE,
  score real NOT NULL CHECK (score > 0),
  reviews_n integer NOT NULL DEFAULT 0,
  likes_n integer NOT NULL DEFAULT 0,
  comments_n integer NOT NULL DEFAULT 0,
  computed_at timestamptz NOT NULL,
  PRIMARY KEY (week, kind, rank),
  CHECK ((kind = 'review') = (review_id IS NOT NULL)))`,
			`CREATE UNIQUE INDEX social_trending_item_uq ON social_trending (week, item_id) WHERE kind = 'item'`,
			`CREATE UNIQUE INDEX social_trending_review_uq ON social_trending (week, review_id) WHERE kind = 'review'`,
			`CREATE INDEX social_trending_review_fk_idx ON social_trending (review_id) WHERE review_id IS NOT NULL`,
			`CREATE INDEX social_trending_item_fk_idx ON social_trending (item_id)`,
		},
		[]string{`DROP TABLE IF EXISTS social_trending`},
	)
}
