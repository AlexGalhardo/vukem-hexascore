//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem/vukemtest"

	"github.com/AlexGalhardo/hexascore-app/app/models"
)

func comment(c *vukemtest.Client, target, id, body, parentID string) *vukemtest.Response {
	in := map[string]any{"body": body}
	if parentID != "" {
		in["parentId"] = parentID
	}

	return c.PostJSON("/comments/"+target+"/"+id, in)
}

func react(c *vukemtest.Client, target, id, kind string) *vukemtest.Response {
	return c.PutJSON("/reactions/"+target+"/"+id, map[string]any{"kind": kind})
}

func (r *listRig) str(q string, args ...any) string {
	r.t.Helper()

	db, err := r.app.DB()
	if err != nil {
		r.t.Fatal(err)
	}

	var s string
	if err := db.QueryRowContext(r.t.Context(), q, args...).Scan(&s); err != nil {
		r.t.Fatal(err)
	}

	return s
}

// reviewProp returns the item page's review card written by author, as raw JSON maps.
func (r *listRig) reviewProp(viewer *models.User, itemID, author string) map[string]any {
	r.t.Helper()

	c := r.app.Client(r.t)
	if viewer != nil {
		c = browserFor(r.t, r.app, *viewer)
	}

	page := c.WithHeader("X-Inertia-Partial-Component", "Catalog/Show").WithHeader("X-Inertia-Partial-Data", "reviews").
		Inertia("").Get(r.itemPath(itemID)).AssertOK().AssertInertia("Catalog/Show")

	raw, _ := page.Prop("reviews")
	for _, e := range raw.(map[string]any)["data"].([]any) {
		m := e.(map[string]any)
		if m["author"].(map[string]any)["nickname"] == author {
			return m
		}
	}

	r.t.Fatalf("no review by %s", author)

	return nil
}

// socialSetup gives Critic a review of a game and a public list, Second and the private Hidden as readers.
func socialSetup(
	t *testing.T,
) (r *listRig, a, b, p models.User, ac, bc, pc *vukemtest.Client, item, reviewID, list string) {
	t.Helper()

	return socialSetupEnv(t, nil)
}

func socialSetupEnv(
	t *testing.T,
	extra map[string]string,
) (r *listRig, a, b, p models.User, ac, bc, pc *vukemtest.Client, item, reviewID, list string) {
	t.Helper()

	r = newSocialRigEnv(t, true, extra)
	a, ac = r.member("ca@example.com", "Critic", false)
	b, bc = r.member("cb@example.com", "Second", false)
	p, pc = r.member("cp@example.com", "Hidden", true)

	item = r.releasedItem("game")
	r.rate(ac, item, 8)
	review(ac, item, "Great game", false).AssertStatus(http.StatusSeeOther)
	reviewID = r.str(`SELECT id::text FROM reviews WHERE user_id = $1`, a.ID)
	list = r.publicList(a, item, true)

	return r, a, b, p, ac, bc, pc, item, reviewID, list
}

func TestComments(t *testing.T) {
	r, a, b, _, ac, bc, pc, item, rv, list := socialSetup(t)

	comment(bc, "review", rv, "   ", "").AssertUnprocessable()
	comment(bc, "review", rv, strings.Repeat("x", 2001), "").AssertUnprocessable()
	comment(bc, "review", rv, "Agreed", "").AssertStatus(http.StatusSeeOther)
	top := r.str(`SELECT id::text FROM comments WHERE user_id = $1`, b.ID)

	comment(ac, "review", rv, "Thanks", top).AssertStatus(http.StatusSeeOther)
	reply := r.str(`SELECT id::text FROM comments WHERE user_id = $1 AND parent_id IS NOT NULL`, a.ID)
	comment(bc, "review", rv, "Too deep", reply).AssertUnprocessable().AssertSee("reply-depth")
	comment(bc, "list", list, "Wrong target", top).AssertNotFound()
	comment(pc, "review", rv, "From a private profile", "").AssertStatus(http.StatusSeeOther)

	comment(bc, "list", list, "Nice list", "").AssertStatus(http.StatusSeeOther)
	privateList := r.publicList(a, item, false)
	comment(bc, "list", privateList, "Hidden list", "").AssertNotFound()
	comment(bc, "item", item, "Bad target", "").AssertNotFound()
	comment(r.app.Client(t), "review", rv, "Anonymous", "").AssertUnauthorized()

	// Critic hears about both comments on his review and list; Second hears about the reply.
	if n := r.notices(a, "new_comment"); n != 3 {
		t.Fatalf("Critic has %d new_comment notices, want 3 (two on the review, one on the list)", n)
	}
	if n := r.notices(b, "new_comment"); n != 1 {
		t.Fatalf("Second has %d new_comment notices, want 1 (the reply)", n)
	}

	if own := r.reviewProp(&a, item, "Critic")["comments"].([]any)[0].(map[string]any); own["removable"] != true {
		t.Fatalf("the review owner's view of Second's comment = %v, want removable", own)
	}

	card := r.reviewProp(nil, item, "Critic")
	comments := card["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("comments = %v, want Second's only (the private author is left out)", comments)
	}
	first := comments[0].(map[string]any)
	if first["removable"] != nil {
		t.Fatalf("an anonymous reader sees the comment as removable: %v", first)
	}
	replies, _ := first["replies"].([]any)
	if first["body"] != "Agreed" || len(replies) != 1 || replies[0].(map[string]any)["body"] != "Thanks" {
		t.Fatalf("thread = %v", first)
	}

	page := r.app.Client(t).WithHeader("X-Inertia-Partial-Component", "User/List").
		WithHeader("X-Inertia-Partial-Data", "comments").Inertia("").Get("/u/critic/lists/" + list).AssertOK().
		AssertInertia("User/List")
	if v, _ := page.Prop("comments"); v == nil || len(v.([]any)) != 1 {
		t.Fatalf("list comments = %v", v)
	}

	pc.Delete("/comments/" + top).AssertNotFound()
	bc.Delete("/comments/" + top).AssertStatus(http.StatusSeeOther)
	if n := r.count(`SELECT count(*) FROM comments WHERE review_id = $1`, rv); n != 1 {
		t.Fatalf("%d review comments after deleting the thread, want 1 (the reply went with it)", n)
	}
}

func TestComments_RateLimit(t *testing.T) {
	r, _, _, _, _, bc, _, _, rv, _ := socialSetup(t)

	for range 60 {
		comment(bc, "review", rv, "c", "").AssertStatus(http.StatusSeeOther)
	}
	comment(bc, "review", rv, "one too many", "").AssertStatus(http.StatusTooManyRequests)

	if n := r.count(`SELECT count(*) FROM comments`); n != 60 {
		t.Fatalf("%d comments, want 60", n)
	}
}

func TestReactions(t *testing.T) {
	r, a, b, _, ac, bc, _, item, rv, list := socialSetup(t)

	react(ac, "review", rv, "helpful").AssertUnprocessable().AssertSee("cannot-react-to-self")
	react(bc, "review", rv, "love").AssertUnprocessable()
	react(bc, "item", item, "like").AssertNotFound()

	react(bc, "review", rv, "helpful").AssertStatus(http.StatusSeeOther)
	if n := r.count(`SELECT helpful_count FROM reviews WHERE id = $1`, rv); n != 1 {
		t.Fatalf("helpful_count = %d, want 1", n)
	}

	// One reaction per user and target: switching kinds moves the count.
	react(bc, "review", rv, "like").AssertStatus(http.StatusSeeOther)
	if h, l := r.count(`SELECT helpful_count FROM reviews WHERE id = $1`, rv),
		r.count(`SELECT like_count FROM reviews WHERE id = $1`, rv); h != 0 || l != 1 {
		t.Fatalf("helpful %d, like %d after switching, want 0 and 1", h, l)
	}
	if n := r.count(`SELECT count(*) FROM reactions WHERE user_id = $1`, b.ID); n != 1 {
		t.Fatalf("%d reaction rows, want 1", n)
	}

	card := r.reviewProp(&b, item, "Critic")
	if fmt.Sprint(card["likes"]) != "1" || card["myReaction"] != "like" {
		t.Fatalf("card = likes %v, mine %v", card["likes"], card["myReaction"])
	}

	bc.Delete("/reactions/review/" + rv).AssertStatus(http.StatusSeeOther)
	if n := r.count(`SELECT like_count FROM reviews WHERE id = $1`, rv); n != 0 {
		t.Fatalf("like_count = %d after removing, want 0", n)
	}

	react(bc, "list", list, "like").AssertStatus(http.StatusSeeOther)
	if n := r.count(`SELECT like_count FROM lists WHERE id = $1`, list); n != 1 {
		t.Fatalf("list like_count = %d", n)
	}

	comment(ac, "review", rv, "Author note", "").AssertStatus(http.StatusSeeOther)
	cid := r.str(`SELECT id::text FROM comments WHERE user_id = $1`, a.ID)
	react(bc, "comment", cid, "helpful").AssertStatus(http.StatusSeeOther)
	if n := r.count(`SELECT helpful_count FROM comments WHERE id = $1`, cid); n != 1 {
		t.Fatalf("comment helpful_count = %d", n)
	}

	// The notice is deduplicated per reactor and target, so toggling does not spam: review, list, comment.
	if n := r.notices(a, "new_reaction"); n != 3 {
		t.Fatalf("Critic has %d new_reaction notices, want 3", n)
	}

	comment(bc, "review", rv, "Nice", "").AssertStatus(http.StatusSeeOther)
	page := browserFor(t, r.app, a).Inertia("").Get("/me/notifications").AssertOK()
	if !strings.Contains(page.Body(), "new_reaction") || !strings.Contains(page.Body(), "new_comment") {
		t.Fatal("/me/notifications does not list the comment and reaction notices")
	}

	// Purging the reactor's account takes the counts back down.
	r.exec(`DELETE FROM users WHERE id = $1`, b.ID)
	if n := r.count(`SELECT like_count FROM lists WHERE id = $1`, list); n != 0 {
		t.Fatalf("list like_count = %d after the reactor went, want 0", n)
	}
}

func TestCommentsDisabledByDefault(t *testing.T) {
	r := newSocialRig(t, false)
	_, bc := r.member("cd@example.com", "Second", false)
	g := r.releasedItem("game")

	comment(bc, "list", g, "x", "").AssertNotFound()
	react(bc, "list", g, "like").AssertNotFound()
}

// A comment is removed by its author, by the owner of the review or list it sits on, or by a moderator or admin;
// everyone else gets 404. Removal by someone else tells the author.
func TestComments_DeleteByOwnerAndModerator(t *testing.T) {
	r, _, b, _, ac, bc, pc, _, rv, list := socialSetupEnv(
		t,
		map[string]string{"FLAG_COMMUNITY_TRUSTED_MODERATORS": "true"},
	)

	mod := makeUser(t, r.app, "cmod@example.com", models.RoleModerator, true)
	modc := browserFor(t, r.app, mod)

	post := func(target, id string) string {
		t.Helper()
		comment(bc, target, id, "note "+target, "").AssertStatus(http.StatusSeeOther)

		return r.str(`SELECT id::text FROM comments WHERE user_id = $1 AND `+target+`_id = $2`, b.ID, id)
	}

	onReview, onList := post("review", rv), post("list", list)

	// A bystander cannot delete either; nothing is removed and nobody is told.
	pc.Delete("/comments/" + onReview).AssertNotFound()
	pc.Delete("/comments/" + onList).AssertNotFound()

	if n := r.count(`SELECT count(*) FROM comments`); n != 2 {
		t.Fatalf("%d comments after the refused deletes, want 2", n)
	}

	ac.Delete("/comments/" + onReview).AssertStatus(http.StatusSeeOther)
	modc.Delete("/comments/" + onList).AssertStatus(http.StatusSeeOther)

	if n := r.count(`SELECT count(*) FROM comments`); n != 0 {
		t.Fatalf("%d comments left, want 0", n)
	}

	if n := r.notices(b, "content_deleted"); n != 2 {
		t.Fatalf("author has %d content_deleted notices, want 2", n)
	}

	// Deleting your own comment sends no notice.
	own := post("review", rv)
	bc.Delete("/comments/" + own).AssertStatus(http.StatusSeeOther)

	if n := r.notices(b, "content_deleted"); n != 2 {
		t.Fatalf("author has %d content_deleted notices after deleting his own, want 2", n)
	}

	// A plain member keeps getting 404 on a comment that is gone.
	modc.Delete("/comments/" + own).AssertNotFound()
}

// Security: the moderator override of DeleteComment follows the same rule as the panel (admin.CanModerate): a
// banned or unverified moderator, or any moderator while community_trusted_moderators is off, is a plain member.
func TestSecurity_CommentDeleteModeratorGate(t *testing.T) {
	banned := `UPDATE users SET banned_at = now() WHERE id = $1`
	unverified := `UPDATE users SET email_verified_at = NULL WHERE id = $1`

	for name, tc := range map[string]struct {
		env   map[string]string
		setup string
	}{
		"flag off":   {nil, ""},
		"banned":     {map[string]string{"FLAG_COMMUNITY_TRUSTED_MODERATORS": "true"}, banned},
		"unverified": {map[string]string{"FLAG_COMMUNITY_TRUSTED_MODERATORS": "true"}, unverified},
	} {
		t.Run(name, func(t *testing.T) {
			r, _, b, _, _, bc, _, _, rv, _ := socialSetupEnv(t, tc.env)
			comment(bc, "review", rv, "keep me", "").AssertStatus(http.StatusSeeOther)
			id := r.str(`SELECT id::text FROM comments WHERE user_id = $1`, b.ID)

			mod := makeUser(t, r.app, "gmod@example.com", models.RoleModerator, true)
			if tc.setup != "" {
				r.count(`WITH u AS (`+tc.setup+` RETURNING 1) SELECT count(*) FROM u`, mod.ID)
			}

			// 404 (flag off), 403 (banned) or a redirect to verification (unverified): whichever, the row must stay.
			browserFor(t, r.app, mod).Delete("/comments/" + id)

			if n := r.count(`SELECT count(*) FROM comments WHERE id = $1`, id); n != 1 {
				t.Fatalf("comment gone after a refused delete (%s)", name)
			}
		})
	}
}
