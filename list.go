// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Device is a hub as returned by the v1 hub management endpoints.
type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedOn string `json:"createdOn"`
	Online    bool   `json:"online,omitempty"`
}

func cmdShockers(g *globals, args []string) error {
	if len(args) == 0 {
		return usagef("usage: britzelator shockers list|info|pause|logs ...")
	}
	sub, rest := args[0], args[1:]
	c, err := g.client()
	if err != nil {
		return err
	}
	switch sub {
	case "list", "ls":
		return shockersList(g, c, rest)
	case "info":
		return shockerInfo(g, c, rest)
	case "pause":
		return shockerPause(g, c, rest)
	case "logs":
		return shockerLogs(g, c, rest)
	default:
		return usagef("unknown shockers subcommand %q", sub)
	}
}

func shockersList(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("shockers list")
	scope := fs.String("scope", "all", "which shockers to list: all, own, shared")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator shockers list [--scope all|own|shared]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	index, err := listShockers(c)
	if err != nil {
		return fmt.Errorf("listing shockers: %w", err)
	}
	var filtered []Shocker
	for _, s := range index {
		if *scope == "all" || s.Scope == *scope {
			filtered = append(filtered, s)
		}
	}
	if g.jsonOut {
		return printJSON(filtered)
	}
	if len(filtered) == 0 {
		fmt.Println("no shockers found")
		return nil
	}
	w := newTable(os.Stdout)
	fmt.Fprintln(w, "NAME\tHUB\tSCOPE\tPAUSED\tMODEL\tID")
	for _, s := range filtered {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Name, s.Hub, s.Scope, yesNo(s.Paused), s.Model, s.ID)
	}
	return w.Flush()
}

func shockerInfo(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("shockers info")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator shockers info <shocker>"); err != nil {
		return err
	}
	targets, err := resolveTargets(g, fs.Args()[:1])
	if err != nil {
		return err
	}
	res, err := c.doJSON("GET", "/1/shockers/"+targets[0].ID, nil, nil)
	if err != nil {
		return err
	}
	raw, err := getData(res)
	if err != nil {
		return err
	}
	if g.jsonOut {
		printRawJSON(raw)
		return nil
	}
	printRawJSON(raw)
	return nil
}

func shockerPause(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("shockers pause")
	paused := fs.Bool("paused", true, "set the paused state")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator shockers pause <shocker>... [--paused=false]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator shockers pause <shocker>..."); err != nil {
		return err
	}
	targets, err := resolveTargets(g, fs.Args())
	if err != nil {
		return err
	}
	for _, t := range targets {
		if err := c.Post("/1/shockers/"+t.ID+"/pause", map[string]any{"paused": *paused}, nil); err != nil {
			return err
		}
		g.printfErr("%s → paused=%v\n", t.Label(), *paused)
	}
	return nil
}

func shockerLogs(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("shockers logs")
	limit := fs.Int("limit", 20, "maximum number of log entries to show")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator shockers logs <shocker> [--limit N]"); err != nil {
		return err
	}
	targets, err := resolveTargets(g, fs.Args()[:1])
	if err != nil {
		return err
	}
	res, err := c.doJSON("GET", fmt.Sprintf("/1/shockers/%s/logs?limit=%d", targets[0].ID, *limit), nil, nil)
	if err != nil {
		return err
	}
	raw, err := getData(res)
	if err != nil {
		return err
	}
	if g.jsonOut {
		printRawJSON(raw)
		return nil
	}
	var entries []struct {
		CreatedOn string `json:"createdOn"`
		Type      string `json:"type"`
		Intensity int    `json:"intensity"`
		Duration  int    `json:"duration"`
		Sender    struct {
			Name string `json:"name"`
		} `json:"controlledBy"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		printRawJSON(raw)
		return nil
	}
	w := newTable(os.Stdout)
	fmt.Fprintln(w, "WHEN\tTYPE\tINTENSITY\tDURATION\tBY")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%d\t%dms\t%s\n", e.CreatedOn, e.Type, e.Intensity, e.Duration, e.Sender.Name)
	}
	return w.Flush()
}

func cmdHubs(g *globals, args []string) error {
	if len(args) == 0 {
		return usagef("usage: britzelator hubs list|create|shockers|lcg|pair|delete ...")
	}
	sub, rest := args[0], args[1:]
	c, err := g.client()
	if err != nil {
		return err
	}
	switch sub {
	case "list", "ls":
		return hubsList(g, c, rest)
	case "create":
		return hubCreate(g, c, rest)
	case "shockers":
		return hubShockers(g, c, rest)
	case "lcg":
		return hubLCG(g, c, rest)
	case "pair":
		return hubPair(g, c, rest)
	case "delete", "rm":
		return hubDelete(g, c, rest)
	default:
		return usagef("unknown hubs subcommand %q", sub)
	}
}

func listDevices(c *Client) ([]Device, error) {
	res, err := c.doJSON("GET", "/1/devices", nil, nil)
	if err != nil {
		return nil, err
	}
	raw, err := getData(res)
	if err != nil {
		return nil, err
	}
	var devices []Device
	if err := json.Unmarshal(raw, &devices); err != nil {
		return nil, err
	}
	return devices, nil
}

func hubsList(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("hubs list")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	devices, err := listDevices(c)
	if err != nil {
		return err
	}
	if g.jsonOut {
		return printJSON(devices)
	}
	if len(devices) == 0 {
		fmt.Println("no hubs found")
		return nil
	}
	w := newTable(os.Stdout)
	fmt.Fprintln(w, "NAME\tCREATED\tID")
	for _, d := range devices {
		fmt.Fprintf(w, "%s\t%s\t%s\n", d.Name, d.CreatedOn, d.ID)
	}
	return w.Flush()
}

func hubCreate(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("hubs create")
	name := fs.String("name", "", "hub name")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator hubs create --name <name>")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if *name == "" && len(fs.Args()) == 1 {
		*name = fs.Args()[0]
	}
	if *name == "" {
		return usagef("usage: britzelator hubs create --name <name>")
	}
	// v2 is used where available; hub creation lives at POST /2/devices.
	res, err := c.doJSON("POST", "/2/devices", map[string]string{"name": *name}, nil)
	if err != nil {
		return err
	}
	id := strings.Trim(strings.TrimSpace(string(res.Body)), `"`)
	if g.jsonOut {
		printRawJSON(res.Body)
		return nil
	}
	fmt.Printf("created hub %q: %s\n", *name, id)
	return nil
}

func hubShockers(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("hubs shockers")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator hubs shockers <hub>"); err != nil {
		return err
	}
	devices, err := listDevices(c)
	if err != nil {
		return err
	}
	dev, err := resolveDevice(fs.Args()[0], devices)
	if err != nil {
		return err
	}
	res, err := c.doJSON("GET", "/1/devices/"+dev.ID+"/shockers", nil, nil)
	if err != nil {
		return err
	}
	raw, err := getData(res)
	if err != nil {
		return err
	}
	if g.jsonOut {
		printRawJSON(raw)
		return nil
	}
	var shockers []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		RFID     int    `json:"rfId"`
		Model    string `json:"model"`
		IsPaused bool   `json:"isPaused"`
	}
	if err := json.Unmarshal(raw, &shockers); err != nil {
		printRawJSON(raw)
		return nil
	}
	w := newTable(os.Stdout)
	fmt.Fprintln(w, "NAME\tRFID\tMODEL\tPAUSED\tID")
	for _, s := range shockers {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", s.Name, s.RFID, s.Model, yesNo(s.IsPaused), s.ID)
	}
	return w.Flush()
}

func hubLCG(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("hubs lcg")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator hubs lcg <hub>"); err != nil {
		return err
	}
	devices, err := listDevices(c)
	if err != nil {
		return err
	}
	dev, err := resolveDevice(fs.Args()[0], devices)
	if err != nil {
		return err
	}
	// v2 exposes the live control gateway including its path prefix.
	res, err := c.doJSON("GET", "/2/devices/"+dev.ID+"/lcg", nil, nil)
	if err != nil {
		return err
	}
	if g.jsonOut {
		printRawJSON(res.Body)
		return nil
	}
	var lcg struct {
		Host       string `json:"host"`
		Port       int    `json:"port"`
		PathPrefix string `json:"pathPrefix"`
		Country    string `json:"country"`
	}
	if err := json.Unmarshal(res.Body, &lcg); err != nil {
		printRawJSON(res.Body)
		return nil
	}
	fmt.Printf("%s:%d%s (%s)\n", lcg.Host, lcg.Port, lcg.PathPrefix, lcg.Country)
	return nil
}

func hubPair(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("hubs pair")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator hubs pair <hub>"); err != nil {
		return err
	}
	devices, err := listDevices(c)
	if err != nil {
		return err
	}
	dev, err := resolveDevice(fs.Args()[0], devices)
	if err != nil {
		return err
	}
	res, err := c.doJSON("GET", "/1/devices/"+dev.ID+"/pair", nil, nil)
	if err != nil {
		return err
	}
	raw, err := getData(res)
	if err != nil {
		return err
	}
	fmt.Println(strings.Trim(strings.TrimSpace(string(raw)), `"`))
	return nil
}

func hubDelete(g *globals, c *Client, args []string) error {
	fs := g.newFlagSet("hubs delete")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator hubs delete <hub>"); err != nil {
		return err
	}
	devices, err := listDevices(c)
	if err != nil {
		return err
	}
	dev, err := resolveDevice(fs.Args()[0], devices)
	if err != nil {
		return err
	}
	if err := c.Delete("/1/devices/"+dev.ID, nil); err != nil {
		return err
	}
	g.printfErr("deleted hub %s (%s)\n", dev.Name, dev.ID)
	return nil
}

// resolveDevice matches a hub by UUID, exact name, or unique name substring.
func resolveDevice(arg string, devices []Device) (Device, error) {
	if uuidRe.MatchString(arg) {
		for _, d := range devices {
			if strings.EqualFold(d.ID, arg) {
				return d, nil
			}
		}
		return Device{ID: arg, Name: arg}, nil
	}
	lower := strings.ToLower(arg)
	var exact, partial []Device
	for _, d := range devices {
		name := strings.ToLower(d.Name)
		if name == lower {
			exact = append(exact, d)
		} else if strings.Contains(name, lower) {
			partial = append(partial, d)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) == 0 && len(partial) == 1 {
		return partial[0], nil
	}
	candidates := append(exact, partial...)
	if len(candidates) == 0 {
		return Device{}, fmt.Errorf("no hub matches %q (try `britzelator hubs list`)", arg)
	}
	labels := make([]string, 0, len(candidates))
	for _, d := range candidates {
		labels = append(labels, fmt.Sprintf("%s (%s)", d.Name, d.ID))
	}
	return Device{}, fmt.Errorf("hub %q is ambiguous, candidates: %s", arg, strings.Join(labels, ", "))
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
