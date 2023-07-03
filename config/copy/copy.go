package copy

import (
	"github.com/datarhei/core/v16/config/value"
	"github.com/datarhei/core/v16/slices"
)

func StringMap(src map[string]string) map[string]string {
	dst := make(map[string]string)

	for k, v := range src {
		dst[k] = v
	}

	return dst
}

func TenantSlice(src []value.Auth0Tenant) []value.Auth0Tenant {
	dst := slices.Copy(src)

	for i, t := range src {
		dst[i].Users = slices.Copy(t.Users)
	}

	return dst
}
