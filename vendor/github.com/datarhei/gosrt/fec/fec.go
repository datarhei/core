package fec

import (
	"fmt"
	"strconv"
	"strings"
)

// Config represents the parsed FEC configuration.
type Config struct {
	Cols   int
	Rows   int
	Layout string // "even" or "staircase"
	ARQ    string // "always", "onreq", "never"
}

// ParseConfig parses a packet filter string into an FEC config.
// Example: "fec,cols:10,rows:5,layout:even"
func ParseConfig(filterStr string) (Config, error) {
	cfg := Config{
		Cols:   0,
		Rows:   1, // default
		Layout: "even",
		ARQ:    "always",
	}

	parts := strings.Split(filterStr, ",")
	if len(parts) == 0 || strings.ToLower(parts[0]) != "fec" {
		return cfg, fmt.Errorf("not an fec config")
	}

	for _, part := range parts[1:] {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(kv[0])
		val := kv[1]

		switch key {
		case "cols":
			cols, err := strconv.Atoi(val)
			if err != nil || cols < 2 || cols > 65535 {
				return cfg, fmt.Errorf("invalid cols: %s", val)
			}
			cfg.Cols = cols
		case "rows":
			rows, err := strconv.Atoi(val)
			if err != nil || rows < 1 || rows > 65535 {
				return cfg, fmt.Errorf("invalid rows: %s", val)
			}
			cfg.Rows = rows
		case "layout":
			if val != "even" && val != "staircase" {
				return cfg, fmt.Errorf("invalid layout: %s", val)
			}
			cfg.Layout = val
		case "arq":
			if val != "always" && val != "onreq" && val != "never" {
				return cfg, fmt.Errorf("invalid arq: %s", val)
			}
			cfg.ARQ = val
		}
	}

	if cfg.Cols < 2 {
		return cfg, fmt.Errorf("cols must be specified and >= 2")
	}

	return cfg, nil
}
