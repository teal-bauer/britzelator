// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

type randomAction struct {
	mode   string
	weight int
}

func parseRandomActions(mode, weights string) ([]randomAction, error) {
	modes := strings.Split(mode, "/")
	var parts []string
	if weights != "" {
		parts = strings.Split(weights, ":")
		if len(parts) != len(modes) {
			return nil, usagef("--weights must have one weight per --mode action (%d)", len(modes))
		}
	}
	actions := make([]randomAction, 0, len(modes))
	seen := make(map[string]bool)
	total := 0
	for i, raw := range modes {
		name, err := normalizeMode(strings.TrimSpace(raw))
		if err != nil {
			return nil, usagef("%v", err)
		}
		if name == "Stop" {
			return nil, usagef("--mode stop makes no sense for random mode")
		}
		if seen[name] {
			return nil, usagef("--mode repeats action %s", name)
		}
		seen[name] = true
		weight := 1
		if parts != nil {
			weight, err = strconv.Atoi(strings.TrimSpace(parts[i]))
			if err != nil || weight < 0 {
				return nil, usagef("--weights must be nonnegative integers")
			}
		}
		if weight > int(^uint(0)>>1)-total {
			return nil, usagef("--weights total is too large")
		}
		total += weight
		actions = append(actions, randomAction{mode: name, weight: weight})
	}
	if total == 0 {
		return nil, usagef("--weights must include at least one positive weight")
	}
	return actions, nil
}

func describeActions(actions []randomAction) string {
	if len(actions) == 1 {
		return actions[0].mode
	}
	parts := make([]string, len(actions))
	for i, action := range actions {
		parts[i] = fmt.Sprintf("%s:%d", action.mode, action.weight)
	}
	return strings.Join(parts, "/")
}

func (o randomOptions) drawMode(rnd *rand.Rand) string {
	if len(o.actions) == 0 {
		return o.mode
	}
	if len(o.actions) == 1 {
		return o.actions[0].mode
	}
	total := 0
	for _, action := range o.actions {
		total += action.weight
	}
	draw := rnd.Intn(total)
	for _, action := range o.actions {
		if draw < action.weight {
			return action.mode
		}
		draw -= action.weight
	}
	panic("unreachable action draw")
}
