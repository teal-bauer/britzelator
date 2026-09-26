// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parseMillis parses a duration. Bare numbers are milliseconds; "ms", "s", and
// "m" suffixes are accepted so "500", "500ms", "1.5s", and "1m" all work.
func parseMillis(raw string) (int, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, fmt.Errorf("invalid duration %q", raw)
	}
	multiplier := 1.0
	switch {
	case strings.HasSuffix(s, "ms"):
		s = strings.TrimSuffix(s, "ms")
	case strings.HasSuffix(s, "s"):
		s = strings.TrimSuffix(s, "s")
		multiplier = 1000
	case strings.HasSuffix(s, "m"):
		s = strings.TrimSuffix(s, "m")
		multiplier = 60000
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (want e.g. 500, 500ms, 1.5s, 1m)", raw)
	}
	return int(math.Round(value * multiplier)), nil
}

// parseSeconds parses a wait. Bare numbers are seconds; suffixed values are
// converted, so "30", "30s", "500ms", and "2m" all work.
func parseSeconds(raw string) (float64, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, fmt.Errorf("invalid delay %q", raw)
	}
	switch {
	case strings.HasSuffix(s, "ms"), strings.HasSuffix(s, "s"), strings.HasSuffix(s, "m"):
		ms, err := parseMillis(s)
		if err != nil {
			return 0, err
		}
		return float64(ms) / 1000, nil
	}
	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid delay %q (want e.g. 30, 30s, 500ms, 2m)", raw)
	}
	return value, nil
}

// durationValue is a flag.Value holding milliseconds.
type durationValue struct{ v *int }

func (d durationValue) String() string {
	if d.v == nil {
		return "0"
	}
	return strconv.Itoa(*d.v)
}

func (d durationValue) Set(raw string) error {
	ms, err := parseMillis(raw)
	if err != nil {
		return err
	}
	*d.v = ms
	return nil
}

// optionalDuration is durationValue that records whether it was set, so token
// limits can be updated without resetting the others.
type optionalDuration struct{ p **int }

func (o optionalDuration) String() string {
	if o.p == nil || *o.p == nil {
		return ""
	}
	return strconv.Itoa(**o.p)
}

func (o optionalDuration) Set(raw string) error {
	ms, err := parseMillis(raw)
	if err != nil {
		return err
	}
	*o.p = &ms
	return nil
}

// secondsValue is a flag.Value holding seconds as a float.
type secondsValue struct{ v *float64 }

func (s secondsValue) String() string {
	if s.v == nil {
		return "0"
	}
	return strconv.FormatFloat(*s.v, 'g', -1, 64)
}

func (s secondsValue) Set(raw string) error {
	seconds, err := parseSeconds(raw)
	if err != nil {
		return err
	}
	*s.v = seconds
	return nil
}
