// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var version = "0.1.0"

// usageError marks a bad invocation: reported without an "error:" prefix chain
// and exiting with status 2.
type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }

type globals struct {
	token      string
	session    string
	server     string
	userAgent  string
	configPath string
	envFile    string
	timeout    time.Duration
	jsonOut    bool
	verbose    bool
	quiet      bool
	errOut     io.Writer
	file       FileConfig
	fs         *flag.FlagSet
}

func parseGlobals(args []string) (*globals, []string, error) {
	g := &globals{}
	fs := flag.NewFlagSet("britzelator", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&g.token, "token", "", "API token (default: $OPENSHOCK_TOKEN, then config file)")
	fs.StringVar(&g.session, "session", "", "session cookie value for endpoints that require a user session")
	fs.StringVar(&g.server, "server", "", "API base URL (default "+defaultServer+")")
	fs.StringVar(&g.userAgent, "user-agent", "", "User-Agent sent with every request")
	fs.StringVar(&g.configPath, "config", "", "path to the config file (default ~/.config/britzelator/config.json)")
	fs.StringVar(&g.envFile, "env-file", ".env", "path to a .env file holding credentials (empty to skip)")
	fs.DurationVar(&g.timeout, "timeout", 15*time.Second, "HTTP timeout")
	fs.BoolVar(&g.jsonOut, "json", false, "print raw JSON responses")
	fs.BoolVar(&g.verbose, "v", false, "log requests to stderr")
	fs.BoolVar(&g.quiet, "q", false, "suppress progress output (errors still print)")
	fs.BoolVar(&g.quiet, "quiet", false, "suppress progress output (errors still print)")
	fs.Usage = func() { printUsage() }
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	g.fs = fs
	g.errOut = os.Stderr
	dotenv := loadDotEnv(g.envFile)

	if g.configPath == "" {
		g.configPath = defaultConfigPath()
	}
	cfg, err := loadFileConfig(g.configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", g.configPath, err)
	}
	g.file = cfg

	// Resolution order per setting: flag, environment variable, .env file,
	// then the config file.
	env := func(names ...string) string {
		for _, n := range names {
			if v := os.Getenv(n); v != "" {
				return v
			}
		}
		for _, n := range names {
			if v := dotenv[n]; v != "" {
				return v
			}
		}
		return ""
	}

	if g.token == "" {
		g.token = env("OPENSHOCK_TOKEN", "OPENSHOCK_API_TOKEN")
	}
	if g.token == "" {
		g.token = cfg.Token
	}
	if g.session == "" {
		g.session = env("OPENSHOCK_SESSION")
	}
	if g.session == "" {
		g.session = cfg.Session
	}
	if g.server == "" {
		g.server = env("OPENSHOCK_SERVER")
	}
	if g.server == "" {
		g.server = cfg.Server
	}
	if g.server == "" {
		g.server = defaultServer
	}
	if g.userAgent == "" {
		g.userAgent = env("OPENSHOCK_USER_AGENT")
	}
	if g.userAgent == "" {
		g.userAgent = cfg.UserAgent
	}
	if g.userAgent == "" {
		g.userAgent = defaultUserAgent
	}
	return g, fs.Args(), nil
}

func (g *globals) client() (*Client, error) {
	if g.token == "" && g.session == "" {
		return nil, errors.New("no credentials: pass --token, set OPENSHOCK_TOKEN (or OPENSHOCK_API_TOKEN), put one in .env, or run `britzelator config set token <token>`")
	}
	return &Client{
		Base:      g.server,
		Token:     g.token,
		Session:   g.session,
		UserAgent: g.userAgent,
		Timeout:   g.timeout,
		Verbose:   g.verbose,
		HTTP:      &http.Client{Timeout: g.timeout},
	}, nil
}

// clientNoAuth is for endpoints that need neither a token nor a session.
func (g *globals) clientNoAuth() *Client {
	return &Client{
		Base:      g.server,
		UserAgent: g.userAgent,
		Timeout:   g.timeout,
		Verbose:   g.verbose,
		HTTP:      &http.Client{Timeout: g.timeout},
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `britzelator — CLI client for the OpenShock API (v2 preferred)

Usage:
  britzelator [global flags] <command> [flags]

Global flags:
  --token <t>        API token (or $OPENSHOCK_TOKEN / $OPENSHOCK_API_TOKEN)
  --session <c>      user session cookie (or $OPENSHOCK_SESSION)
  --server <url>     API base URL (default https://api.openshock.app)
  --env-file <path>  .env file to read credentials from (default .env)
  --user-agent <ua>  User-Agent header (required by the API)
  --json             print raw JSON responses
  --timeout <dur>    HTTP timeout (default 15s)
  -q, --quiet        suppress progress output (errors still print)
  -v                 log requests to stderr
  --config <path>    config file path

Commands:
  control <shocker>...   send one control message (shock/vibrate/sound/stop)
  random <shocker>...    randomized control loop over delay/intensity/duration ranges
  stop <shocker>...      stop control on shockers
  shockers list|info|pause|logs
  hubs list|create|shockers|lcg|pair|delete
  tokens self|list|create|update|pause|revoke
  whoami                 show the current token and account
  login                  log in to obtain a session cookie (token management)
  raw <METHOD> <path>    call any endpoint directly
  config set|show|path
  version

Shockers can be referenced by UUID, by exact name, or by a unique name
substring; qualified names of the form "hub/shocker" or "owner/shocker"
disambiguate duplicates.

Examples:
  britzelator shockers list
  britzelator control LivingRoom --mode shock --intensity 40 --duration 1000
  britzelator control Collar1 Collar2 --mode vibrate --intensity 20 --duration 3000
  britzelator random Collar --delay-min 10 --delay-max 60 \
      --intensity-min 5 --intensity-max 30 --duration-min 500 --duration-max 2000
  britzelator random Collar --delay-min 2 --delay-max 2 --count 1 --dry-run
  britzelator hubs list
  britzelator raw GET /1/shockers/own
`)
}

// newFlagSet builds a subcommand flag set that also accepts the global flags,
// so `britzelator tokens list --json` works as well as `britzelator --json tokens
// list`. Registering the same flag.Value keeps both sets pointed at the same
// storage.
func (g *globals) newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	g.fs.VisitAll(func(f *flag.Flag) {
		if fs.Lookup(f.Name) == nil {
			fs.Var(f.Value, f.Name, f.Usage)
		}
	})
	return fs
}

// flagWasSet reports whether the named flag appeared on the command line.
// The stdlib flag package only exposes this through Visit.
func flagWasSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// parseOrHelp accepts flags before, between, or after positional arguments.
// The stdlib flag package stops at the first non-flag, so flags are separated
// out first and re-parsed ahead of the positionals.
func parseOrHelp(fs *flag.FlagSet, args []string) error {
	flags, positional, err := splitFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return usageError{flag.ErrHelp}
		}
		return err
	}
	if err := fs.Parse(append(flags, positional...)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return usageError{flag.ErrHelp}
		}
		return usageError{err}
	}
	return nil
}

func splitFlags(fs *flag.FlagSet, args []string) ([]string, []string, error) {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return flags, append(positional, args[i+1:]...), nil
		case a == "-" || a == "":
			positional = append(positional, a)
		case a[0] != '-':
			positional = append(positional, a)
		default:
			name := strings.TrimLeft(a, "-")
			inline := ""
			if eq := strings.Index(name, "="); eq >= 0 {
				inline, name = name[eq+1:], name[:eq]
			}
			f := fs.Lookup(name)
			if f == nil {
				return nil, nil, usagef("unknown flag %q for `%s`", a, fs.Name())
			}
			flags = append(flags, a)
			if inline != "" {
				continue
			}
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
				continue
			}
			if i+1 >= len(args) {
				return nil, nil, usagef("flag %s needs a value", a)
			}
			i++
			flags = append(flags, args[i])
		}
	}
	return flags, positional, nil
}

func usagef(format string, a ...any) error {
	return usageError{fmt.Errorf(format, a...)}
}

// requireArgs enforces a minimum positional argument count.
func requireArgs(args []string, min int, usage string) error {
	if len(args) < min {
		return usagef("usage: %s", usage)
	}
	return nil
}

func maskToken(t string) string {
	if t == "" {
		return "(not set)"
	}
	if len(t) <= 8 {
		return strings.Repeat("*", len(t))
	}
	return t[:4] + "…" + t[len(t)-4:]
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		var ue usageError
		if errors.As(err, &ue) {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	g, rest, err := parseGlobals(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return flag.ErrHelp
		}
		return usageError{err}
	}
	if len(rest) == 0 {
		printUsage()
		return flag.ErrHelp
	}
	cmd, cmdArgs := rest[0], rest[1:]
	switch cmd {
	case "help", "-h", "--help":
		printUsage()
		return nil
	case "version", "--version":
		fmt.Println("britzelator", version)
		return nil
	case "control":
		return cmdControl(g, cmdArgs)
	case "random":
		return cmdRandom(g, cmdArgs)
	case "stop":
		return cmdStop(g, cmdArgs)
	case "shockers":
		return cmdShockers(g, cmdArgs)
	case "hubs":
		return cmdHubs(g, cmdArgs)
	case "tokens":
		return cmdTokens(g, cmdArgs)
	case "whoami":
		return cmdWhoami(g, cmdArgs)
	case "login":
		return cmdLogin(g, cmdArgs)
	case "raw":
		return cmdRaw(g, cmdArgs)
	case "config":
		return cmdConfig(g, cmdArgs)
	default:
		return usagef("unknown command %q (run `britzelator help`)", cmd)
	}
}
