package copy

import (
	"maps"
	"slices"

	"github.com/datarhei/core/v16/config/value"
)

func StringMap(src map[string]string) map[string]string {
	return maps.Clone(src)
}

func TenantSlice(src []value.Auth0Tenant) []value.Auth0Tenant {
	dst := slices.Clone(src)

	for i, t := range src {
		dst[i].Users = slices.Clone(t.Users)
	}

	return dst
}
