package api

import (
	"errors"
	"net/http"

	"github.com/datarhei/core/v16/http/api"
	apierrors "github.com/datarhei/core/v16/http/errors"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/zone"

	"github.com/labstack/echo/v4"
)

// The ZoneHandler type provides handler functions for managing zones.
type ZoneHandler struct {
	zones zone.Manager
}

// NewZone returns a new ZoneHandler type. You have to provide a zone manager.
func NewZone(zones zone.Manager) *ZoneHandler {
	return &ZoneHandler{
		zones: zones,
	}
}

// List returns all zones
// @Summary List all zones
// @Description List all zones with their limits and current usage.
// @Tags v16.?.?
// @ID zone-3-list
// @Produce json
// @Success 200 {array} api.ZoneInfo
// @Security ApiKeyAuth
// @Router /api/v3/zone [get]
func (h *ZoneHandler) List(c echo.Context) error {
	zones := []api.ZoneInfo{}

	for _, info := range h.zones.List() {
		z := api.ZoneInfo{}
		z.Marshal(info)
		zones = append(zones, z)
	}

	return c.JSON(http.StatusOK, zones)
}

// Add adds a zone
// @Summary Add a zone
// @Description Add a zone. Only superusers are allowed to modify zones.
// @Tags v16.?.?
// @ID zone-3-add
// @Accept json
// @Produce json
// @Param zone body api.Zone true "Zone"
// @Success 200 {object} api.Zone
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/zone [post]
func (h *ZoneHandler) Add(c echo.Context) error {
	if err := checkZoneSuperuser(c); err != nil {
		return err
	}

	z := api.Zone{}

	if err := util.ShouldBindJSON(c, &z); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if err := h.zones.Add(z.Unmarshal()); err != nil {
		return zoneError(err)
	}

	return c.JSON(http.StatusOK, z)
}

// Get returns a zone
// @Summary Get a zone
// @Description Get the zone with the given ID with its limits and current usage
// @Tags v16.?.?
// @ID zone-3-get
// @Produce json
// @Param id path string true "Zone ID, $default for the default zone"
// @Success 200 {object} api.ZoneInfo
// @Failure 404 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/zone/{id} [get]
func (h *ZoneHandler) Get(c echo.Context) error {
	id := util.PathParam(c, "id")

	info, err := h.zones.Get(id)
	if err != nil {
		return zoneError(err)
	}

	z := api.ZoneInfo{}
	z.Marshal(info)

	return c.JSON(http.StatusOK, z)
}

// Update replaces the limits of a zone
// @Summary Replace the limits of a zone
// @Description Replace the limits of the zone with the given ID. The limits apply to processes that are added or updated afterwards. Only superusers are allowed to modify zones.
// @Tags v16.?.?
// @ID zone-3-update
// @Accept json
// @Produce json
// @Param id path string true "Zone ID, $default for the default zone"
// @Param zone body api.Zone true "Zone"
// @Success 200 {object} api.Zone
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/zone/{id} [put]
func (h *ZoneHandler) Update(c echo.Context) error {
	if err := checkZoneSuperuser(c); err != nil {
		return err
	}

	id := util.PathParam(c, "id")

	z := api.Zone{}

	if err := util.ShouldBindJSON(c, &z); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if len(z.ID) == 0 {
		z.ID = id
	} else if z.ID != id {
		return apierrors.Err(http.StatusBadRequest, "", "the ID of the zone can't be changed")
	}

	if err := h.zones.Update(z.Unmarshal()); err != nil {
		return zoneError(err)
	}

	return c.JSON(http.StatusOK, z)
}

// Delete removes a zone
// @Summary Delete a zone
// @Description Delete the zone with the given ID. A zone with processes and the default zone can't be deleted. Only superusers are allowed to modify zones.
// @Tags v16.?.?
// @ID zone-3-delete
// @Produce json
// @Param id path string true "Zone ID"
// @Success 200 {string} string
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/zone/{id} [delete]
func (h *ZoneHandler) Delete(c echo.Context) error {
	if err := checkZoneSuperuser(c); err != nil {
		return err
	}

	id := util.PathParam(c, "id")

	if err := h.zones.Remove(id); err != nil {
		return zoneError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// canAccessZone returns whether the user is allowed to access the zone with the given action
// in the given domain. Everybody is allowed to access the default zone, an empty zone is the
// default zone.
func canAccessZone(iam iam.Enforcer, user, domain, id, action string) bool {
	if len(id) == 0 || id == zone.DefaultID {
		return true
	}

	return iam.Enforce(user, domain, "zone", id, action)
}

func checkZoneSuperuser(c echo.Context) error {
	if !util.DefaultContext(c, "superuser", false) {
		return apierrors.Err(http.StatusForbidden, "", "only superusers are allowed to modify zones")
	}

	return nil
}

func zoneError(err error) error {
	switch {
	case errors.Is(err, zone.ErrInvalid), errors.Is(err, zone.ErrReadOnly):
		return apierrors.Err(http.StatusBadRequest, "", "%s", err.Error())
	case errors.Is(err, zone.ErrExists), errors.Is(err, zone.ErrInUse):
		return apierrors.Err(http.StatusConflict, "", "%s", err.Error())
	case errors.Is(err, zone.ErrNotFound):
		return apierrors.Err(http.StatusNotFound, "", "%s", err.Error())
	}

	return apierrors.Err(http.StatusInternalServerError, "", "%s", err.Error())
}
