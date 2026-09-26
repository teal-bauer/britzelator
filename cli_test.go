// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"bytes"
	"flag"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func testIndex() []Shocker {
	return []Shocker{
		{ID: "3f0a5c9e-2222-4a2b-8c3d-000000000002", Name: "LivingRoom", Hub: "Home Hub", Scope: "own"},
		{ID: "3f0a5c9e-3333-4a2b-8c3d-000000000003", Name: "Collar", Hub: "Home Hub", Scope: "own"},
		{ID: "3f0a5c9e-4444-4a2b-8c3d-000000000004", Name: "Collar", Hub: "Alice Hub", Owner: "Alice", Scope: "shared"},
	}
}

func TestResolveShockersByUUID(t *testing.T) {
	got, err := resolveShockers([]string{"3f0a5c9e-2222-4a2b-8c3d-000000000002"}, testIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "LivingRoom" {
		t.Fatalf("got %+v", got)
	}
}

// A UUID that is not in the listing is passed through: the API is authoritative
// and public-share shockers may not appear in the user's listings.
func TestResolveShockersUnknownUUID(t *testing.T) {
	id := "abcdabcd-1111-4a2b-8c3d-00000000000f"
	got, err := resolveShockers([]string{id}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != id {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveShockersByNameAndQualifier(t *testing.T) {
	cases := []struct {
		arg  string
		want string
	}{
		{"LivingRoom", "3f0a5c9e-2222-4a2b-8c3d-000000000002"},
		{"livingroom", "3f0a5c9e-2222-4a2b-8c3d-000000000002"},
		{"Liv", "3f0a5c9e-2222-4a2b-8c3d-000000000002"},
		{"Alice/Collar", "3f0a5c9e-4444-4a2b-8c3d-000000000004"},
		{"Home Hub/Collar", "3f0a5c9e-3333-4a2b-8c3d-000000000003"},
	}
	for _, tc := range cases {
		got, err := resolveShockers([]string{tc.arg}, testIndex())
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.arg, err)
		}
		if len(got) != 1 || got[0].ID != tc.want {
			t.Fatalf("%s: got %+v", tc.arg, got)
		}
	}
}

func TestResolveShockersAmbiguous(t *testing.T) {
	_, err := resolveShockers([]string{"Collar"}, testIndex())
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestResolveShockersNoMatch(t *testing.T) {
	_, err := resolveShockers([]string{"Nope"}, testIndex())
	if err == nil || !strings.Contains(err.Error(), "no shocker matches") {
		t.Fatalf("expected no-match error, got %v", err)
	}
}

func TestResolveShockersDeduplicates(t *testing.T) {
	got, err := resolveShockers([]string{"Liv", "LivingRoom"}, testIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one target, got %+v", got)
	}
}

func TestNormalizeMode(t *testing.T) {
	for arg, want := range map[string]string{
		"shock": "Shock", "zap": "Shock", "vibrate": "Vibrate", "vib": "Vibrate",
		"tone": "Sound", "sound": "Sound", "TONE": "Sound", "stop": "Stop",
	} {
		got, err := normalizeMode(arg)
		if err != nil || got != want {
			t.Fatalf("%s: got %q, %v", arg, got, err)
		}
	}
	if _, err := normalizeMode("sparkle"); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestValidateControl(t *testing.T) {
	if err := validateControl("Shock", 0, minDurationMs); err != nil {
		t.Fatalf("boundaries should be valid: %v", err)
	}
	if err := validateControl("Shock", 101, 1000); err == nil {
		t.Fatal("intensity above 100 must fail")
	}
	if err := validateControl("Shock", 10, 299); err == nil {
		t.Fatal("duration below 300ms must fail")
	}
	if err := validateControl("Shock", 10, 65536); err == nil {
		t.Fatal("duration above 65535ms must fail")
	}
}

func TestBuildControlRequest(t *testing.T) {
	targets := []Shocker{{ID: "id-1"}, {ID: "id-2"}}
	req := buildControlRequest(targets, "Stop", 0, minDurationMs, false, "")
	if len(req.Shocks) != 2 {
		t.Fatalf("got %d shocks", len(req.Shocks))
	}
	if req.CustomName != nil {
		t.Fatal("customName should be omitted when empty")
	}
	req = buildControlRequest(targets[:1], "Shock", 42, 900, true, "cli")
	if req.Shocks[0].Exclusive == nil || !*req.Shocks[0].Exclusive {
		t.Fatal("exclusive flag not propagated")
	}
	if req.CustomName == nil || *req.CustomName != "cli" {
		t.Fatal("customName not propagated")
	}
}

func TestSplitFlagsAcceptsPositionalsLast(t *testing.T) {
	g := &globals{}
	gfs := flag.NewFlagSet("britzelator", flag.ContinueOnError)
	gfs.StringVar(&g.token, "token", "", "")
	gfs.BoolVar(&g.jsonOut, "json", false, "")
	g.fs = gfs

	fs := g.newFlagSet("control")
	mode := fs.String("mode", "shock", "")
	duration := fs.Int("duration", 1000, "")

	args := []string{"LivingRoom", "--mode", "vibrate", "--duration", "500", "Collar"}
	if err := parseOrHelp(fs, args); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if *mode != "vibrate" || *duration != 500 {
		t.Fatalf("flags not applied: mode=%q duration=%d", *mode, *duration)
	}
	got := fs.Args()
	if len(got) != 2 || got[0] != "LivingRoom" || got[1] != "Collar" {
		t.Fatalf("positionals out of order: %v", got)
	}
}

// Global flags may appear after the subcommand as well as before it.
func TestGlobalFlagsAfterSubcommand(t *testing.T) {
	g := &globals{}
	gfs := flag.NewFlagSet("britzelator", flag.ContinueOnError)
	gfs.BoolVar(&g.jsonOut, "json", false, "")
	g.fs = gfs

	fs := g.newFlagSet("tokens list")
	if err := parseOrHelp(fs, []string{"--json"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !g.jsonOut {
		t.Fatal("--json did not reach the global flag")
	}
}

func TestSplitFlagsDoubleDash(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("dry-run", false, "")
	flags, positional, err := splitFlags(fs, []string{"--dry-run", "--", "--not-a-flag"})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(flags) != 1 || flags[0] != "--dry-run" {
		t.Fatalf("flags: %v", flags)
	}
	if len(positional) != 1 || positional[0] != "--not-a-flag" {
		t.Fatalf("positionals: %v", positional)
	}
}

func TestSplitFlagsUnknown(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if _, _, err := splitFlags(fs, []string{"--nope"}); err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func TestParseHeaders(t *testing.T) {
	got, err := parseHeaders("DeviceToken: abc, X-Test: 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["DeviceToken"] != "abc" || got["X-Test"] != "1" {
		t.Fatalf("got %v", got)
	}
	if _, err := parseHeaders("broken"); err == nil {
		t.Fatal("expected error for a header without a colon")
	}
}

func TestParseDate(t *testing.T) {
	got, err := parseDate("2027-01-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2027-01-02T00:00:00Z" {
		t.Fatalf("got %q", got)
	}
	if _, err := parseDate("02/01/2027"); err == nil {
		t.Fatal("expected error for an unsupported format")
	}
}

func TestTokenLimitFlagsApply(t *testing.T) {
	min := 5
	max := 40
	f := &tokenLimitFlags{minIntensity: &min, maxIntensity: &max}
	got, err := f.apply(defaultShockerControl())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Intensity.Min != 5 || got.Intensity.Max != 40 {
		t.Fatalf("got %+v", got.Intensity)
	}
	if got.Duration.Min != minDurationMs || got.Duration.Max != maxDurationMs {
		t.Fatalf("untouched duration limits changed: %+v", got.Duration)
	}

	bad := 100
	f = &tokenLimitFlags{minDuration: &bad}
	if _, err := f.apply(defaultShockerControl()); err == nil {
		t.Fatal("expected validation error for duration below 300ms")
	}

	// A minimum above the current maximum must be rejected as well.
	tooHigh := 90000
	f = &tokenLimitFlags{maxDuration: &tooHigh}
	if _, err := f.apply(defaultShockerControl()); err == nil {
		t.Fatal("expected validation error for duration above 65535ms")
	}
}

func TestProblemError(t *testing.T) {
	p := &Problem{Type: "Authentication.TokenInvalid", Title: "The token is invalid", Status: 401, TraceID: "t1"}
	msg := p.Error()
	for _, want := range []string{"401", "Authentication.TokenInvalid", "trace t1"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	content := "# comment\nexport OPENSHOCK_TOKEN=\"quoted value\"\nOPENSHOCK_API_TOKEN=plain\n\nBAD_LINE\nOPENSHOCK_SERVER='http://localhost:1'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadDotEnv(path)
	if got["OPENSHOCK_TOKEN"] != "quoted value" {
		t.Fatalf("quoted value mangled: %q", got["OPENSHOCK_TOKEN"])
	}
	if got["OPENSHOCK_API_TOKEN"] != "plain" {
		t.Fatalf("plain value missing: %q", got["OPENSHOCK_API_TOKEN"])
	}
	if got["OPENSHOCK_SERVER"] != "http://localhost:1" {
		t.Fatalf("server value wrong: %q", got["OPENSHOCK_SERVER"])
	}
	if _, ok := got["BAD_LINE"]; ok {
		t.Fatal("line without '=' should be ignored")
	}
	if len(loadDotEnv(dir+"/missing")) != 0 {
		t.Fatal("missing file should yield no values")
	}
	if len(loadDotEnv("")) != 0 {
		t.Fatal("empty path should yield no values")
	}
}

func TestParseMillis(t *testing.T) {
	cases := map[string]int{
		"500":    500,
		"500ms":  500,
		"1s":     1000,
		"1.5s":   1500,
		"2S":     2000,
		"1m":     60000,
		"305ms":  305,
		" 750ms": 750,
	}
	for in, want := range cases {
		got, err := parseMillis(in)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %d want %d", in, got, want)
		}
	}
	for _, bad := range []string{"", "abc", "5x", "ms", "1.2.3"} {
		if _, err := parseMillis(bad); err == nil {
			t.Fatalf("%q should not parse", bad)
		}
	}
}

func TestParseSeconds(t *testing.T) {
	cases := map[string]float64{
		"30":    30,
		"30s":   30,
		"0.5":   0.5,
		"500ms": 0.5,
		"2m":    120,
		"1.5s":  1.5,
	}
	for in, want := range cases {
		got, err := parseSeconds(in)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %g want %g", in, got, want)
		}
	}
	for _, bad := range []string{"", "soon", "5x"} {
		if _, err := parseSeconds(bad); err == nil {
			t.Fatalf("%q should not parse", bad)
		}
	}
}

// bare numbers keep their documented meaning: milliseconds for durations,
// seconds for delays.
func TestBareNumberUnits(t *testing.T) {
	ms, err := parseMillis("3000")
	if err != nil || ms != 3000 {
		t.Fatalf("bare duration should be ms: %d %v", ms, err)
	}
	s, err := parseSeconds("3")
	if err != nil || s != 3 {
		t.Fatalf("bare delay should be seconds: %g %v", s, err)
	}
}

func TestDurationAndSecondsFlagValues(t *testing.T) {
	d := 0
	if err := (durationValue{&d}).Set("1.5s"); err != nil || d != 1500 {
		t.Fatalf("durationValue: %d %v", d, err)
	}
	var s float64
	if err := (secondsValue{&s}).Set("250ms"); err != nil || s != 0.25 {
		t.Fatalf("secondsValue: %g %v", s, err)
	}
	var opt *int
	od := optionalDuration{&opt}
	if err := od.Set("2s"); err != nil || opt == nil || *opt != 2000 {
		t.Fatalf("optionalDuration: %v %v", opt, err)
	}
	if od.String() != "2000" {
		t.Fatalf("optionalDuration String: %q", od.String())
	}
	if got := (optionalDuration{&opt}).String(); got != "2000" {
		t.Fatalf("optionalDuration String with set pointer: %q", got)
	}
}

func TestRoundRange(t *testing.T) {
	// no count flags: fixed --count
	got := randomOptions{count: 4}
	lo, hi, err := got.roundRange()
	if err != nil || lo != 4 || hi != 4 {
		t.Fatalf("count only: %d %d %v", lo, hi, err)
	}

	min, max := 3, 10
	got = randomOptions{countMin: &min, countMax: &max}
	lo, hi, err = got.roundRange()
	if err != nil || lo != 3 || hi != 10 {
		t.Fatalf("range: %d %d %v", lo, hi, err)
	}

	// one-sided ranges collapse to the supplied end
	got = randomOptions{countMin: &min}
	lo, hi, err = got.roundRange()
	if err != nil || lo != 3 || hi != 3 {
		t.Fatalf("min only: %d %d %v", lo, hi, err)
	}
	got = randomOptions{countMax: &max}
	lo, hi, err = got.roundRange()
	if err != nil || lo != 10 || hi != 10 {
		t.Fatalf("max only: %d %d %v", lo, hi, err)
	}

	low, high := 10, 3
	got = randomOptions{countMin: &low, countMax: &high}
	if _, _, err := got.roundRange(); err == nil {
		t.Fatal("inverted range should fail")
	}
}

func TestDescribeRounds(t *testing.T) {
	if got := describeRounds(0, 0); got != "until interrupted" {
		t.Fatalf("got %q", got)
	}
	if got := describeRounds(5, 5); got != "5" {
		t.Fatalf("got %q", got)
	}
	if got := describeRounds(3, 10); got != "3..10 per loop (0 = until interrupted)" {
		t.Fatalf("got %q", got)
	}
}

// quiet mode silences progress while leaving errors to main.
func TestQuietSuppressesProgress(t *testing.T) {
	var buf bytes.Buffer
	g := &globals{errOut: &buf}
	g.printfErr("progress\n")
	if buf.String() != "progress\n" {
		t.Fatalf("expected output, got %q", buf.String())
	}

	buf.Reset()
	g.quiet = true
	g.printfErr("progress\n")
	if buf.String() != "" {
		t.Fatalf("quiet mode should print nothing, got %q", buf.String())
	}
}

func TestRandIntInclusive(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		v := randInt(rnd, 3, 5)
		if v < 3 || v > 5 {
			t.Fatalf("out of range: %d", v)
		}
		seen[v] = true
	}
	if !seen[3] || !seen[4] || !seen[5] {
		t.Fatalf("bounds not covered: %v", seen)
	}
	if v := randInt(rnd, 7, 7); v != 7 {
		t.Fatalf("degenerate range should return the value: %d", v)
	}
}
