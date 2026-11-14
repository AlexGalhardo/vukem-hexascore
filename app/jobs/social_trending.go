package jobs

import (
	"context"
	"time"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/queue"

	"github.com/AlexGalhardo/hexascore-app/app/social"
)

// ComputeSocialTrending ranks the most active reviews and items of the last 7 days into social_trending (SDD 14
// §12.13). Scheduled weekly, Sunday 03:00 America/Sao_Paulo, in routes/schedule.go. It does nothing while the
// `social` flag is off or on a non-Postgres database. Rerunning it rewrites the same week, so a retry is safe; the
// run is one set of aggregate queries over seven days of activity, so 10 minutes is ample.
type ComputeSocialTrending struct{}

// Kind names the job on the queue.
func (ComputeSocialTrending) Kind() string { return "social.compute_trending" }

func handleComputeSocialTrending(app *vukem.App) queue.Registration {
	return queue.Handle(func(ctx context.Context, _ ComputeSocialTrending) error {
		db, err := app.DB()
		if err != nil {
			return err
		}

		if db.Dialect().Name() != "postgres" {
			return nil
		}

		ff, err := app.Flags()
		if err != nil {
			return err
		}

		if !(&social.Follows{DB: db, Flags: ff}).Enabled(ctx) {
			return nil
		}

		if err := social.ComputeTrending(ctx, db, app.Now()); err != nil {
			return err
		}

		app.Log.InfoContext(ctx, "social trending computed", "week", social.WeekOf(app.Now()))

		return nil
	}, queue.Timeout(10*time.Minute), queue.Retries(2))
}
