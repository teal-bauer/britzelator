# Changelog

Notable changes per released version. This project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.1.0 — 2026-09-27

Initial release.

- Control shockers through the OpenShock v2 API: `shock`, `vibrate`, `tone`
  (Sound), and `stop`, with intensity, duration, `exclusive`, and a custom
  control-log name, validated against the documented bounds before sending.
- Random mode: draws a delay, waits it out, then sends a control with randomly
  drawn intensity and duration, repeating until interrupted or until the round
  count is reached. `--count-min`/`--count-max` draw the round count per loop,
  and `--independent` runs one loop per shocker so several shockers fire on
  unrelated schedules.
- Targets are UUIDs, exact names, unique name substrings, or `hub/shocker` and
  `owner/shocker` qualifiers, resolved against owned and shared shockers.
- Hub commands: list, create, shockers, live control gateway info, pair code,
  delete. Shocker commands: list, info, pause, control logs.
- API token commands: self, list, get, create, update, pause, revoke, plus
  session login for the token routes that require a user session cookie.
- `raw` for arbitrary endpoints, with custom headers and request bodies.
- Credentials from flags, environment variables (`OPENSHOCK_TOKEN` or
  `OPENSHOCK_API_TOKEN`, `OPENSHOCK_SESSION`), a `.env` file, or the config file.
- Durations accept `ms`, `s`, and `m` suffixes; delays accept the same suffixes
  with bare numbers read as seconds.
- `-q`/`--quiet` suppresses progress output while still reporting errors, and
  `--json` returns raw API payloads.
- Prebuilt binaries for Linux, macOS, and Windows on amd64 and arm64.

Licensed under the GNU Affero General Public License, version 3 or later.