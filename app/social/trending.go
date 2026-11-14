package social

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem/database"

	"github.com/AlexGalhardo/hexascore-app/app/catalog"
)

// Weekly social trending (SDD 14 §12.13). Every event of the last 7 days on a review adds its weight, halved for
// each TrendingHalfLife it is old, so a comment from yesterday outweighs a like from last Monday. A review scores
// its own publication, the reactions on it and the comments on it by other members; an item scores the sum of its
// reviews.
const (
	// TrendingWindow is how far back the job looks.
	TrendingWindow = 7 * 24 * time.Hour
	// TrendingHalfLife is the age at which an event counts half.
	TrendingHalfLife = 72 * time.Hour
	// WeightReview, WeightComment and WeightReaction are the points of one event at age zero. A `helpful` counts as
	// a like; the author's own comments on their review are not counted, so a thread cannot pump itself.
	WeightReview   = 3.0
	WeightComment  = 2.0
	WeightReaction = 1.0
	// TrendingSize is the number of ranks per kind kept for a week.
	TrendingSize = 10
	// TrendingKeepWeeks is how many weeks the prune keeps.
	TrendingKeepWeeks = 12
)

// Zone is the zone of the schedule: the weekly job runs on Sunday at 03:00 here, and `week` is the Sunday date here.
// Brazil has had no DST since 2019, so the fixed fallback is exact when the image lacks tzdata.
var Zone = func() *time.Location {
	if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return loc
	}

	return time.FixedZone("BRT", -3*60*60)
}()

// WeekOf returns the date of the Sunday that starts the week of t in Zone, as YYYY-MM-DD.
func WeekOf(t time.Time) string {
	local := t.In(Zone)

	return local.AddDate(0, 0, -int(local.Weekday())).Format(time.DateOnly)
}

// scoredReviews is the per-review score of the window: $1 window start, $2 now, $3 half-life in seconds, $4 to $6
// the weights of a review, a comment and a reaction. Only reviews a visitor may see count (§12.5): not hidden, rating
// still there, public item, public and active author.
const scoredReviews = `scored AS (
  SELECT a.review_id, v.item_id,
    sum(a.w * power(0.5::float8, extract(epoch FROM ($2::timestamptz - a.at))::float8 / $3::float8)) AS score,
    count(*) FILTER (WHERE a.kind = 'review') AS reviews_n,
    count(*) FILTER (WHERE a.kind = 'reaction') AS likes_n,
    count(*) FILTER (WHERE a.kind = 'comment') AS comments_n
  FROM (
    SELECT id AS review_id, 'review' AS kind, created_at AS at, $4::float8 AS w FROM reviews
    UNION ALL
    SELECT review_id, 'reaction', created_at, $6::float8 FROM reactions WHERE review_id IS NOT NULL
    UNION ALL
    SELECT c.review_id, 'comment', c.created_at, $5::float8 FROM comments c
      JOIN reviews rv ON rv.id = c.review_id
      WHERE NOT c.hidden AND c.user_id <> rv.user_id
  ) a
  JOIN reviews v ON v.id = a.review_id AND NOT v.hidden
  JOIN ratings r ON r.user_id = v.user_id AND r.item_id = v.item_id
  JOIN users u ON u.id = v.user_id
  JOIN items i ON i.id = v.item_id AND i.visibility = 'public'
  WHERE a.at >= $1 AND a.at <= $2 AND ` + visible + `
  GROUP BY a.review_id, v.item_id)`

const insertTrending = `INSERT INTO social_trending
  (week, kind, rank, item_id, review_id, score, reviews_n, likes_n, comments_n, computed_at) `

// ComputeTrending rebuilds the ranking of the week of now: the TrendingSize best reviews and items of the
// TrendingWindow ending at now. It replaces the week's rows in one transaction, so a retry or a manual rerun leaves
// the same rows and readers never see a half-written week.
func ComputeTrending(ctx context.Context, db *database.DB, now time.Time) error {
	now = now.UTC()
	week := WeekOf(now)
	args := func() []any {
		return []any{
			now.Add(-TrendingWindow), now, TrendingHalfLife.Seconds(),
			WeightReview, WeightComment, WeightReaction, week, TrendingSize,
		}
	}

	return db.Tx(ctx, func(tx *database.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM social_trending WHERE week = $1::date`, week); err != nil {
			return fmt.Errorf("social: clear trending week: %w", err)
		}

		if _, err := tx.ExecContext(ctx, insertTrending+`WITH `+scoredReviews+`
SELECT $7::date, 'review', row_number() OVER (ORDER BY score DESC, review_id DESC), item_id, review_id, score,
  reviews_n, likes_n, comments_n, $2::timestamptz
FROM scored ORDER BY score DESC, review_id DESC LIMIT $8`, args()...); err != nil {
			return fmt.Errorf("social: rank trending reviews: %w", err)
		}

		if _, err := tx.ExecContext(ctx, insertTrending+`WITH `+scoredReviews+`
SELECT $7::date, 'item', row_number() OVER (ORDER BY sum(score) DESC, item_id DESC), item_id, NULL::uuid, sum(score),
  sum(reviews_n), sum(likes_n), sum(comments_n), $2::timestamptz
FROM scored GROUP BY item_id ORDER BY sum(score) DESC, item_id DESC LIMIT $8`, args()...); err != nil {
			return fmt.Errorf("social: rank trending items: %w", err)
		}

		return nil
	})
}

// PruneTrending deletes the weeks older than TrendingKeepWeeks at now.
func PruneTrending(ctx context.Context, db *database.DB, now time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM social_trending WHERE week < $1::date`,
		now.In(Zone).AddDate(0, 0, -7*TrendingKeepWeeks).Format(time.DateOnly))
	if err != nil {
		return 0, fmt.Errorf("social: prune trending: %w", err)
	}

	return res.RowsAffected()
}

// TrendingItem is one ranked item with the activity that put it there.
type TrendingItem struct {
	Rank     int      `json:"rank"`
	Score    float64  `json:"score"`
	Reviews  int      `json:"reviews"`
	Likes    int      `json:"likes"`
	Comments int      `json:"comments"`
	Item     FeedItem `json:"item"`
}

// TrendingReview is one ranked review.
type TrendingReview struct {
	Rank     int     `json:"rank"`
	Score    float64 `json:"score"`
	Likes    int     `json:"likes"`
	Comments int     `json:"comments"`
	Review   Review  `json:"review"`
}

// Trending is the latest ranking; Week is "" before the first run.
type Trending struct {
	Week       string           `json:"week"`
	ComputedAt *time.Time       `json:"computedAt"`
	Items      []TrendingItem   `json:"items"`
	Reviews    []TrendingReview `json:"reviews"`
}

// Trending reads the latest computed week, dropping what stopped being visible since the job ran.
func (s *Follows) Trending(ctx context.Context, locale string) (Trending, error) {
	out := Trending{Items: []TrendingItem{}, Reviews: []TrendingReview{}}

	var week *string
	if err := s.DB.QueryRowContext(ctx, `SELECT max(week)::text, max(computed_at) FROM social_trending`).
		Scan(&week, &out.ComputedAt); err != nil {
		return out, fmt.Errorf("social: trending week: %w", err)
	}

	if week == nil {
		return out, nil
	}

	out.Week = *week

	rows, err := s.DB.QueryContext(ctx, `SELECT t.rank, t.score, t.reviews_n, t.likes_n, t.comments_n,
  i.id, i.type, i.slug, coalesce(tr.title, te.title, i.original_title, i.slug), i.cover_thumb_key
FROM social_trending t
JOIN items i ON i.id = t.item_id AND i.visibility = 'public'
LEFT JOIN item_translations tr ON tr.item_id = i.id AND tr.locale = $2
LEFT JOIN item_translations te ON te.item_id = i.id AND te.locale = 'en'
WHERE t.week = $1::date AND t.kind = 'item' ORDER BY t.rank`, out.Week, locale)
	if err != nil {
		return out, fmt.Errorf("social: trending items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			it    TrendingItem
			cover *string
		)

		if err := rows.Scan(&it.Rank, &it.Score, &it.Reviews, &it.Likes, &it.Comments, &it.Item.ID, &it.Item.Type,
			&it.Item.Slug, &it.Item.Title, &cover); err != nil {
			return out, fmt.Errorf("social: trending item row: %w", err)
		}

		it.Item.Cover = catalog.CoverFromThumbKey(cover)
		out.Items = append(out.Items, it)
	}

	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("social: trending item rows: %w", err)
	}

	out.Reviews, err = s.trendingReviews(ctx, out.Week, locale)

	return out, err
}

func (s *Follows) trendingReviews(ctx context.Context, week, locale string) ([]TrendingReview, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT review_id::text, rank, score, likes_n, comments_n
FROM social_trending WHERE week = $1::date AND kind = 'review' ORDER BY rank`, week)
	if err != nil {
		return nil, fmt.Errorf("social: trending reviews: %w", err)
	}
	defer rows.Close()

	var (
		ids  []string
		rank = map[string]TrendingReview{}
	)

	for rows.Next() {
		var (
			id string
			tr TrendingReview
		)

		if err := rows.Scan(&id, &tr.Rank, &tr.Score, &tr.Likes, &tr.Comments); err != nil {
			return nil, fmt.Errorf("social: trending review row: %w", err)
		}

		ids, rank[id] = append(ids, id), tr
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("social: trending review rows: %w", err)
	}

	out := []TrendingReview{}
	if len(ids) == 0 {
		return out, nil
	}

	cards, err := s.reviews(ctx, 0, reviewQuery(`v.id = ANY($2::uuid[]) AND i.visibility = 'public' AND (`+visible+`)`),
		locale, "{"+strings.Join(ids, ",")+"}")
	if err != nil {
		return nil, err
	}

	byID := map[string]Review{}
	for _, c := range cards {
		byID[c.ID] = c
	}

	for _, id := range ids {
		if c, ok := byID[id]; ok {
			tr := rank[id]
			tr.Review = c
			out = append(out, tr)
		}
	}

	return out, nil
}
