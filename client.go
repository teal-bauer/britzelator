// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Problem is the RFC 7807 body OpenShock returns on failures.
type Problem struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	Detail  string `json:"detail"`
	Message string `json:"message"`
	TraceID string `json:"traceId"`
}

func (p *Problem) Error() string {
	msg := p.Title
	if p.Detail != "" {
		msg = p.Detail
	} else if p.Message != "" {
		msg = p.Message
	}
	if p.Type != "" {
		msg = p.Type + ": " + msg
	}
	if p.TraceID != "" {
		msg += " (trace " + p.TraceID + ")"
	}
	return fmt.Sprintf("http %d: %s", p.Status, msg)
}

type Result struct {
	Status  int
	Body    []byte
	Cookies map[string]string
}

type Client struct {
	Base      string
	Token     string
	Session   string
	UserAgent string
	Timeout   time.Duration
	Verbose   bool
	HTTP      *http.Client
	Headers   map[string]string
}

func (c *Client) do(method, path string, body io.Reader, contentType string) (Result, error) {
	url := strings.TrimRight(c.Base, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	// The OpenAPI document names the API token scheme OpenShockToken, with the
	// token carried in a header of that name.
	req.Header.Set("OpenShockToken", c.Token)
	if c.Session != "" {
		req.Header.Set("Cookie", "openShockSession="+c.Session)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "→ %s %s\n", method, path)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	res := Result{Status: resp.StatusCode, Body: raw, Cookies: map[string]string{}}
	for _, ck := range resp.Cookies() {
		res.Cookies[ck.Name] = ck.Value
	}
	if resp.StatusCode >= 400 {
		var p Problem
		if json.Unmarshal(raw, &p) == nil && (p.Title != "" || p.Type != "") {
			p.Status = resp.StatusCode
			return res, &p
		}
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 400 {
			snippet = snippet[:400] + "…"
		}
		return res, fmt.Errorf("http %d: %s", resp.StatusCode, snippet)
	}
	return res, nil
}

// doJSON performs a request, encodes payload as JSON when non-nil, and decodes
// the response into out when out is non-nil.
func (c *Client) doJSON(method, path string, payload any, out any) (Result, error) {
	var body io.Reader
	contentType := ""
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return Result{}, err
		}
		body = bytes.NewReader(raw)
		contentType = "application/json"
	}
	res, err := c.do(method, path, body, contentType)
	if err != nil {
		return res, err
	}
	if out != nil && len(res.Body) > 0 {
		if err := json.Unmarshal(res.Body, out); err != nil {
			return res, fmt.Errorf("decoding %s %s response: %w", method, path, err)
		}
	}
	return res, nil
}

func (c *Client) Get(path string, out any) error {
	_, err := c.doJSON(http.MethodGet, path, nil, out)
	return err
}
func (c *Client) Post(path string, payload, out any) error {
	_, err := c.doJSON(http.MethodPost, path, payload, out)
	return err
}

func (c *Client) Patch(path string, payload, out any) error {
	_, err := c.doJSON(http.MethodPatch, path, payload, out)
	return err
}

func (c *Client) Delete(path string, out any) error {
	_, err := c.doJSON(http.MethodDelete, path, nil, out)
	return err
}

type legacyEnvelope struct {
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// getData unwraps the `{message, data}` envelope used by the v1 endpoints.
func getData(res Result) (json.RawMessage, error) {
	var env legacyEnvelope
	if err := json.Unmarshal(res.Body, &env); err != nil {
		return nil, err
	}
	if env.Data == nil {
		return nil, errors.New("response has no data field")
	}
	return env.Data, nil
}

func prettyJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}
