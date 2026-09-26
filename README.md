# britzelator

A command-line client for the [OpenShock](https://openshock.app) API, built
around controlling shockers: single control messages, and a randomized mode that
fires controls at drawn intervals with drawn intensity and duration.

[![CI](https://github.com/teal-bauer/britzelator/actions/workflows/ci.yml/badge.svg)](https://github.com/teal-bauer/britzelator/actions/workflows/ci.yml)
[![License: AGPL-3.0-or-later](https://img.shields.io/badge/license-AGPL--3.0--or--later-blue.svg)](LICENSE)

[Download the latest release](https://github.com/teal-bauer/britzelator/releases) — prebuilt binaries for Linux, macOS, and Windows.

## ⚠️ Safety and responsible use

This program drives devices that deliberately cause painful electric shocks.
Treat it the way you would treat any other tool that can hurt someone:

- **Consent first, every time.** Only control a device worn by a person who has
  explicitly agreed to it, and who knows what you are about to run. Never use
  this on a person who has not agreed, cannot meaningfully consent, or is not
  expecting it — including as a prank.
- **Respect limits.** Set conservative intensity and duration, stay inside what
  the wearer agreed to, and stop immediately when asked. Use `--dry-run` and
  low intensity to rehearse a command before running it for real.
- **Watch the device's own limits.** The API enforces per-token intensity and
  duration caps (`britzelator tokens self` shows yours). Those caps exist to
  protect the wearer; do not try to work around them.
- **Never share credentials.** Your API token controls the hardware. Keep it out
  of screenshots, logs, shell history, issues, and commits — which is why this
  tool defaults to reading it from the environment or a gitignored `.env`.
- **Health and law.** Do not use this on anyone with a heart condition, an
  implanted electronic device, or any condition where electrical stimulation is
  risky, and make sure what you are doing is legal where you are.
- **Unauthorized automation.** The `random` and `--independent` modes schedule
  controls that fire while nobody is watching the screen. Only run them on a
  willing person who knows a session is running, and keep a way to stop it
  (`britzelator stop <shocker>` and Ctrl-C) within reach.

The authors take no responsibility for misuse or for injury caused by devices
driven by this software.

## Install

Prebuilt binaries for Linux, macOS, and Windows (amd64 and arm64) are attached
to each [release](https://github.com/teal-bauer/britzelator/releases), together
with `checksums.txt`:

```sh
# example: Linux amd64, adjust the version and architecture to match
curl -LO https://github.com/teal-bauer/britzelator/releases/latest/download/britzelator_0.1.0_linux_amd64.tar.gz
tar -xzf britzelator_0.1.0_linux_amd64.tar.gz britzelator
install -m 0755 britzelator ~/.local/bin/
```

From source, which needs Go 1.25 or newer:

```sh
git clone git@github.com:teal-bauer/britzelator.git
cd britzelator
go build -o britzelator .

# or install straight into $GOBIN
go install .
```

The module path is in a private repository, so `go install
github.com/teal-bauer/britzelator@latest` from elsewhere needs credentials
(`GOPRIVATE=github.com/teal-bauer/*` plus a token or SSH key). Cloning and
building avoids that.

## Setup

The API requires a `User-Agent` header, which the tool always sends, and takes
the API token in the `OpenShockToken` header. Create a token in the OpenShock
web UI, then provide it in any of these ways — first match wins:

```sh
britzelator --token <token> shockers list     # per invocation
export OPENSHOCK_TOKEN=<token>                # environment (OPENSHOCK_API_TOKEN also works)
echo 'OPENSHOCK_API_TOKEN=<token>' > .env     # .env in the working directory, read by default
britzelator config set token <token>          # ~/.config/britzelator/config.json (mode 0600)
```

`--env-file <path>` relocates the `.env` lookup and `--env-file ""` disables it.
`--server` or `OPENSHOCK_SERVER` points the tool at another host and defaults to
`https://api.openshock.app`. `britzelator config show` prints the effective
configuration with the secrets masked, and `britzelator whoami` shows which
token and account are in use.

## Usage

```
britzelator [global flags] <command> [flags]

control <shocker>...   send one control message (shock/vibrate/tone/stop)
random <shocker>...    randomized control loop over delay/intensity/duration ranges
stop <shocker>...      stop control on shockers
shockers list|info|pause|logs
hubs list|create|shockers|lcg|pair|delete
tokens self|list|get|create|update|pause|revoke
whoami                 current token and account
login                  obtain a session cookie (required for token management)
raw <METHOD> <path>    call any endpoint directly
config show|path|set|unset
version
```

Global flags (`--token`, `--session`, `--server`, `--user-agent`, `--json`,
`--timeout`, `-q`/`--quiet`, `-v`, `--config`, `--env-file`) may appear before or
after the subcommand.

### Referencing shockers

A target is a UUID, an exact shocker name, a unique name substring, or a
qualified `hub/shocker` / `owner/shocker` label. Names resolve against the
shockers you own and those shared with you; an ambiguous name fails with the
list of candidates. UUIDs skip the lookup entirely, so control works for shares
that do not appear in your listings.

```sh
britzelator shockers list
britzelator control LivingRoom --mode shock --intensity 40 --duration 1000
britzelator control Collar1 Collar2 --mode vibrate --intensity 20 --duration 3s
britzelator control "Home Hub/Collar" --mode tone --intensity 10 --duration 800
britzelator control LivingRoom --mode shock --intensity 30 --duration 500 --exclusive --name cli
britzelator stop LivingRoom
```

`--mode` accepts `shock`/`zap`, `vibrate`/`vib`, `tone`/`sound`/`beep`, and
`stop`. Intensity is 0..100 and duration is 300..65535 ms, matching the API's
documented bounds; out-of-range values are rejected before any request is sent.
`--duration` takes an optional `ms`, `s`, or `m` suffix (`1000`, `1000ms`,
`1.5s`), with bare numbers read as milliseconds. `--exclusive` sets the API's
exclusive flag, `--name` sets `customName` in the control log, and `--dry-run`
prints the exact JSON body instead of sending it.

### Random mode

`random` draws a delay, waits it out, then sends a control whose intensity and
duration are drawn uniformly from the supplied ranges. It repeats until
interrupted with Ctrl-C or until the round count is reached.

```sh
# one shock somewhere in the next 5..60s, intensity 10..40, duration 0.5..2s
britzelator random Collar --delay-min 5 --delay-max 60 \
    --intensity-min 10 --intensity-max 40 --duration-min 500ms --duration-max 2s

# a random 3..10 rounds, reproducible draws, nothing actually sent
britzelator random Collar --delay-min 10s --delay-max 30s --count-min 3 --count-max 10 --seed 42 --dry-run

# three shockers buzzing to their own schedules rather than together
britzelator random Collar1 Collar2 Collar3 --independent --delay-min 10s --delay-max 5m \
    --intensity-min 5 --intensity-max 20 --duration-min 300ms --duration-max 1s
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--delay-min` / `--delay-max` | 5 / 30 | wait before each round; bare numbers are seconds |
| `--intensity-min` / `--intensity-max` | 5 / 30 | intensity range (0..100) |
| `--duration-min` / `--duration-max` | 500 / 2000 | duration range (300ms..65535ms); bare numbers are milliseconds |
| `--mode` | shock | mode used for every round |
| `--count` | 0 | fixed rounds per loop; 0 means until interrupted |
| `--count-min` / `--count-max` | | draw the round count per loop from this range |
| `--independent` | off | one loop per shocker instead of one shared draw |
| `--seed` | 0 | fixed RNG seed for reproducible draws |
| `--exclusive`, `--name` | | as for `control` |
| `--dry-run` | | draw and print, but do not send |

Durations accept a `ms`, `s`, or `m` suffix (`300`, `300ms`, `1.5s`, `2m`) and
delays accept the same suffixes with bare numbers read as seconds (`30`, `30s`,
`500ms`, `2m`). The same suffixes work for `control --duration` and the token
limit flags.

Without `--independent`, one draw drives every named shocker in a single control
request. With it, each shocker gets its own goroutine, random source, and
schedule, so they fire at unrelated times; `--count-min`/`--count-max` and
`--seed` are then drawn per loop (a fixed seed derives one seed per shocker, so
runs stay reproducible). The first API error stops the other loops and Ctrl-C
ends all of them.

Progress goes to stderr and can be silenced with `-q`/`--quiet`, which hides the
schedule, the per-round lines, and the summary while still reporting errors.
`--dry-run` still waits out the delays, which makes it usable as a pacing check.

### Hubs, shockers, and tokens

```sh
britzelator hubs list                       # hubs (devices) on the account
britzelator hubs shockers "Home Hub"        # shockers attached to a hub
britzelator hubs lcg "Home Hub"             # live control gateway host, port, and path
britzelator hubs pair "Home Hub"            # pairing code for a new device
britzelator hubs create --name "New Hub"
britzelator hubs delete "New Hub"

britzelator shockers info LivingRoom
britzelator shockers pause LivingRoom --paused=false
britzelator shockers logs Zap --limit 20

britzelator tokens self                     # the token in use, with its limits
britzelator tokens list
britzelator tokens create --name "cli" --permissions shockers.use --max-intensity 40 --valid-until 2027-01-01
britzelator tokens update <token-id> --max-intensity 25 --paused
britzelator tokens pause <token-id> --paused=false
britzelator tokens revoke <token-id>
```

Token management routes need a user *session* rather than an API token, so
`britzelator login --user <name-or-email> --password <pw>` stores the session
cookie the API returns (pass `--turnstile <token>` if the server demands a
Cloudflare Turnstile response, and prefer `OPENSHOCK_PASSWORD` over
`--password`). Revoking a token is the exception and works with an API token.

### Arbitrary endpoints

`raw` reaches anything else, including hub-facing endpoints that expect a
`DeviceToken` header:

```sh
britzelator raw GET /1/shockers/own
britzelator raw POST /2/shockers/control --data '{"shocks":[]}'
britzelator raw GET /1/device/self --header DeviceToken:<hub-token>
```

## API coverage

Endpoints are taken from the published OpenAPI documents
(`https://api.openshock.app/swagger/2/swagger.json` and the `swagger/1/`
equivalent), preferring v2 wherever v2 provides the route:

- **Control** uses `POST /2/shockers/control`.
- **Listings** use v1 (`/1/shockers/own`, `/1/shockers/shared`, `/1/devices`,
  `/1/devices/{id}/shockers`, `/1/devices/{id}/pair`, `/1/shockers/{id}/logs`)
  because v2 exposes no shocker or hub listing; their `{message, data}` envelope
  is unwrapped for display. Hub live-control-gateway info uses
  `GET /2/devices/{id}/lcg`.
- **Tokens** use `/2/tokens` (except `self`, they require a session cookie), and
  revocation uses `DELETE /1/tokens/{id}`, the only deletion route an API token
  may call.
- **Everything else** is reachable through `raw`.

Known limitations:

- No live-control WebSocket client, so nothing here uses the low-latency LCG
  path; hub LCG information is only reported, not connected to.
- Account, admin, share, and OTA surfaces are not wrapped as commands, though
  `raw` can call them.
- Random mode is a scheduler, not a session manager: it keeps no state across
  runs and does not resume.
- Only the `ApiToken` and session-cookie credentials are supported, not hub
  (`DeviceToken`) authentication as a first-class mode.

Exit codes: `0` success, `1` API or runtime error, `2` usage error.

## Development

```sh
go build ./...
go vet ./...
go test ./...
gofmt -l .
```

CI runs build, vet, tests, and a formatting check on Linux, macOS, and Windows.
Pushing a `v*` tag triggers GoReleaser, which builds the cross-platform archives
and publishes them as a GitHub release.

## License

Copyright (C) 2026 Teal Bauer.

Licensed under the GNU Affero General Public License, version 3 or later
(`AGPL-3.0-or-later`); the full text is in [LICENSE](LICENSE). Source files
carry `SPDX-License-Identifier: AGPL-3.0-or-later` headers.

This project is an independent client for the OpenShock API and is not
affiliated with or endorsed by the OpenShock project.