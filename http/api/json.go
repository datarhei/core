package api

import (
	"fmt"

	"github.com/datarhei/core/v16/encoding/json"
)

func ToNumber(f float64) json.Number {
	var s string

	if f == float64(int64(f)) {
		s = fmt.Sprintf("%.0f", f) // 0 decimal if integer
	} else {
		s = fmt.Sprintf("%.3f", f) // max. 3 decimal if float
	}

	return json.Number(s)
}
