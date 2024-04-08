package cluster

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

type ClusterVersion struct {
	Major uint64
	Minor uint64
	Patch uint64
}

func (v ClusterVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v ClusterVersion) Equal(x ClusterVersion) bool {
	return v.String() == x.String()
}

func ParseClusterVersion(version string) (ClusterVersion, error) {
	v := ClusterVersion{}

	sv, err := semver.NewVersion(version)
	if err != nil {
		return v, err
	}

	v.Major = sv.Major()
	v.Minor = sv.Minor()
	v.Patch = sv.Patch()

	return v, nil
}

// Version of the cluster
var Version = ClusterVersion{
	Major: 2,
	Minor: 0,
	Patch: 0,
}
