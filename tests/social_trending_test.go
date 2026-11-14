//go:build integration

package tests

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/AlexGalhardo/hexascore-app/app/jobs"
	"github.com/AlexGalhardo/hexascore-app/app/social"
)

// decay is the reference weight of one event of the given age, the formula of SDD 14 §12.13.
func decay(weight float64, age time.Duration) float64 {
	return weight * math.Pow(0.5, age.Hours()/social.TrendingHalfLife.Hours())
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()

	if math.Abs(got-want) > 0.001 {
		t.Fatalf("%s = %.4f, want %.4f", what, got, want)
	}
}

func TestSocialTrending_WeeklyRanking(t *testing.T) {
	r := newSocialRig(t, true)
	a, ac := r.member("tr-a@example.com", "Alfa", false)
	_, bc := r.member("tr-b@example.com", "Bravo", false)
	_, cc := r.member("tr-c@example.com", "Charlie", false)
	p, pc := r.member("tr-p@example.com", "Delta", true)

	// Sunday 2026-11-15 03:00 in America/Sao_Paulo.
	now := time.Date(2026, 11, 15, 6, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	const day = 24 * time.Hour

	hot, quiet, old, stale, hiddenItem := r.releasedItem("game"), r.releasedItem("game"), r.releasedItem("game"),
		r.releasedItem("game"), r.releasedItem("game")

	for _, it := range []string{hot, quiet, old, stale, hiddenItem} {
		r.rate(ac, it, 8)
		review(ac, it, "text "+it, false).AssertStatus(http.StatusSeeOther)
	}

	r.rate(pc, hot, 8)
	review(pc, hot, "private author", false).AssertStatus(http.StatusSeeOther)

	rv := func(owner int64, item string) string {
		return r.str(`SELECT id::text FROM reviews WHERE user_id = $1 AND item_id = $2`, owner, item)
	}
	hotRv, quietRv, oldRv, staleRv, hiddenRv, privRv := rv(a.ID, hot), rv(a.ID, quiet), rv(a.ID, old),
		rv(a.ID, stale), rv(a.ID, hiddenItem), rv(p.ID, hot)

	set := func(q string, at time.Time, args ...any) {
		t.Helper()
		r.exec(q, append([]any{at}, args...)...)
	}

	set(`UPDATE reviews SET created_at = $1 WHERE id = $2`, ago(day), hotRv)
	set(`UPDATE reviews SET created_at = $1 WHERE id = $2`, ago(6*day), quietRv)
	set(`UPDATE reviews SET created_at = $1 WHERE id = $2`, ago(30*day), oldRv)
	set(`UPDATE reviews SET created_at = $1 WHERE id = $2`, ago(8*day), staleRv)
	set(`UPDATE reviews SET created_at = $1, hidden = true WHERE id = $2`, ago(day), hiddenRv)
	set(`UPDATE reviews SET created_at = $1 WHERE id = $2`, ago(day), privRv)

	// Activity: a like and an outside comment on the hot review, a like on the 30-day-old one, a self comment that
	// must not count, activity on a hidden review and on a private author's review.
	react(bc, "review", hotRv, "like").AssertStatus(http.StatusSeeOther)
	comment(cc, "review", hotRv, "great", "").AssertStatus(http.StatusSeeOther)
	comment(ac, "review", hotRv, "thanks", "").AssertStatus(http.StatusSeeOther)
	react(cc, "review", oldRv, "helpful").AssertStatus(http.StatusSeeOther)
	react(cc, "review", staleRv, "like").AssertStatus(http.StatusSeeOther)

	set(`UPDATE reactions SET created_at = $1 WHERE review_id = $2`, ago(day), hotRv)
	set(`UPDATE comments SET created_at = $1 WHERE review_id = $2`, ago(day), hotRv)
	set(`UPDATE reactions SET created_at = $1 WHERE review_id = $2`, ago(12*time.Hour), oldRv)
	set(`UPDATE reactions SET created_at = $1 WHERE review_id = $2`, ago(9*day), staleRv)

	if err := social.ComputeTrending(t.Context(), r.acc.DB, now); err != nil {
		t.Fatal(err)
	}
	// Running it again for the same week leaves the same rows.
	if err := social.ComputeTrending(t.Context(), r.acc.DB, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	if n := r.count(`SELECT count(*) FROM social_trending WHERE week = '2026-11-15'`); n != 6 {
		t.Fatalf("%d rows for the week, want 6 (3 reviews and 3 items)", n)
	}

	svc := &social.Follows{DB: r.acc.DB}
	got, err := svc.Trending(t.Context(), "en")
	if err != nil {
		t.Fatal(err)
	}

	if got.Week != "2026-11-15" || len(got.Reviews) != 3 || len(got.Items) != 3 {
		t.Fatalf("trending = week %q, %d reviews, %d items; want 2026-11-15, 3 and 3", got.Week, len(got.Reviews),
			len(got.Items))
	}

	hotScore := decay(social.WeightReview, day) + decay(social.WeightReaction, day) + decay(social.WeightComment, day)
	wantReviews := []struct {
		id    string
		score float64
	}{
		{hotRv, hotScore},
		{oldRv, decay(social.WeightReaction, 12*time.Hour)},
		{quietRv, decay(social.WeightReview, 6*day)},
	}

	for i, w := range wantReviews {
		g := got.Reviews[i]
		if g.Rank != i+1 || g.Review.ID != w.id {
			t.Fatalf("review rank %d = %s (rank %d), want %s", i+1, g.Review.ID, g.Rank, w.id)
		}

		near(t, "review score", g.Score, w.score)
	}

	if g := got.Reviews[0]; g.Likes != 1 || g.Comments != 1 {
		t.Fatalf("hot review counts = %d likes, %d comments, want 1 and 1 (the author's own comment is ignored)",
			g.Likes, g.Comments)
	}

	for i, w := range []struct{ item string }{{hot}, {old}, {quiet}} {
		if got.Items[i].Item.ID != w.item {
			t.Fatalf("item rank %d = %s, want %s", i+1, got.Items[i].Item.ID, w.item)
		}
	}

	near(t, "item score", got.Items[0].Score, hotScore)

	// Read-time filters: a review hidden after the job ran and an item taken down leave the page.
	r.exec(`UPDATE reviews SET hidden = true WHERE id = $1`, hotRv)
	r.exec(`UPDATE items SET visibility = 'hidden' WHERE id = $1`, quiet)

	got, err = svc.Trending(t.Context(), "en")
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Reviews) != 1 || got.Reviews[0].Review.ID != oldRv || len(got.Items) != 2 {
		t.Fatalf("after hiding: %d reviews, %d items", len(got.Reviews), len(got.Items))
	}

	// A quiet next week writes nothing and keeps the old rows; the prune drops weeks older than 12.
	if err := social.ComputeTrending(t.Context(), r.acc.DB, now.Add(7*day)); err != nil {
		t.Fatal(err)
	}

	if n := r.count(`SELECT count(DISTINCT week) FROM social_trending`); n != 1 {
		t.Fatalf("%d weeks stored, want 1", n)
	}

	if _, err := social.PruneTrending(t.Context(), r.acc.DB, now.Add(7*social.TrendingKeepWeeks*day+14*day)); err != nil {
		t.Fatal(err)
	}

	if n := r.count(`SELECT count(*) FROM social_trending`); n != 0 {
		t.Fatalf("%d rows after the prune, want 0", n)
	}

}

func TestSocialTrending_Empty(t *testing.T) {
	r := newSocialRig(t, true)

	got, err := (&social.Follows{DB: r.acc.DB}).Trending(t.Context(), "en")
	if err != nil {
		t.Fatal(err)
	}

	if got.Week != "" || len(got.Items) != 0 || len(got.Reviews) != 0 {
		t.Fatalf("trending before the first run = %+v", got)
	}
}

func TestSocialTrending_WeekOf(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"2026-11-15T06:00:00Z": "2026-11-15", // Sunday 03:00 in Sao Paulo
		"2026-11-15T02:00:00Z": "2026-11-08", // still Saturday 23:00 in Sao Paulo
		"2026-11-18T12:00:00Z": "2026-11-15",
		"2026-11-21T23:59:00Z": "2026-11-15",
	} {
		at, err := time.Parse(time.RFC3339, in)
		if err != nil {
			t.Fatal(err)
		}

		if got := social.WeekOf(at); got != want {
			t.Errorf("WeekOf(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestSocialTrending_Schedule(t *testing.T) {
	r := newSocialRig(t, true)

	sched, err := r.app.Schedule()
	if err != nil {
		t.Fatal(err)
	}

	want := "job:" + jobs.ComputeSocialTrending{}.Kind()
	for _, task := range sched.Tasks() {
		if task.Name() == want {
			if task.Expression() != "0 3 * * 0" {
				t.Fatalf("%s runs at %q, want Sunday 03:00 (0 3 * * 0)", want, task.Expression())
			}

			return
		}
	}

	t.Fatalf("schedule missing %s", want)
}
