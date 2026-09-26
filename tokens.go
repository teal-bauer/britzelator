// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type ShockerControlSettings struct {
	Paused    bool                   `json:"paused"`
	Intensity IntensityLimitSettings `json:"intensity"`
	Duration  DurationLimitSettings  `json:"duration"`
}

type IntensityLimitSettings struct {
	Min  int    `json:"min"`
	Max  int    `json:"max"`
	Mode string `json:"mode"`
}

type DurationLimitSettings struct {
	Min  int    `json:"min"`
	Max  int    `json:"max"`
	Mode string `json:"mode"`
}

type TokenResponseV2 struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	CreatedOn      string                 `json:"createdOn"`
	ValidUntil     *string                `json:"validUntil"`
	LastUsed       *string                `json:"lastUsed"`
	Permissions    []string               `json:"permissions"`
	ShockerControl ShockerControlSettings `json:"shockerControl"`
}

func defaultShockerControl() ShockerControlSettings {
	return ShockerControlSettings{
		Intensity: IntensityLimitSettings{Min: 0, Max: maxIntensity, Mode: "Clamp"},
		Duration:  DurationLimitSettings{Min: minDurationMs, Max: maxDurationMs, Mode: "Clamp"},
	}
}

func cmdTokens(g *globals, args []string) error {
	if len(args) == 0 {
		return usagef("usage: britzelator tokens self|list|get|create|update|pause|revoke ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "self":
		return tokensSelf(g, rest)
	case "list", "ls":
		return tokensList(g, rest)
	case "get":
		return tokensGet(g, rest)
	case "create":
		return tokensCreate(g, rest)
	case "update":
		return tokensUpdate(g, rest)
	case "pause":
		return tokensPause(g, rest)
	case "revoke", "delete", "rm":
		return tokensRevoke(g, rest)
	default:
		return usagef("unknown tokens subcommand %q", sub)
	}
}

func tokensSelf(g *globals, args []string) error {
	fs := g.newFlagSet("tokens self")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	c, err := g.client()
	if err != nil {
		return err
	}
	var tok TokenResponseV2
	if err := c.Get("/2/tokens/self", &tok); err != nil {
		return err
	}
	if g.jsonOut {
		return printJSON(tok)
	}
	printToken(os.Stdout, tok)
	return nil
}

func tokensList(g *globals, args []string) error {
	fs := g.newFlagSet("tokens list")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	c, err := g.client()
	if err != nil {
		return err
	}
	var toks []TokenResponseV2
	if err := c.Get("/2/tokens", &toks); err != nil {
		return err
	}
	if g.jsonOut {
		return printJSON(toks)
	}
	if len(toks) == 0 {
		fmt.Println("no tokens found")
		return nil
	}
	w := newTable(os.Stdout)
	fmt.Fprintln(w, "NAME\tPAUSED\tLIMITS\tVALID UNTIL\tLAST USED\tID")
	for _, t := range toks {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			t.Name, yesNo(t.ShockerControl.Paused), limitSummary(t.ShockerControl),
			derefOr(t.ValidUntil, "never"), derefOr(t.LastUsed, "never"), t.ID)
	}
	return w.Flush()
}

func tokensGet(g *globals, args []string) error {
	fs := g.newFlagSet("tokens get")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator tokens get <token-id>"); err != nil {
		return err
	}
	c, err := g.client()
	if err != nil {
		return err
	}
	var tok TokenResponseV2
	if err := c.Get("/2/tokens/"+fs.Args()[0], &tok); err != nil {
		return err
	}
	if g.jsonOut {
		return printJSON(tok)
	}
	printToken(os.Stdout, tok)
	return nil
}

// optionalInt flags carry "not supplied" as nil so that updates can start from
// a token's current limits instead of resetting them.
type optionalInt struct{ p **int }

func (o optionalInt) String() string {
	if o.p == nil || *o.p == nil {
		return ""
	}
	return strconv.Itoa(**o.p)
}

func (o optionalInt) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*o.p = &n
	return nil
}

func intFlag(p **int) flag.Value { return optionalInt{p} }

type tokenLimitFlags struct {
	name          string
	permissions   string
	minIntensity  *int
	maxIntensity  *int
	minDuration   *int
	maxDuration   *int
	intensityMode string
	durationMode  string
	paused        *bool
	validUntil    string
}

func defineTokenFlags(fs *flag.FlagSet, f *tokenLimitFlags, withValidUntil bool) {
	fs.StringVar(&f.name, "name", "", "token name")
	fs.StringVar(&f.permissions, "permissions", "shockers.use", "comma-separated permissions (e.g. shockers.use,shockers.pause)")
	fs.Var(intFlag(&f.minIntensity), "min-intensity", "minimum intensity allowed for this token")
	fs.Var(intFlag(&f.maxIntensity), "max-intensity", "maximum intensity allowed for this token")
	fs.Var(optionalDuration{&f.minDuration}, "min-duration", "minimum duration allowed for this token, e.g. 300ms")
	fs.Var(optionalDuration{&f.maxDuration}, "max-duration", "maximum duration allowed for this token, e.g. 5s")
	fs.StringVar(&f.intensityMode, "intensity-mode", "", "limit mode for intensity: Clamp or Lerp")
	fs.StringVar(&f.durationMode, "duration-mode", "", "limit mode for duration: Clamp or Lerp")
	f.paused = fs.Bool("paused", false, "pause the token so it cannot send control messages")
	if withValidUntil {
		fs.StringVar(&f.validUntil, "valid-until", "", "expiry date (RFC3339 or YYYY-MM-DD); default never")
	}
}

// apply overlays the supplied flags on top of the given settings and validates
// the result against the API's documented bounds.
func (f *tokenLimitFlags) apply(s ShockerControlSettings) (ShockerControlSettings, error) {
	if f.minIntensity != nil {
		s.Intensity.Min = *f.minIntensity
	}
	if f.maxIntensity != nil {
		s.Intensity.Max = *f.maxIntensity
	}
	if f.intensityMode != "" {
		s.Intensity.Mode = normalizeLimitMode(f.intensityMode)
	}
	if f.minDuration != nil {
		s.Duration.Min = *f.minDuration
	}
	if f.maxDuration != nil {
		s.Duration.Max = *f.maxDuration
	}
	if f.durationMode != "" {
		s.Duration.Mode = normalizeLimitMode(f.durationMode)
	}
	if s.Intensity.Min < minIntensity || s.Intensity.Max > maxIntensity || s.Intensity.Max < s.Intensity.Min {
		return s, fmt.Errorf("intensity limits invalid: %d..%d (allowed %d..%d)", s.Intensity.Min, s.Intensity.Max, minIntensity, maxIntensity)
	}
	if s.Duration.Min < minDurationMs || s.Duration.Max > maxDurationMs || s.Duration.Max < s.Duration.Min {
		return s, fmt.Errorf("duration limits invalid: %d..%dms (allowed %d..%dms)", s.Duration.Min, s.Duration.Max, minDurationMs, maxDurationMs)
	}
	return s, nil
}

func normalizeLimitMode(m string) string {
	if strings.EqualFold(m, "lerp") {
		return "Lerp"
	}
	return "Clamp"
}

func tokensCreate(g *globals, args []string) error {
	fs := g.newFlagSet("tokens create")
	f := &tokenLimitFlags{}
	defineTokenFlags(fs, f, true)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator tokens create --name <name> [--permissions p1,p2] [--max-intensity N] [--max-duration MS] [--valid-until DATE]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if f.name == "" {
		return usagef("--name is required")
	}
	settings, err := f.apply(defaultShockerControl())
	if err != nil {
		return err
	}
	settings.Paused = *f.paused

	payload := map[string]any{
		"name":           f.name,
		"permissions":    splitList(f.permissions),
		"shockerControl": settings,
	}
	if f.validUntil != "" {
		ts, err := parseDate(f.validUntil)
		if err != nil {
			return err
		}
		payload["validUntil"] = ts
	}

	c, err := g.client()
	if err != nil {
		return err
	}
	var created struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if err := c.Post("/2/tokens", payload, &created); err != nil {
		return err
	}
	if g.jsonOut {
		return printJSON(created)
	}
	fmt.Printf("created token %q (%s)\n", created.Name, created.ID)
	fmt.Printf("token: %s\n", created.Token)
	g.printfErr("store it now: the API does not return token secrets again\n")
	return nil
}

func tokensUpdate(g *globals, args []string) error {
	fs := g.newFlagSet("tokens update")
	f := &tokenLimitFlags{}
	defineTokenFlags(fs, f, false)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator tokens update <token-id> [--name N] [--permissions p1,p2] [--max-intensity N] [--paused]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator tokens update <token-id> [flags]"); err != nil {
		return err
	}
	id := fs.Args()[0]
	c, err := g.client()
	if err != nil {
		return err
	}
	// PATCH replaces the whole representation, so start from the current token.
	var current TokenResponseV2
	if err := c.Get("/2/tokens/"+id, &current); err != nil {
		return err
	}
	settings, err := f.apply(current.ShockerControl)
	if err != nil {
		return err
	}
	if flagWasSet(fs, "paused") {
		settings.Paused = *f.paused
	}
	name := current.Name
	if f.name != "" {
		name = f.name
	}
	permissions := current.Permissions
	if flagWasSet(fs, "permissions") {
		permissions = splitList(f.permissions)
	}
	payload := map[string]any{
		"name":           name,
		"permissions":    permissions,
		"shockerControl": settings,
	}
	if err := c.Patch("/2/tokens/"+id, payload, nil); err != nil {
		return err
	}
	g.printfErr("updated token %s\n", id)
	return nil
}

func tokensPause(g *globals, args []string) error {
	fs := g.newFlagSet("tokens pause")
	paused := fs.Bool("paused", true, "paused state to set")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator tokens pause <token-id> [--paused=false]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator tokens pause <token-id>"); err != nil {
		return err
	}
	c, err := g.client()
	if err != nil {
		return err
	}
	var res struct {
		Paused bool `json:"paused"`
	}
	if err := c.Patch("/2/tokens/"+fs.Args()[0]+"/paused", map[string]bool{"paused": *paused}, &res); err != nil {
		return err
	}
	g.printfErr("token %s paused=%v\n", fs.Args()[0], res.Paused)
	return nil
}

// tokensRevoke uses the v1 endpoint: it is the only deletion route an API
// token is permitted to call, since v2 exposes no delete.
func tokensRevoke(g *globals, args []string) error {
	fs := g.newFlagSet("tokens revoke")
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator tokens revoke <token-id>"); err != nil {
		return err
	}
	c, err := g.client()
	if err != nil {
		return err
	}
	for _, id := range fs.Args() {
		if err := c.Delete("/1/tokens/"+id, nil); err != nil {
			return err
		}
		g.printfErr("revoked token %s\n", id)
	}
	return nil
}

func printToken(w io.Writer, t TokenResponseV2) {
	fmt.Fprintf(w, "name:        %s\n", t.Name)
	fmt.Fprintf(w, "id:          %s\n", t.ID)
	fmt.Fprintf(w, "created:     %s\n", t.CreatedOn)
	fmt.Fprintf(w, "valid until: %s\n", derefOr(t.ValidUntil, "never"))
	fmt.Fprintf(w, "last used:   %s\n", derefOr(t.LastUsed, "never"))
	fmt.Fprintf(w, "permissions: %s\n", strings.Join(t.Permissions, ", "))
	fmt.Fprintf(w, "control:     paused=%v %s\n", t.ShockerControl.Paused, limitSummary(t.ShockerControl))
}

func limitSummary(s ShockerControlSettings) string {
	return fmt.Sprintf("i %d..%d (%s), d %d..%dms (%s)",
		s.Intensity.Min, s.Intensity.Max, s.Intensity.Mode,
		s.Duration.Min, s.Duration.Max, s.Duration.Mode)
}

func derefOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseDate accepts RFC3339 or a plain YYYY-MM-DD date.
func parseDate(s string) (string, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("invalid date %q (want RFC3339 or YYYY-MM-DD)", s)
}
