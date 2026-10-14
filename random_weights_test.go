// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"bytes"
	"flag"
	"reflect"
	"strings"
	"testing"
)

func TestDeviceWeightsFlag(t *testing.T) {
	var values []deviceWeights
	f := deviceWeightsFlag{values: &values}
	for _, raw := range []string{"Collar", "=20:80", "Collar="} {
		if err := f.Set(raw); err == nil {
			t.Fatalf("expected rejection of %q", raw)
		}
	}
	if err := f.Set(" Home Hub/Collar = 20:80 "); err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].target != "Home Hub/Collar" || values[0].weights != "20:80" {
		t.Fatalf("unexpected values: %v", values)
	}
}

func TestResolveDeviceWeights(t *testing.T) {
	index := testIndex()
	actions := []randomAction{{"Vibrate", 20}, {"Shock", 80}}
	values := []deviceWeights{{target: "Liv", actions: actions}}
	ranges := make(map[string]deviceRandomRanges)
	if err := resolveDeviceWeights(index, values, ranges); err != nil {
		t.Fatal(err)
	}
	opts := randomOptions{mode: "Shock", actions: []randomAction{{"Shock", 1}}, deviceRanges: ranges}
	targetOpts := opts.forTarget(index[0])
	if !reflect.DeepEqual(targetOpts.actions, actions) || targetOpts.mode != "Vibrate:20/Shock:80" || targetOpts.deviceRanges != nil {
		t.Fatalf("unexpected target options: %+v", targetOpts)
	}
	if !reflect.DeepEqual(opts.forTarget(index[1]).actions, opts.actions) {
		t.Fatal("unoverridden device must retain global action weights")
	}
	for _, target := range []string{"LivingRoom", strings.ToUpper(index[0].ID), "Collar", "Missing", "abcdabcd-1111-4a2b-8c3d-00000000000f"} {
		if err := resolveDeviceWeights(index, []deviceWeights{{target: target, actions: actions}}, ranges); err == nil {
			t.Fatalf("expected duplicate/selection rejection for %q", target)
		}
	}
}

func TestInvalidDeviceWeightsBeforeRequests(t *testing.T) {
	for _, weights := range []string{"0:0", "20", "-20:80", "20:80:10"} {
		g := &globals{fs: flag.NewFlagSet("test", flag.ContinueOnError)}
		err := cmdRandom(g, []string{"Collar", "--mode", "vibrate/shock", "--device-weights", "Collar=" + weights})
		if _, ok := err.(usageError); !ok {
			t.Fatalf("expected usage error before credentials/lookup: %v", err)
		}
	}
}

func TestDeviceWeightsDryRun(t *testing.T) {
	var output bytes.Buffer
	g := &globals{errOut: &output, fs: flag.NewFlagSet("test", flag.ContinueOnError)}
	index := testIndex()
	if err := cmdRandom(g, []string{index[0].ID, index[1].ID, "--mode", "vibrate/shock",
		"--device-weights", index[0].ID + "=100:0", "--device-weights", index[1].ID + "=0:100",
		"--dry-run", "--count", "1", "--delay-min", "0", "--delay-max", "0"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), index[0].ID+": round 1: Vibrate") || !strings.Contains(output.String(), index[1].ID+": round 1: Shock") {
		t.Fatalf("unexpected output: %s", &output)
	}
}
