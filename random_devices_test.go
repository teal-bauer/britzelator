// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDeviceRangeFlag(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		duration bool
		want     randomRange
		invalid  bool
	}{
		{"Collar=5..15", false, randomRange{5, 15}, false},
		{" Home Hub/Collar = 0..100 ", false, randomRange{0, 100}, false},
		{"Collar=8", false, randomRange{8, 8}, false},
		{"Collar=500ms..1.5s", true, randomRange{500, 1500}, false},
		{"Collar=300..65535", true, randomRange{300, 65535}, false},
		{"Collar=1s", true, randomRange{1000, 1000}, false},
		{"Collar=1m..65s", true, randomRange{60000, 65000}, false},
		{"Collar", false, randomRange{}, true},
		{"=5..10", false, randomRange{}, true},
		{"Collar=", false, randomRange{}, true},
		{"Collar=1..", false, randomRange{}, true},
		{"Collar=1..2..3", false, randomRange{}, true},
		{"Collar=10..5", false, randomRange{}, true},
		{"Collar=-1..5", false, randomRange{}, true},
		{"Collar=5..101", false, randomRange{}, true},
		{"Collar=1.5", false, randomRange{}, true},
		{"Collar=299..500", true, randomRange{}, true},
		{"Collar=1s..2m", true, randomRange{}, true},
		{"Collar=2s..1s", true, randomRange{}, true},
		{"Collar=oops", true, randomRange{}, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var values []deviceRange
			err := (deviceRangeFlag{values: &values, duration: tc.duration}).Set(tc.raw)
			if tc.invalid {
				if err == nil || len(values) != 0 {
					t.Fatalf("expected rejection, got %v, %v", values, err)
				}
				return
			}
			if err != nil || len(values) != 1 || values[0].randomRange != tc.want {
				t.Fatalf("got %v, %v; want %v", values, err, tc.want)
			}
		})
	}
}

func TestResolveDeviceRanges(t *testing.T) {
	index := testIndex()
	intensity := deviceRange{"livingroom", randomRange{2, 4}}
	duration := deviceRange{strings.ToUpper(index[0].ID), randomRange{300, 900}}
	got, err := resolveDeviceRanges(index, []deviceRange{intensity}, []deviceRange{duration})
	if err != nil {
		t.Fatal(err)
	}
	ranges := got[index[0].ID]
	if len(got) != 1 || *ranges.intensity != intensity.randomRange || *ranges.duration != duration.randomRange {
		t.Fatalf("got %+v", got)
	}
	for _, tc := range []struct {
		name                   string
		intensities, durations []deviceRange
	}{
		{"ambiguous", []deviceRange{{"Collar", randomRange{1, 2}}}, nil},
		{"missing name", []deviceRange{{"Nope", randomRange{1, 2}}}, nil},
		{"unselected UUID", []deviceRange{{"abcdabcd-1111-4a2b-8c3d-00000000000f", randomRange{1, 2}}}, nil},
		{"duplicate intensity alias", []deviceRange{intensity, {"Liv", randomRange{1, 2}}}, nil},
		{"duplicate duration alias", nil, []deviceRange{duration, {"LivingRoom", randomRange{300, 400}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveDeviceRanges(index, tc.intensities, tc.durations); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestRandomDeviceRangesCLI(t *testing.T) {
	for _, independent := range []bool{false, true} {
		t.Run(map[bool]string{false: "synchronized", true: "independent"}[independent], func(t *testing.T) {
			var mu sync.Mutex
			var requests []ControlRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/2/shockers/control" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				var req ControlRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				mu.Lock()
				requests = append(requests, req)
				mu.Unlock()
			}))
			defer srv.Close()
			g := &globals{token: "test", server: srv.URL, quiet: true, fs: flag.NewFlagSet("test", flag.ContinueOnError)}
			index := testIndex()
			args := []string{index[0].ID, index[1].ID, index[2].ID,
				"--delay-min", "0", "--delay-max", "0", "--count", "20", "--seed", "42",
				"--intensity-min", "7", "--intensity-max", "7", "--duration-min", "400", "--duration-max", "400",
				"--device-intensity", index[0].ID + "=1..3", "--device-intensity", index[1].ID + "=12",
				"--device-duration", index[0].ID + "=500ms..800ms", "--exclusive", "--name", "test"}
			if independent {
				args = append(args, "--independent")
			}
			if err := cmdRandom(g, args); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			wantRequests, wantControls := 20, 3
			if independent {
				wantRequests, wantControls = 60, 1
			}
			if len(requests) != wantRequests {
				t.Fatalf("got %d requests; want %d", len(requests), wantRequests)
			}
			counts := make(map[string]int)
			for _, req := range requests {
				if len(req.Shocks) != wantControls || req.CustomName == nil || *req.CustomName != "test" {
					t.Fatalf("unexpected payload: %+v", req)
				}
				for _, control := range req.Shocks {
					counts[control.ID]++
					if control.Type != "Shock" || control.Exclusive == nil || !*control.Exclusive {
						t.Fatalf("unexpected control: %+v", control)
					}
					switch control.ID {
					case index[0].ID:
						if control.Intensity < 1 || control.Intensity > 3 || control.Duration < 500 || control.Duration > 800 {
							t.Fatalf("override outside bounds: %+v", control)
						}
					case index[1].ID:
						if control.Intensity != 12 || control.Duration != 400 {
							t.Fatalf("partial override: %+v", control)
						}
					case index[2].ID:
						if control.Intensity != 7 || control.Duration != 400 {
							t.Fatalf("global defaults: %+v", control)
						}
					default:
						t.Fatalf("unexpected target: %+v", control)
					}
				}
			}
			for _, target := range index {
				if counts[target.ID] != 20 {
					t.Fatalf("counts: %v", counts)
				}
			}
		})
	}
}

func TestRandomDeviceRangesDryRun(t *testing.T) {
	var output bytes.Buffer
	g := &globals{errOut: &output, fs: flag.NewFlagSet("test", flag.ContinueOnError)}
	id := testIndex()[0].ID
	if err := cmdRandom(g, []string{id, "--device-intensity", id + "=2", "--device-duration", id + "=300ms", "--dry-run", "--count", "1", "--delay-min", "0", "--delay-max", "0"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Shock i=2 d=300ms") || !strings.Contains(output.String(), "dry-run, not sending") {
		t.Fatalf("unexpected output: %s", &output)
	}
}

func TestRandomDeviceRangesReproducible(t *testing.T) {
	targets := testIndex()
	opts := randomOptions{mode: "Vibrate", actions: []randomAction{{"Vibrate", 20}, {"Shock", 80}}, intensityMin: 1, intensityMax: 10, durationMin: 300, durationMax: 500,
		deviceRanges: map[string]deviceRandomRanges{targets[0].ID: {intensity: &randomRange{20, 30}, duration: &randomRange{600, 900}}}}
	run := func() []ControlRequest {
		var requests []ControlRequest
		c := &Client{Base: "http://test.invalid", HTTP: &http.Client{Transport: randomTransport(func(r *http.Request) (*http.Response, error) {
			var req ControlRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			requests = append(requests, req)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})}}
		if _, err := runRandomLoopWithClock(context.Background(), c, targets, "test", opts, 10, 10, newRand(42, 0), &loopLogger{g: &globals{quiet: true}}, time.Now, func(context.Context, time.Duration) error { return nil }); err != nil {
			t.Fatal(err)
		}
		return requests
	}
	first, second := run(), run()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("fixed seed must reproduce per-device draws")
	}
	for _, req := range first {
		if req.Shocks[0].Type != req.Shocks[1].Type || req.Shocks[1].Type != req.Shocks[2].Type {
			t.Fatalf("devices without action overrides must share actions: %+v", req)
		}
		if req.Shocks[1].Intensity != req.Shocks[2].Intensity || req.Shocks[1].Duration != req.Shocks[2].Duration {
			t.Fatalf("unoverridden devices must share draws: %+v", req)
		}
	}
}
