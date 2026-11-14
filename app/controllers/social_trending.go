package controllers

import (
	"github.com/AlexGalhardo/vukem/i18n"
	"github.com/AlexGalhardo/vukem/inertia"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/hexascore-app/app/social"
)

// SocialTrending serves /trending: the weekly ranking of the most active reviews and items (SDD 14 §12.13). It is
// public, read from the table the Sunday job writes; 404 while the flag `social` is off.
type SocialTrending struct {
	svc *social.Follows
}

// NewSocialTrending builds the handler.
func NewSocialTrending(svc *social.Follows) *SocialTrending {
	return &SocialTrending{svc: svc}
}

// Index renders the latest computed week.
func (h *SocialTrending) Index(c *web.Ctx) error {
	if !h.svc.Enabled(c.Context()) {
		return NotFound(c)
	}

	t, err := h.svc.Trending(c.Context(), i18n.Locale(c.Context()))
	if err != nil {
		return err
	}

	return inertia.Render(c, "Trending/Index", inertia.Props{"trending": t})
}
