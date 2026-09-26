// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Shocker is a flattened view of a shocker reachable with the current token.
type Shocker struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Hub    string `json:"hub,omitempty"`
	HubID  string `json:"hubId,omitempty"`
	Owner  string `json:"owner,omitempty"` // set only for shockers shared by another user
	Scope  string `json:"scope"`           // "own" or "shared"
	Paused bool   `json:"paused"`
	Model  string `json:"model,omitempty"`
}

// Label is the human-facing qualified name used in listings and ambiguity errors.
func (s Shocker) Label() string {
	qualifier := s.Hub
	if s.Owner != "" {
		qualifier = s.Owner
	}
	if qualifier == "" {
		return s.Name
	}
	return qualifier + "/" + s.Name
}

type ownDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Shockers []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		RFID     int    `json:"rfId"`
		Model    string `json:"model"`
		IsPaused bool   `json:"isPaused"`
	} `json:"shockers"`
}

type sharedOwner struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Devices []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Shockers []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			IsPaused bool   `json:"isPaused"`
		} `json:"shockers"`
	} `json:"devices"`
}

// listShockers loads the caller's own shockers plus those shared with them.
// The v1 endpoints are the only ones exposing listings; v2 covers control.
func listShockers(c *Client) ([]Shocker, error) {
	var out []Shocker

	ownRes, err := c.doJSON("GET", "/1/shockers/own", nil, nil)
	if err != nil {
		return nil, err
	}
	ownRaw, err := getData(ownRes)
	if err != nil {
		return nil, err
	}
	var devices []ownDevice
	if err := json.Unmarshal(ownRaw, &devices); err != nil {
		return nil, err
	}
	for _, d := range devices {
		for _, s := range d.Shockers {
			out = append(out, Shocker{
				ID: s.ID, Name: s.Name, Hub: d.Name, HubID: d.ID,
				Scope: "own", Paused: s.IsPaused, Model: s.Model,
			})
		}
	}

	sharedRes, err := c.doJSON("GET", "/1/shockers/shared", nil, nil)
	if err == nil {
		if sharedRaw, err := getData(sharedRes); err == nil {
			var owners []sharedOwner
			if json.Unmarshal(sharedRaw, &owners) == nil {
				for _, o := range owners {
					for _, d := range o.Devices {
						for _, s := range d.Shockers {
							out = append(out, Shocker{
								ID: s.ID, Name: s.Name, Hub: d.Name, HubID: d.ID,
								Owner: o.Name, Scope: "shared", Paused: s.IsPaused,
							})
						}
					}
				}
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope == "own"
		}
		if out[i].Label() != out[j].Label() {
			return out[i].Label() < out[j].Label()
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// resolveTargets resolves CLI arguments to shockers. Arguments that are all
// UUIDs need no lookup, so a token is only required for name-based selection.
func resolveTargets(g *globals, args []string) ([]Shocker, error) {
	allIDs := true
	for _, a := range args {
		if !uuidRe.MatchString(a) {
			allIDs = false
			break
		}
	}
	if allIDs {
		return resolveShockers(args, nil)
	}
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	index, err := listShockers(c)
	if err != nil {
		return nil, fmt.Errorf("listing shockers: %w", err)
	}
	return resolveShockers(args, index)
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// resolveShockers maps CLI arguments to shockers. An argument may be a UUID,
// a qualified "hub/name" (or "owner/name") label, an exact name, or a unique
// case-insensitive substring of a name.
func resolveShockers(args []string, index []Shocker) ([]Shocker, error) {
	byID := map[string]Shocker{}
	for _, s := range index {
		byID[strings.ToLower(s.ID)] = s
	}

	var out []Shocker
	seen := map[string]bool{}
	for _, arg := range args {
		if uuidRe.MatchString(arg) {
			if s, ok := byID[strings.ToLower(arg)]; ok {
				if !seen[s.ID] {
					seen[s.ID] = true
					out = append(out, s)
				}
				continue
			}
			// Not in the listing (e.g. a public share); the API decides.
			if !seen[strings.ToLower(arg)] {
				seen[strings.ToLower(arg)] = true
				out = append(out, Shocker{ID: arg, Name: arg, Scope: "unknown"})
			}
			continue
		}

		matches := matchByName(arg, index)
		if len(matches) == 1 {
			if !seen[matches[0].ID] {
				seen[matches[0].ID] = true
				out = append(out, matches[0])
			}
			continue
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no shocker matches %q (try `britzelator shockers list`)", arg)
		}
		labels := make([]string, 0, len(matches))
		for _, m := range matches {
			labels = append(labels, fmt.Sprintf("%s (%s)", m.Label(), m.ID))
		}
		return nil, fmt.Errorf("shocker %q is ambiguous, candidates: %s", arg, strings.Join(labels, ", "))
	}
	return out, nil
}

func matchByName(arg string, index []Shocker) []Shocker {
	lower := strings.ToLower(arg)

	var exactName, exactLabel, partial []Shocker
	for _, s := range index {
		name := strings.ToLower(s.Name)
		label := strings.ToLower(s.Label())
		switch {
		case name == lower:
			exactName = append(exactName, s)
		case label == lower:
			exactLabel = append(exactLabel, s)
		case strings.Contains(name, lower) || strings.Contains(label, lower):
			partial = append(partial, s)
		}
	}
	if len(exactName) > 0 {
		return exactName
	}
	if len(exactLabel) > 0 {
		return exactLabel
	}
	return partial
}
