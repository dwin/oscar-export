# oscar-export

`oscar-export` is a standalone Go CLI for exporting cache data from [OSCAR-code](https://gitlab.com/CrimsonNape/OSCAR-code) directly to CSV without using the OSCAR GUI.

The Go module lives at `github.com/dwin/oscar-export`. If you install it with `go install`, the executable name is `oscar-export`.

It reproduces OSCAR's existing `Summary`, `Sessions`, and `Details` CSV exports from the on-disk cache under an OSCAR data root such as:

```text
/path/to/OSCAR_Data
```

## Relationship to OSCAR

- Current upstream repository: <https://gitlab.com/CrimsonNape/OSCAR-code>
- Upstream project license: GPL v3 only, documented in the upstream [`COPYING`](https://gitlab.com/CrimsonNape/OSCAR-code/-/blob/master/COPYING) file
- `oscar-export` is focused on interoperability with OSCAR cache formats and reproducing OSCAR CSV output behavior
- `oscar-export` is an independent CLI and is not affiliated with the OSCAR maintainers

## What It Exports

The CLI supports these export modes:

- `summary`: day-level CSV matching OSCAR's `Summary` export
- `sessions`: per-session CSV matching OSCAR's `Sessions` export
- `details`: raw event rows matching OSCAR's `Details` export

By default it:

- loads the selected OSCAR profile
- merges all CPAP machines in that profile
- uses the profile timezone from `Profile.xml`
- uses the profile sleep-day split rules from `Profile.xml`
- writes the same default filename style OSCAR uses when `--out` is omitted

## Supported Cache Formats

This tool is intentionally scoped to the OSCAR cache formats present in the target data set:

- `Summaries.xml.gz`: version `1`
- summary `.000`: version `18`
- event `.001`: version `10`
- `Sessions.info`: version `2`

Unsupported versions fail fast with a clear error.

## License

This repository is licensed under the GNU General Public License v3.0 only. See [LICENSE](LICENSE).

Using GPL v3 only here keeps redistribution terms aligned with the current OSCAR-code upstream license and removes ambiguity around reuse of OSCAR-compatible export behavior and future upstream-derived work.

## Installation

Install the latest version with Go:

```bash
go install github.com/dwin/oscar-export@latest
```

Or run it from a local checkout:

```bash
git clone https://github.com/dwin/oscar-export.git
cd oscar-export
```

## CLI Layout

The command layout is:

```text
oscar-export export summary
oscar-export export sessions
oscar-export export details
```

If you're running from a local checkout instead of an installed binary, replace `oscar-export` with `go run .`.

Shared flags:

- `--root`
- `--profile-user`
- `--from`
- `--to`
- `--out`
- `--serial` optional, for single-machine debugging

## Usage

Generate a summary export:

```bash
oscar-export export summary \
  --root /path/to/OSCAR_Data \
  --profile-user your-profile-user \
  --from 2026-03-03 \
  --to 2026-03-17
```

Generate a sessions export:

```bash
oscar-export export sessions \
  --root /path/to/OSCAR_Data \
  --profile-user your-profile-user \
  --from 2026-03-03 \
  --to 2026-03-17
```

Generate a details export:

```bash
oscar-export export details \
  --root /path/to/OSCAR_Data \
  --profile-user your-profile-user \
  --from 2026-02-20 \
  --to 2026-03-06
```

Write to a specific file:

```bash
oscar-export export summary \
  --root /path/to/OSCAR_Data \
  --profile-user your-profile-user \
  --from 2026-03-03 \
  --to 2026-03-17 \
  --out ./oscar_summary.csv
```

Export a single machine by serial:

```bash
oscar-export export sessions \
  --root /path/to/OSCAR_Data \
  --profile-user your-profile-user \
  --serial 23182797776 \
  --from 2026-03-03 \
  --to 2026-03-17
```

## Behavior Notes

- The profile password in `Profile.xml` is only an OSCAR UI access check. It does not encrypt the cache files.
- Sleep-day assignment follows OSCAR's day split behavior from `Profile.xml`.
- `Summary` uses merged day-level aggregates across enabled CPAP sessions.
- `Sessions` exports per-session rows for the grouped sleep days.
- `Details` exports only sessions that have event data.
- Total time uses overlap-aware union logic, matching OSCAR rather than naively summing session lengths.

## Development

Fetch dependencies:

```bash
go mod tidy
```

Run unit tests:

```bash
go test ./...
```

Run the env-gated fixture comparisons against a real OSCAR data directory and existing OSCAR CSV exports:

```bash
OSCAR_DATA_DIR='/path/to/OSCAR_Data' \
OSCAR_EXPORT_DIR='/path/to/oscar-csv-exports' \
go test ./...
```

Optional profile override for integration tests:

```bash
OSCAR_PROFILE_USER=your-profile-user
```

## Project Layout

```text
.
  main.go
  cmd/
    root.go
    export.go
  internal/
    cache/
    export/
```
