// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"fmt"
	"strconv"
	"strings"
)

type randomRange struct{ min, max int }

type deviceRange struct {
	target string
	randomRange
}

type deviceRangeFlag struct {
	values   *[]deviceRange
	duration bool
}

func (f deviceRangeFlag) String() string { return "" }

func (f deviceRangeFlag) Set(raw string) error {
	target, value, ok := strings.Cut(raw, "=")
	target = strings.TrimSpace(target)
	if !ok || target == "" {
		return fmt.Errorf("want SHOCKER=MIN..MAX or SHOCKER=VALUE")
	}
	parts := strings.Split(value, "..")
	if len(parts) > 2 {
		return fmt.Errorf("invalid range %q", value)
	}
	parse := func(s string) (int, error) { return strconv.Atoi(strings.TrimSpace(s)) }
	lo, hi := minIntensity, maxIntensity
	if f.duration {
		parse = parseMillis
		lo, hi = minDurationMs, maxDurationMs
	}
	min, err := parse(parts[0])
	if err != nil {
		return err
	}
	max := min
	if len(parts) == 2 {
		max, err = parse(parts[1])
		if err != nil {
			return err
		}
	}
	if min < lo || max > hi || max < min {
		return fmt.Errorf("invalid range %q (allowed %d..%d)", value, lo, hi)
	}
	*f.values = append(*f.values, deviceRange{target: target, randomRange: randomRange{min, max}})
	return nil
}

type deviceRandomRanges struct {
	intensity *randomRange
	duration  *randomRange
}

// Overrides resolve only against selected shockers; they never add targets.
func resolveDeviceRanges(targets []Shocker, intensities, durations []deviceRange) (map[string]deviceRandomRanges, error) {
	out := make(map[string]deviceRandomRanges)
	for kind, values := range [][]deviceRange{intensities, durations} {
		for _, value := range values {
			resolved, err := resolveShockers([]string{value.target}, targets)
			if err != nil {
				return nil, usagef("device range: %v", err)
			}
			id := strings.ToLower(resolved[0].ID)
			selected := false
			for _, target := range targets {
				if strings.EqualFold(target.ID, id) {
					selected = true
					break
				}
			}
			if !selected {
				return nil, usagef("device range: %q is not a selected shocker", value.target)
			}
			ranges := out[id]
			field := &ranges.intensity
			flag := "--device-intensity"
			if kind == 1 {
				field = &ranges.duration
				flag = "--device-duration"
			}
			if *field != nil {
				return nil, usagef("%s specified more than once for %q", flag, value.target)
			}
			r := value.randomRange
			*field = &r
			out[id] = ranges
		}
	}
	return out, nil
}

func (o randomOptions) forTarget(target Shocker) randomOptions {
	ranges := o.deviceRanges[strings.ToLower(target.ID)]
	if ranges.intensity != nil {
		o.intensityMin, o.intensityMax = ranges.intensity.min, ranges.intensity.max
	}
	if ranges.duration != nil {
		o.durationMin, o.durationMax = ranges.duration.min, ranges.duration.max
	}
	o.deviceRanges = nil
	return o
}
