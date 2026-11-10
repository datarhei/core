// Copyright (c) The M1CPU Authors
// SPDX-License-Identifier: MPL-2.0

package m1cpu

func eCoreVoltageState(model string) int {
	switch model {
	case "Apple M5 Pro", "Apple M5 Max":
		return 22
	default:
		return 1
	}
}

func toHz(hz uint64, model string) uint64 {
	// Starting with M4, Apple reports clock speed in kHz.
	// See https://github.com/exelban/stats/commit/3e056562b360c937b883725f14f3427d5401b6fe
	if generation(model) >= 4 {
		return hz * 1000
	}
	return hz
}

func toGhz(hz uint64, model string) float64 {
	return float64(toHz(hz, model)) / 1_000_000_000
}
