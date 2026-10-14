// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"encoding/json"
	"flag"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func TestParseRandomActions(t *testing.T) {
	for _, tc := range []struct {
		mode, weights string
		want          []randomAction
	}{
		{"shock", "", []randomAction{{"Shock", 1}}},
		{"vib/zap", "20:80", []randomAction{{"Vibrate", 20}, {"Shock", 80}}},
		{"tone/vibrate/shock", "1:2:7", []randomAction{{"Sound", 1}, {"Vibrate", 2}, {"Shock", 7}}},
		{"vibrate/shock", "0:80", []randomAction{{"Vibrate", 0}, {"Shock", 80}}},
		{"vibrate/shock", "", []randomAction{{"Vibrate", 1}, {"Shock", 1}}},
		{" vibrate / shock ", " 20 : 80 ", []randomAction{{"Vibrate", 20}, {"Shock", 80}}},
	} {
		t.Run(tc.mode+tc.weights, func(t *testing.T) {
			got, err := parseRandomActions(tc.mode, tc.weights)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct{ mode, weights string }{
		{"shock", "20:80"}, {"vibrate/shock", "20"}, {"vibrate/shock", "20:80:5"},
		{"vibrate/shock", "-20:80"}, {"vibrate/shock", "20.5:80"},
		{"vibrate/shock", "20:"}, {"vibrate/shock", "0:0"},
		{"shock", "0"}, {"shock/stop", "80:20"}, {"shock/stop", "100:0"},
		{"shock/zap", "20:80"}, {"shock/", ""}, {"/shock", ""}, {"sparkle", ""},
		{"vibrate/shock", strconv.Itoa(int(^uint(0)>>1)) + ":1"},
	} {
		t.Run("invalid "+tc.mode+tc.weights, func(t *testing.T) {
			if _, err := parseRandomActions(tc.mode, tc.weights); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestDrawRandomActions(t *testing.T) {
	actions, err := parseRandomActions("vibrate/shock/tone", "20:80:0")
	if err != nil {
		t.Fatal(err)
	}
	opts := randomOptions{actions: actions}
	first, second := newRand(42, 0), newRand(42, 0)
	counts := make(map[string]int)
	for i := 0; i < 100000; i++ {
		a, b := opts.drawMode(first), opts.drawMode(second)
		if a != b {
			t.Fatal("fixed seed must reproduce action draws")
		}
		counts[a]++
	}
	if counts["Sound"] != 0 || math.Abs(float64(counts["Vibrate"])/100000-0.2) > 0.005 {
		t.Fatalf("unexpected distribution: %v", counts)
	}
}

func TestSingleRandomActionPreservesDraws(t *testing.T) {
	opts := randomOptions{actions: []randomAction{{"Shock", 80}}}
	first, second := newRand(42, 0), newRand(42, 0)
	if got := opts.drawMode(first); got != "Shock" || first.Int63() != second.Int63() {
		t.Fatalf("single action must not consume RNG: %s", got)
	}
}

func TestWeightedRandomActionsCLI(t *testing.T) {
	for _, independent := range []bool{false, true} {
		t.Run(strconv.FormatBool(independent), func(t *testing.T) {
			var mu sync.Mutex
			counts := make(map[string]map[string]int)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/2/shockers/control" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				var req ControlRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				want := 2
				if independent {
					want = 1
				}
				if len(req.Shocks) != want {
					t.Errorf("got %d controls; want %d", len(req.Shocks), want)
				}
				for _, control := range req.Shocks {
					if control.Type != "Vibrate" && control.Type != "Shock" {
						t.Errorf("unexpected action: %+v", control)
					}
					if control.Intensity != 4 || control.Duration != 300 {
						t.Errorf("unexpected device ranges: %+v", control)
					}
					if counts[control.ID] == nil {
						counts[control.ID] = make(map[string]int)
					}
					counts[control.ID][control.Type]++
				}
			}))
			defer srv.Close()
			g := &globals{token: "test", server: srv.URL, quiet: true, fs: flag.NewFlagSet("test", flag.ContinueOnError)}
			index := testIndex()
			args := []string{index[0].ID, index[1].ID, "--mode", "vibrate/shock", "--weights", "20:80",
				"--delay-min", "0", "--delay-max", "0", "--count", "50", "--seed", "42",
				"--intensity-min", "4", "--intensity-max", "4", "--duration-min", "300", "--duration-max", "300",
				"--device-intensity", index[0].ID + "=4", "--device-weights", index[0].ID + "=100:0"}
			if independent {
				args = append(args, "--independent")
			}
			if err := cmdRandom(g, args); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if counts[index[0].ID]["Vibrate"] != 50 || counts[index[0].ID]["Shock"] != 0 {
				t.Fatalf("per-device action weights not applied: %v", counts)
			}
			if counts[index[1].ID]["Vibrate"] == 0 || counts[index[1].ID]["Shock"] == 0 || counts[index[1].ID]["Vibrate"]+counts[index[1].ID]["Shock"] != 50 {
				t.Fatalf("global action weights not applied: %v", counts)
			}
		})
	}
}

func TestInvalidRandomActionsBeforeRequests(t *testing.T) {
	g := &globals{fs: flag.NewFlagSet("test", flag.ContinueOnError)}
	err := cmdRandom(g, []string{"Collar", "--mode", "vibrate/shock", "--weights", "0:0"})
	if _, ok := err.(usageError); !ok {
		t.Fatalf("expected usage error before credentials/lookup: %v", err)
	}
}
