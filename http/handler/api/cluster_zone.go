package api

import (
	"errors"
	"net/http"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/http/api"
	apierrors "github.com/datarhei/core/v16/http/errors"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/zone"

	"github.com/labstack/echo/v4"
)

// ZoneList returns all zones from the cluster DB
// @Summary List all zones in the cluster
// @Description List all zones in the cluster with their limits and current usage.
// @Tags v16.?.?
// @ID cluster-3-zone-list
// @Produce json
// @Success 200 {array} api.ZoneInfo
// @Security ApiKeyAuth
// @Router /api/v3/cluster/zone [get]
func (h *ClusterHandler) ZoneList(c echo.Context) error {
	zones := []api.ZoneInfo{}

	for _, info := range h.cluster.Store().ZoneList() {
		z := api.ZoneInfo{}
		z.Marshal(info)
		zones = append(zones, z)
	}

	return c.JSON(http.StatusOK, zones)
}

// ZoneAdd adds a zone to the cluster
// @Summary Add a zone to the cluster
// @Description Add a zone to the cluster. Only superusers are allowed to modify zones.
// @Tags v16.?.?
// @ID cluster-3-zone-add
// @Accept json
// @Produce json
// @Param zone body api.Zone true "Zone"
// @Success 200 {object} api.Zone
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/zone [post]
func (h *ClusterHandler) ZoneAdd(c echo.Context) error {
	if err := checkZoneSuperuser(c); err != nil {
		return err
	}

	z := api.Zone{}

	if err := util.ShouldBindJSON(c, &z); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if err := h.cluster.ZoneAdd("", z.Unmarshal()); err != nil {
		return clusterZoneError(err)
	}

	return c.JSON(http.StatusOK, z)
}

// ZoneGet returns a zone from the cluster DB
// @Summary Get a zone from the cluster
// @Description Get the zone with the given ID from the cluster with its limits and current usage
// @Tags v16.?.?
// @ID cluster-3-zone-get
// @Produce json
// @Param id path string true "Zone ID, $default for the default zone"
// @Success 200 {object} api.ZoneInfo
// @Failure 404 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/zone/{id} [get]
func (h *ClusterHandler) ZoneGet(c echo.Context) error {
	id := util.PathParam(c, "id")

	info, err := h.cluster.Store().ZoneGet(id)
	if err != nil {
		return clusterZoneError(err)
	}

	z := api.ZoneInfo{}
	z.Marshal(info)

	return c.JSON(http.StatusOK, z)
}

// ZoneUpdate replaces the limits of a zone in the cluster
// @Summary Replace the limits of a zone in the cluster
// @Description Replace the limits of the zone with the given ID in the cluster. The limits apply to processes that are added or updated afterwards. Only superusers are allowed to modify zones.
// @Tags v16.?.?
// @ID cluster-3-zone-update
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
// @Router /api/v3/cluster/zone/{id} [put]
func (h *ClusterHandler) ZoneUpdate(c echo.Context) error {
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

	if err := h.cluster.ZoneUpdate("", z.Unmarshal()); err != nil {
		return clusterZoneError(err)
	}

	return c.JSON(http.StatusOK, z)
}

// ZoneRemove removes a zone from the cluster
// @Summary Delete a zone from the cluster
// @Description Delete the zone with the given ID from the cluster. A zone with processes and the default zone can't be deleted. Only superusers are allowed to modify zones.
// @Tags v16.?.?
// @ID cluster-3-zone-delete
// @Produce json
// @Param id path string true "Zone ID"
// @Success 200 {string} string
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/zone/{id} [delete]
func (h *ClusterHandler) ZoneRemove(c echo.Context) error {
	if err := checkZoneSuperuser(c); err != nil {
		return err
	}

	id := util.PathParam(c, "id")

	if err := h.cluster.ZoneRemove("", id); err != nil {
		return clusterZoneError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// clusterZoneError maps an error from the cluster to a status code. Errors that
// have been forwarded from the leader only carry the store errors.
func clusterZoneError(err error) error {
	switch {
	case errors.Is(err, store.ErrBadRequest), errors.Is(err, zone.ErrInvalid):
		return apierrors.Err(http.StatusBadRequest, "", "%s", err.Error())
	case errors.Is(err, store.ErrConflict), errors.Is(err, zone.ErrExists), errors.Is(err, zone.ErrInUse):
		return apierrors.Err(http.StatusConflict, "", "%s", err.Error())
	case errors.Is(err, store.ErrNotFound), errors.Is(err, zone.ErrNotFound):
		return apierrors.Err(http.StatusNotFound, "", "%s", err.Error())
	}

	return apierrors.Err(http.StatusInternalServerError, "", "%s", err.Error())
}
