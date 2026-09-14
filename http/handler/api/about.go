package api

import (
	"net/http"
	"time"

	"github.com/datarhei/core/v16/app"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/restream"

	"github.com/labstack/echo/v4"
)

// The AboutHandler type provides handler functions for retrieving details
// about the API version and build infos.
type AboutHandler struct {
	restream     restream.Restreamer
	auths        []string
	totpRequired func() bool
}

// NewAbout returns a new About type
func NewAbout(restream restream.Restreamer, auths []string, totpRequired func() bool) *AboutHandler {
	return &AboutHandler{
		restream:     restream,
		auths:        auths,
		totpRequired: totpRequired,
	}
}

// About returns API version and build infos
// @Summary API version and build infos
// @Description API version and build infos in case auth is valid or not required. If auth is required, just the name field is populated.
// @ID about
// @Produce json
// @Success 200 {object} api.About
// @Security ApiKeyAuth
// @Router /api [get]
func (p *AboutHandler) About(c echo.Context) error {
	createdAt := p.restream.CreatedAt()

	totpRequired := false
	if p.totpRequired != nil {
		totpRequired = p.totpRequired()
	}

	about := api.About{
		App:          app.Name,
		Name:         p.restream.Name(),
		Auths:        p.auths,
		TOTPRequired: totpRequired,
		ID:           p.restream.ID(),
		CreatedAt: createdAt.Format(time.RFC3339),
		Uptime:    uint64(time.Since(createdAt).Seconds()),
		Version: api.Version{
			Number:   app.Version.String(),
			Commit:   app.Commit,
			Branch:   app.Branch,
			Build:    app.Build,
			Arch:     app.Arch,
			Compiler: app.Compiler,
		},
	}

	return c.JSON(http.StatusOK, about)
}
