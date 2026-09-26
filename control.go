// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Control is one entry of POST /2/shockers/control.
type Control struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Intensity int    `json:"intensity"`
	Duration  int    `json:"duration"`
	Exclusive *bool  `json:"exclusive,omitempty"`
}

type ControlRequest struct {
	Shocks     []Control `json:"shocks"`
	CustomName *string   `json:"customName,omitempty"`
}

const (
	minDurationMs = 300
	maxDurationMs = 65535
	minIntensity  = 0
	maxIntensity  = 100
)

// normalizeMode maps CLI spellings onto the ControlType enum.
func normalizeMode(mode string) (string, error) {
	switch strings.ToLower(mode) {
	case "shock", "zap":
		return "Shock", nil
	case "vibrate", "vib", "buzz":
		return "Vibrate", nil
	case "sound", "tone", "beep":
		return "Sound", nil
	case "stop":
		return "Stop", nil
	}
	return "", fmt.Errorf("unknown mode %q (want shock, vibrate, tone/sound, or stop)", mode)
}

func validateControl(mode string, intensity, duration int) error {
	if mode != "Stop" && (intensity < minIntensity || intensity > maxIntensity) {
		return fmt.Errorf("intensity %d out of range %d..%d", intensity, minIntensity, maxIntensity)
	}
	if duration < minDurationMs || duration > maxDurationMs {
		return fmt.Errorf("duration %dms out of range %d..%dms", duration, minDurationMs, maxDurationMs)
	}
	return nil
}

func buildControlRequest(targets []Shocker, mode string, intensity, duration int, exclusive bool, customName string) ControlRequest {
	req := ControlRequest{Shocks: make([]Control, 0, len(targets))}
	for _, t := range targets {
		one := Control{ID: t.ID, Type: mode, Intensity: intensity, Duration: duration}
		if exclusive {
			one.Exclusive = boolPtr(true)
		}
		req.Shocks = append(req.Shocks, one)
	}
	if customName != "" {
		req.CustomName = &customName
	}
	return req
}

func boolPtr(v bool) *bool { return &v }

func describeTargets(targets []Shocker) string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.Label())
	}
	return strings.Join(names, ", ")
}

func cmdControl(g *globals, args []string) error {
	fs := g.newFlagSet("control")
	mode := fs.String("mode", "shock", "control mode: shock, vibrate, tone (sound), stop")
	fs.StringVar(mode, "type", "shock", "alias for --mode")
	intensity := fs.Int("intensity", 50, "intensity 0..100")
	duration := 1000
	fs.Var(durationValue{&duration}, "duration", "duration, e.g. 1000, 1000ms, 1.5s (300ms..65535ms)")
	exclusive := fs.Bool("exclusive", false, "mark the control as exclusive")
	customName := fs.String("name", "", "customName recorded in the control log")
	dryRun := fs.Bool("dry-run", false, "print the request without sending it")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator control <shocker>... [--mode shock|vibrate|tone|stop] [--intensity 0-100] [--duration ms]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator control <shocker>... [--mode ...] [--intensity ...] [--duration ...]"); err != nil {
		return err
	}

	typeName, err := normalizeMode(*mode)
	if err != nil {
		return err
	}
	if typeName == "Stop" && !flagWasSet(fs, "intensity") {
		*intensity = 0
	}
	if err := validateControl(typeName, *intensity, duration); err != nil {
		return err
	}

	targets, err := resolveTargets(g, fs.Args())
	if err != nil {
		return err
	}

	payload := buildControlRequest(targets, typeName, *intensity, duration, *exclusive, *customName)
	if *dryRun {
		raw, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(raw))
		return nil
	}
	c, err := g.client()
	if err != nil {
		return err
	}
	if err := c.Post("/2/shockers/control", payload, nil); err != nil {
		return err
	}
	g.printfErr("sent %s i=%d d=%dms → %s\n", typeName, *intensity, duration, describeTargets(targets))
	return nil
}

func cmdStop(g *globals, args []string) error {
	fs := g.newFlagSet("stop")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator stop <shocker>...")
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator stop <shocker>..."); err != nil {
		return err
	}
	targets, err := resolveTargets(g, fs.Args())
	if err != nil {
		return err
	}
	c, err := g.client()
	if err != nil {
		return err
	}
	payload := buildControlRequest(targets, "Stop", 0, minDurationMs, false, "")
	if err := c.Post("/2/shockers/control", payload, nil); err != nil {
		return err
	}
	g.printfErr("stopped → %s\n", describeTargets(targets))
	return nil
}
