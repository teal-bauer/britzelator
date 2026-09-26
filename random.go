// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type randomOptions struct {
	mode         string
	delayMin     float64
	delayMax     float64
	intensityMin int
	intensityMax int
	durationMin  int
	durationMax  int
	count        int
	countMin     *int
	countMax     *int
	seed         int64
	exclusive    bool
	customName   string
	dryRun       bool
	independent  bool
}

// cmdRandom runs a randomized control loop: draw a delay, wait it out, then
// send a control message whose intensity and duration are drawn from the given
// ranges. It repeats until interrupted or until the drawn round count is done.
//
// With --independent every named shocker gets its own loop and its own random
// draws, so they fire on unrelated schedules. Without it, one draw drives all
// the named shockers at once.
func cmdRandom(g *globals, args []string) error {
	fs := g.newFlagSet("random")
	opts := randomOptions{
		mode:         "shock",
		delayMin:     5,
		delayMax:     30,
		intensityMin: 5,
		intensityMax: 30,
		durationMin:  500,
		durationMax:  2000,
	}
	fs.Var(secondsValue{&opts.delayMin}, "delay-min", "minimum wait before a control round, e.g. 5, 5s, 500ms")
	fs.Var(secondsValue{&opts.delayMax}, "delay-max", "maximum wait before a control round")
	fs.IntVar(&opts.intensityMin, "intensity-min", opts.intensityMin, "minimum intensity (0..100)")
	fs.IntVar(&opts.intensityMax, "intensity-max", opts.intensityMax, "maximum intensity (0..100)")
	fs.Var(durationValue{&opts.durationMin}, "duration-min", "minimum duration, e.g. 500, 500ms, 1s")
	fs.Var(durationValue{&opts.durationMax}, "duration-max", "maximum duration, e.g. 2000, 2s")
	fs.StringVar(&opts.mode, "mode", opts.mode, "control mode: shock, vibrate, tone (sound)")
	fs.IntVar(&opts.count, "count", 0, "fixed number of rounds per loop (0 = until interrupted)")
	fs.Var(intFlag(&opts.countMin), "count-min", "minimum rounds per loop, drawn randomly per loop")
	fs.Var(intFlag(&opts.countMax), "count-max", "maximum rounds per loop, drawn randomly per loop")
	fs.Int64Var(&opts.seed, "seed", 0, "random seed (0 = nondeterministic)")
	fs.BoolVar(&opts.exclusive, "exclusive", false, "mark each control as exclusive")
	fs.StringVar(&opts.customName, "name", "", "customName recorded in the control log")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "draw values and print them without sending")
	fs.BoolVar(&opts.independent, "independent", false, "run one independent random loop per shocker instead of one shared draw")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: britzelator random <shocker>... [--independent] --delay-min 5s --delay-max 60s --intensity-min 5 --intensity-max 30 --duration-min 500ms --duration-max 2s [--count-min 3 --count-max 10]")
		fs.PrintDefaults()
	}
	if err := parseOrHelp(fs, args); err != nil {
		return err
	}
	if err := requireArgs(fs.Args(), 1, "britzelator random <shocker>... [flags]"); err != nil {
		return err
	}

	typeName, err := normalizeMode(opts.mode)
	if err != nil {
		return err
	}
	if typeName == "Stop" {
		return usagef("--mode stop makes no sense for random mode")
	}
	if opts.delayMin < 0 || opts.delayMax < opts.delayMin {
		return usagef("delay range invalid: --delay-min=%gs --delay-max=%gs", opts.delayMin, opts.delayMax)
	}
	if opts.intensityMin < minIntensity || opts.intensityMax > maxIntensity || opts.intensityMax < opts.intensityMin {
		return usagef("intensity range invalid: %d..%d (allowed 0..100)", opts.intensityMin, opts.intensityMax)
	}
	if opts.durationMin < minDurationMs || opts.durationMax > maxDurationMs || opts.durationMax < opts.durationMin {
		return usagef("duration range invalid: %d..%dms (allowed %d..%dms)", opts.durationMin, opts.durationMax, minDurationMs, maxDurationMs)
	}
	if opts.count < 0 {
		return usagef("--count must be >= 0")
	}
	if (opts.countMin != nil && *opts.countMin < 0) || (opts.countMax != nil && *opts.countMax < 0) {
		return usagef("--count-min and --count-max must be >= 0")
	}
	roundsMin, roundsMax, err := opts.roundRange()
	if err != nil {
		return err
	}
	opts.mode = typeName

	targets, err := resolveTargets(g, fs.Args())
	if err != nil {
		return err
	}

	var c *Client
	if !opts.dryRun {
		if c, err = g.client(); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := &loopLogger{g: g}
	if opts.independent && len(targets) > 1 {
		return runIndependentLoops(ctx, c, targets, opts, roundsMin, roundsMax, log)
	}
	log.logf("random mode on %s: delay %.3g..%.3gs, intensity %d..%d, duration %d..%dms, rounds %s, mode %s",
		describeTargets(targets), opts.delayMin, opts.delayMax, opts.intensityMin, opts.intensityMax,
		opts.durationMin, opts.durationMax, describeRounds(roundsMin, roundsMax), opts.mode)
	_, err = runRandomLoop(ctx, c, targets, describeTargets(targets), opts, roundsMin, roundsMax, newRand(opts.seed, 0), log)
	return err
}

// roundRange resolves the round-count flags into a range to draw from. A
// --count value fixes it; --count-min/--count-max define a range. Either way a
// bound of 0 means "until interrupted".
func (o randomOptions) roundRange() (int, int, error) {
	if o.countMin == nil && o.countMax == nil {
		return o.count, o.count, nil
	}
	lo, hi := 0, 0
	if o.countMin != nil {
		lo = *o.countMin
	} else {
		lo = *o.countMax
	}
	if o.countMax != nil {
		hi = *o.countMax
	} else {
		hi = *o.countMin
	}
	if hi < lo {
		return 0, 0, usagef("round count range invalid: --count-min=%d --count-max=%d", lo, hi)
	}
	return lo, hi, nil
}

func describeRounds(lo, hi int) string {
	switch {
	case lo == hi && lo == 0:
		return "until interrupted"
	case lo == hi:
		return fmt.Sprintf("%d", lo)
	default:
		return fmt.Sprintf("%d..%d per loop (0 = until interrupted)", lo, hi)
	}
}

// runIndependentLoops gives each shocker its own goroutine, random source, and
// schedule. The first error stops the others; an interrupt ends every loop.
func runIndependentLoops(ctx context.Context, c *Client, targets []Shocker, opts randomOptions, roundsMin, roundsMax int, log *loopLogger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.logf("independent random mode on %d shockers, one loop each", len(targets))

	var (
		wg    sync.WaitGroup
		sent  int64
		errMu sync.Mutex
		err   error
	)
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target Shocker) {
			defer wg.Done()
			n, loopErr := runRandomLoop(ctx, c, []Shocker{target}, target.Label(), opts, roundsMin, roundsMax, newRand(opts.seed, i), log)
			atomic.AddInt64(&sent, int64(n))
			if loopErr != nil {
				errMu.Lock()
				if err == nil {
					err = loopErr
				}
				errMu.Unlock()
				cancel()
			}
		}(i, target)
	}
	wg.Wait()

	errMu.Lock()
	defer errMu.Unlock()
	if err != nil {
		return err
	}
	log.logf("all loops finished: %d control(s) sent across %d shocker(s)", atomic.LoadInt64(&sent), len(targets))
	return nil
}

// runRandomLoop drives one schedule. targets are sent together in a single
// control request, which is how the non-independent mode applies one draw to
// several shockers.
func runRandomLoop(ctx context.Context, c *Client, targets []Shocker, label string, opts randomOptions, roundsMin, roundsMax int, rnd *rand.Rand, log *loopLogger) (int, error) {
	rounds := randInt(rnd, roundsMin, roundsMax)
	if roundsMin != roundsMax {
		if rounds == 0 {
			log.logf("%s: drew rounds=until interrupted from %d..%d", label, roundsMin, roundsMax)
		} else {
			log.logf("%s: drew rounds=%d from %d..%d", label, rounds, roundsMin, roundsMax)
		}
	}

	sent := 0
	for round := 1; rounds == 0 || round <= rounds; round++ {
		delay := opts.delayMin
		if opts.delayMax > opts.delayMin {
			delay = opts.delayMin + rnd.Float64()*(opts.delayMax-opts.delayMin)
		}
		intensity := randInt(rnd, opts.intensityMin, opts.intensityMax)
		duration := randInt(rnd, opts.durationMin, opts.durationMax)

		if opts.count == 1 && roundsMin == roundsMax {
			log.logf("%s: %s i=%d d=%dms in %.1fs", label, opts.mode, intensity, duration, delay)
		} else {
			log.logf("%s: round %d: %s i=%d d=%dms in %.1fs", label, round, opts.mode, intensity, duration, delay)
		}

		// The delay is drawn before the control message and waited out first,
		// so the next control lands a random time from now.
		if err := sleepCtx(ctx, secondsToDuration(delay)); err != nil {
			log.logf("%s: interrupted after %d round(s), %d sent", label, round-1, sent)
			return sent, nil
		}

		if opts.dryRun {
			log.logf("%s: dry-run, not sending", label)
			continue
		}
		payload := buildControlRequest(targets, opts.mode, intensity, duration, opts.exclusive, opts.customName)
		if err := c.Post("/2/shockers/control", payload, nil); err != nil {
			return sent, err
		}
		sent++
		log.logf("%s: sent", label)
	}
	log.logf("%s: finished, %d sent", label, sent)
	return sent, nil
}

// loopLogger serializes progress lines from concurrent loops.
type loopLogger struct {
	mu sync.Mutex
	g  *globals
}

func (l *loopLogger) logf(format string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.g.printfErr("[%s] "+format+"\n", append([]any{time.Now().Format("15:04:05")}, a...)...)
}

// newRand gives every loop its own source; a fixed seed stays reproducible by
// deriving one seed per loop index.
func newRand(seed int64, index int) *rand.Rand {
	if seed != 0 {
		return rand.New(rand.NewSource(seed + int64(index)))
	}
	return rand.New(rand.NewSource(time.Now().UnixNano() + int64(index)*0x9E3779B1))
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// sleepCtx waits for d or for cancellation, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-t.C:
		return nil
	}
}

// randInt draws uniformly from [min, max], inclusive.
func randInt(r *rand.Rand, min, max int) int {
	if max <= min {
		return min
	}
	return min + r.Intn(max-min+1)
}
