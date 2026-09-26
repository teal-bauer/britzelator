// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func cmdWhoami(g *globals, args []string) error {
	fs := g.newFlagSet("whoami")
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

	userRes, userErr := c.doJSON(http.MethodGet, "/1/users/self", nil, nil)
	var user struct {
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		Email string   `json:"email"`
		Rank  string   `json:"rank"`
		Roles []string `json:"roles"`
	}
	userOK := false
	if userErr == nil {
		if raw, err := getData(userRes); err == nil && json.Unmarshal(raw, &user) == nil {
			userOK = true
		}
	}

	if g.jsonOut {
		out := map[string]any{"token": tok, "server": g.server, "tokenMasked": maskToken(g.token)}
		if userOK {
			out["user"] = user
		}
		return printJSON(out)
	}

	fmt.Printf("server:      %s\n", g.server)
	fmt.Printf("token:       %s (%s)\n", tok.Name, tok.ID)
	fmt.Printf("secret:      %s\n", maskToken(g.token))
	fmt.Printf("permissions: %s\n", strings.Join(tok.Permissions, ", "))
	fmt.Printf("control:     paused=%v %s\n", tok.ShockerControl.Paused, limitSummary(tok.ShockerControl))
	fmt.Printf("valid until: %s\n", derefOr(tok.ValidUntil, "never"))
	if userOK {
		fmt.Printf("account:     %s <%s> rank %s\n", user.Name, user.Email, user.Rank)
	}
	return nil
}

// cmdLogin posts credentials to /2/account/login and stores the session
// cookie, which the token management endpoints require. The API expects a
// Cloudflare Turnstile response, so --turnstile must be supplied when the
// server enforces it.
func cmdLogin(g *globals, args []string) error {
	fs := g.newFlagSet("login")
	user := fs.String("user", "", "username or email")
	password := fs.String("password", "", "password (prefer the OPENSHOCK_PASSWORD env var)")
	turnstile := fs.String("turnstile", "", "Cloudflare Turnstile response token, when required")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator login --user <name-or-email> --password <pw> [--turnstile <token>]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if *password == "" {
		*password = os.Getenv("OPENSHOCK_PASSWORD")
	}
	if *user == "" || *password == "" {
		return usagef("--user and --password are required (or set OPENSHOCK_PASSWORD)")
	}

	c := g.clientNoAuth()
	res, err := c.doJSON(http.MethodPost, "/2/account/login", map[string]string{
		"usernameOrEmail":   *user,
		"password":          *password,
		"turnstileResponse": *turnstile,
	}, nil)
	if err != nil {
		return err
	}
	session := res.Cookies["openShockSession"]
	if session == "" {
		return fmt.Errorf("login succeeded but no openShockSession cookie was returned")
	}
	g.file.Session = session
	g.file.Server = g.server
	if err := saveFileConfig(g.configPath, g.file); err != nil {
		return fmt.Errorf("saving session: %w", err)
	}
	g.printfErr("logged in as %s, session stored in %s\n", *user, g.configPath)
	return nil
}

// parseHeaders turns a comma-separated Name:Value list into a header map.
func parseHeaders(spec string) (map[string]string, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(spec, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, usagef("invalid header %q (want Name:Value)", pair)
		}
		out[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return out, nil
}

// cmdRaw calls an arbitrary endpoint, using the documented request shape.
func cmdRaw(g *globals, args []string) error {
	fs := g.newFlagSet("raw")
	data := fs.String("data", "", "request body as JSON")
	file := fs.String("file", "", "read the request body from a file ('-' for stdin)")
	header := fs.String("header", "", "extra header as Name:Value (repeatable, comma-separated)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator raw <METHOD> <path> [--data JSON | --file PATH] [--header Name:Value]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 2, "britzelator raw <METHOD> <path> [--data JSON]"); err != nil {
		return err
	}
	method := strings.ToUpper(fs.Args()[0])
	path := fs.Args()[1]
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	body := *data
	switch {
	case *file == "-":
		raw, err := os.ReadFile("/dev/stdin")
		if err != nil {
			return err
		}
		body = string(raw)
	case *file != "":
		raw, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	if body != "" && !json.Valid([]byte(body)) {
		return usagef("request body is not valid JSON")
	}

	headerMap, err := parseHeaders(*header)
	if err != nil {
		return err
	}

	c, err := g.client()
	if err != nil {
		return err
	}
	c.Headers = headerMap
	contentType := ""
	if body != "" {
		contentType = "application/json"
	}
	res, err := c.do(method, path, strings.NewReader(body), contentType)
	if err != nil {
		if len(res.Body) > 0 {
			printRawJSON(res.Body)
		}
		return err
	}
	if len(res.Body) == 0 {
		g.printfErr("%d %s %s (empty response)\n", res.Status, method, path)
		return nil
	}
	printRawJSON(res.Body)
	return nil
}

func cmdConfig(g *globals, args []string) error {
	if len(args) == 0 {
		return usagef("usage: britzelator config show|path|set|unset ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "path":
		fmt.Println(g.configPath)
		return nil
	case "show":
		cfg := g.file
		masked := cfg
		masked.Token = maskToken(cfg.Token)
		masked.Session = maskToken(cfg.Session)
		out, _ := json.MarshalIndent(masked, "", "  ")
		fmt.Println(string(out))
		return nil
	case "set":
		if err := requireArgs(rest, 2, "britzelator config set <token|session|server|user-agent> <value>"); err != nil {
			return err
		}
		key, value := rest[0], rest[1]
		switch key {
		case "token":
			g.file.Token = value
		case "session":
			g.file.Session = value
		case "server":
			g.file.Server = value
		case "user-agent", "user_agent":
			g.file.UserAgent = value
		default:
			return usagef("unknown config key %q (want token, session, server, or user-agent)", key)
		}
		if err := saveFileConfig(g.configPath, g.file); err != nil {
			return err
		}
		g.printfErr("saved %s in %s\n", key, g.configPath)
		return nil
	case "unset":
		if err := requireArgs(rest, 1, "britzelator config unset <token|session|server|user-agent>"); err != nil {
			return err
		}
		switch rest[0] {
		case "token":
			g.file.Token = ""
		case "session":
			g.file.Session = ""
		case "server":
			g.file.Server = ""
		case "user-agent", "user_agent":
			g.file.UserAgent = ""
		default:
			return usagef("unknown config key %q", rest[0])
		}
		if err := saveFileConfig(g.configPath, g.file); err != nil {
			return err
		}
		g.printfErr("removed %s from %s\n", rest[0], g.configPath)
		return nil
	default:
		return usagef("unknown config subcommand %q", sub)
	}
}
