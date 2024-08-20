package api

import (
	"net/http"
	"time"

	"github.com/datarhei/core/v16/app"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/resources"
	"github.com/datarhei/core/v16/restream"

	"github.com/labstack/echo/v4"
)

// The AboutHandler type provides handler functions for retrieving details
// about the API version and build infos.
type AboutHandler struct {
	restream  restream.Restreamer
	resources resources.Resources
	auths     func() []string
}

// NewAbout returns a new About type
func NewAbout(restream restream.Restreamer, resources resources.Resources, auths func() []string) *AboutHandler {
	return &AboutHandler{
		restream:  restream,
		resources: resources,
		auths:     auths,
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
	user, _ := c.Get("user").(string)

	if user == "$anon" {
		return c.JSON(http.StatusOK, api.MinimalAbout{
			App:   app.Name,
			Auths: p.auths(),
			Version: api.AboutVersionMinimal{
				Number: app.Version.MajorString(),
			},
		})
	}

	createdAt := p.restream.CreatedAt()

	about := api.About{
		App:       app.Name,
		Name:      p.restream.Name(),
		Auths:     p.auths(),
		ID:        p.restream.ID(),
		CreatedAt: createdAt.Format(time.RFC3339),
		Uptime:    uint64(time.Since(createdAt).Seconds()),
		Version: api.AboutVersion{
			Number:   app.Version.String(),
			Commit:   app.Commit,
			Branch:   app.Branch,
			Build:    app.Build,
			Arch:     app.Arch,
			Compiler: app.Compiler,
		},
	}

	if p.resources != nil {
		res := p.resources.Info()
		about.Resources.IsThrottling = res.CPU.Throttling
		about.Resources.NCPU = res.CPU.NCPU
		about.Resources.CPU = (100 - res.CPU.Idle) * res.CPU.NCPU
		about.Resources.CPULimit = res.CPU.Limit * res.CPU.NCPU
		about.Resources.CPUCore = res.CPU.Core * res.CPU.NCPU
		about.Resources.Mem = res.Mem.Total - res.Mem.Available
		about.Resources.MemLimit = res.Mem.Limit
		about.Resources.MemTotal = res.Mem.Total
		about.Resources.MemCore = res.Mem.Core
	}

	return c.JSON(http.StatusOK, about)
}
