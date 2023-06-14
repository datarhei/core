package api

import (
	"net/http"
	"strings"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/session"

	"github.com/labstack/echo/v4"
)

// The SessionHandler type provides handlers to retrieve session information
type SessionHandler struct {
	registry session.RegistryReader
	iam      iam.IAM
}

// NewSession returns a new Session type. You have to provide a session registry.
func NewSession(registry session.RegistryReader, iam iam.IAM) *SessionHandler {
	return &SessionHandler{
		registry: registry,
		iam:      iam,
	}
}

// Summary returns a summary of all active and past sessions
// @Summary Get a summary of all active and past sessions
// @Description Get a summary of all active and past sessions of the given collector.
// @Tags v16.7.2
// @ID session-3-summary
// @Produce json
// @Security ApiKeyAuth
// @Param collectors query string false "Comma separated list of collectors"
// @Success 200 {object} api.SessionsSummary "Sessions summary"
// @Router /api/v3/session [get]
func (s *SessionHandler) Summary(c echo.Context) error {
	collectors := strings.Split(util.DefaultQuery(c, "collectors", ""), ",")

	sessionsSummary := make(api.SessionsSummary)

	for _, name := range collectors {
		summary := api.SessionSummary{}
		summary.Unmarshal(s.registry.Summary(name))

		sessionsSummary[name] = summary
	}

	return c.JSON(http.StatusOK, sessionsSummary)
}

// Active returns a list of active sessions
// @Summary Get a minimal summary of all active sessions
// @Description Get a minimal summary of all active sessions (i.e. number of sessions, bandwidth).
// @Tags v16.7.2
// @ID session-3-current
// @Produce json
// @Security ApiKeyAuth
// @Param collectors query string false "Comma separated list of collectors"
// @Success 200 {object} api.SessionsActive "Active sessions listing"
// @Router /api/v3/session/active [get]
func (s *SessionHandler) Active(c echo.Context) error {
	collectors := strings.Split(util.DefaultQuery(c, "collectors", ""), ",")

	sessionsActive := make(api.SessionsActive)

	for _, name := range collectors {
		sessions := s.registry.Active(name)

		active := make([]api.Session, len(sessions))

		for i, s := range sessions {
			active[i].Unmarshal(s)
		}

		sessionsActive[name] = active
	}

	return c.JSON(http.StatusOK, sessionsActive)
}

// Request access tokens
// @Summary Request access tokens
// @Description Request access tokens
// @Tags v16.?.?
// @ID session-3-create-token
// @Accept json
// @Produce json
// @Param username path string true "Username"
// @Param config body []api.SessionTokenRequest true "Token request"
// @Success 200 {array} api.SessionTokenRequest
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/session/token/{username} [put]
func (s *SessionHandler) CreateToken(c echo.Context) error {
	username := util.PathParam(c, "username")

	identity, err := s.iam.GetVerifier(username)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "%s", err)
	}

	request := []api.SessionTokenRequest{}

	if err := util.ShouldBindJSONValidation(c, &request, false); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	for _, r := range request {
		err := c.Validate(r)
		if err != nil {
			return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}
	}

	for i, req := range request {
		data := map[string]interface{}{
			"match":  req.Match,
			"remote": req.Remote,
			"extra":  req.Extra,
		}

		request[i].Token = identity.GetServiceSession(data)
	}

	return c.JSON(http.StatusOK, request)
}
