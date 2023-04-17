package cluster

import "github.com/labstack/echo/v4"

// Forwarder forwards any HTTP request from a follower to the leader
type Forwarder interface {
	SetLeader(address string)
	Forward(c echo.Context)
}

type forwarder struct {
	leaderAddr string
}

func NewForwarder() (Forwarder, error) {
	return &forwarder{}, nil
}

func (f *forwarder) SetLeader(address string) {
	if f.leaderAddr == address {
		return
	}

}

func (f *forwarder) Forward(c echo.Context) {

}
