// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type randomTransport func(*http.Request) (*http.Response, error)

func (f randomTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRandomLoopTiming(t *testing.T) {
	for _, tc := range []struct {
		name      string
		latency   []time.Duration
		oversleep time.Duration
		want      []time.Duration
	}{
		{"response latency", []time.Duration{400 * time.Millisecond, 700 * time.Millisecond, 0}, 0, []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}},
		{"overrun recovery", []time.Duration{2500 * time.Millisecond, 100 * time.Millisecond, 0}, 0, []time.Duration{time.Second, 3500 * time.Millisecond, 4500 * time.Millisecond}},
		{"late wakeups", []time.Duration{0, 0, 0}, 200 * time.Millisecond, []time.Duration{1200 * time.Millisecond, 2400 * time.Millisecond, 3600 * time.Millisecond}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Unix(0, 0)
			now := start
			var starts []time.Duration
			c := &Client{Base: "http://test.invalid", HTTP: &http.Client{Transport: randomTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.Path != "/2/shockers/control" {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
				}
				starts = append(starts, now.Sub(start))
				now = now.Add(tc.latency[len(starts)-1])
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			})}}
			opts := randomOptions{mode: "Shock", delayMin: 1, delayMax: 1, intensityMin: 3, intensityMax: 3, durationMin: 300, durationMax: 300}
			sent, err := runRandomLoopWithClock(context.Background(), c, []Shocker{{ID: "test"}}, "test", opts, 3, 3, newRand(1, 0), &loopLogger{g: &globals{quiet: true}}, func() time.Time { return now }, func(ctx context.Context, d time.Duration) error {
				if d < 0 {
					t.Fatalf("negative wait: %s", d)
				}
				now = now.Add(d + tc.oversleep)
				return nil
			})
			if err != nil || sent != 3 {
				t.Fatalf("sent=%d err=%v", sent, err)
			}
			if !reflect.DeepEqual(starts, tc.want) {
				t.Fatalf("starts=%v want=%v", starts, tc.want)
			}
		})
	}
}

func TestRandomLoopCancellationBeforeSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := randomOptions{mode: "Shock"}
	sent, err := runRandomLoopWithClock(ctx, nil, nil, "test", opts, 1, 1, newRand(1, 0), &loopLogger{g: &globals{quiet: true}}, time.Now, func(context.Context, time.Duration) error {
		cancel()
		return nil
	})
	if err != nil || sent != 0 {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
}

func TestSleepCtxCancelledZeroDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 100; i++ {
		if err := sleepCtx(ctx, 0); err != context.Canceled {
			t.Fatalf("got %v", err)
		}
	}
}

func TestRandomLoopDryRunTiming(t *testing.T) {
	now := time.Unix(0, 0)
	var waits []time.Duration
	opts := randomOptions{dryRun: true, delayMin: 1, delayMax: 1}
	sent, err := runRandomLoopWithClock(context.Background(), nil, nil, "test", opts, 3, 3, newRand(1, 0), &loopLogger{g: &globals{quiet: true}}, func() time.Time { return now }, func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		now = now.Add(d)
		return nil
	})
	if err != nil || sent != 0 {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if !reflect.DeepEqual(waits, []time.Duration{time.Second, time.Second, time.Second}) {
		t.Fatalf("waits=%v", waits)
	}
}

func TestRandomLoopRetriesTransientSendErrors(t *testing.T) {
	calls := 0
	c := &Client{Base: "http://test.invalid", HTTP: &http.Client{Transport: randomTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 2 {
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}}
	opts := randomOptions{mode: "Shock", delayMin: 0, delayMax: 0, intensityMin: 3, intensityMax: 3, durationMin: 300, durationMax: 300}
	now := time.Unix(0, 0)
	sent, err := runRandomLoopWithClock(context.Background(), c, []Shocker{{ID: "test"}}, "test", opts, 3, 3, newRand(1, 0), &loopLogger{g: &globals{quiet: true}}, func() time.Time { return now }, func(ctx context.Context, d time.Duration) error {
		now = now.Add(d)
		return nil
	})
	if err != nil {
		t.Fatalf("loop aborted on transient error: %v", err)
	}
	if sent != 3 {
		t.Fatalf("sent=%d want 3", sent)
	}
	if calls != 4 {
		t.Fatalf("calls=%d want 4", calls)
	}
}

func TestControlTimeout(t *testing.T) {
	c := &Client{Timeout: 15 * time.Second}
	for _, tc := range []struct {
		name  string
		delay float64
		want  time.Duration
	}{
		{"sub-second delay floors at 2s", 0.2, 2 * time.Second},
		{"delay above floor is used", 7, 7 * time.Second},
		{"global timeout caps the deadline", 30, 15 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := controlTimeout(c, tc.delay); got != tc.want {
				t.Fatalf("controlTimeout=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestRandomLoopAbortsOnRejectedSend(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"problem body", `{"title":"Invalid control","status":400}`},
		{"plain body", "rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{Base: "http://test.invalid", HTTP: &http.Client{Transport: randomTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}}
			opts := randomOptions{mode: "Shock", delayMin: 0, delayMax: 0, intensityMin: 3, intensityMax: 3, durationMin: 300, durationMax: 300}
			now := time.Unix(0, 0)
			sent, err := runRandomLoopWithClock(context.Background(), c, []Shocker{{ID: "test"}}, "test", opts, 3, 3, newRand(1, 0), &loopLogger{g: &globals{quiet: true}}, func() time.Time { return now }, func(ctx context.Context, d time.Duration) error {
				now = now.Add(d)
				return nil
			})
			if err == nil {
				t.Fatal("expected a rejected request to abort the loop")
			}
			if sent != 0 {
				t.Fatalf("sent=%d want 0", sent)
			}
		})
	}
}
