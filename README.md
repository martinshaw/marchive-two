# Marchive

Marchive keeps a local copy of a webpage, the new articles on a news page, a podcast feed, or the text of a page. One Go binary runs the scheduler, the capture jobs, and the UI.

Version 0.1.0.

## Requirements

- macOS, for the day-to-day app. Released binaries are also built for Linux and Windows
- Go 1.24 or newer, only to build from source
- Google Chrome, for webpage, article, and text captures. Podcast feeds are fetched over HTTP and do not need Chrome

Chrome is not copied into the binary. Marchive looks for it at `/Applications/Google Chrome.app`, then Chromium.

## Build

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o marchive ./cmd/marchive
```

`CGO_ENABLED=0` keeps the binary free of native library dependencies. The SQLite driver is pure Go.

```bash
make test
make build
```

## Run

```bash
./marchive
```

That starts the scheduler process and opens the UI at `http://127.0.0.1:<port>/`. The database, Chrome profile, and port file live in `~/Library/Application Support/Marchive`. Archive files go in the folder you choose for each schedule.

The runner is off until you start it. Stopping it cancels queued captures and leaves the UI open. Cancel stops the capture that is already running. A failed article or episode is recorded on that item and does not stop the other jobs.

`marchive serve` starts the same process without opening a browser. If it is already running, `marchive` only opens the UI.

```bash
./marchive runner start
./marchive runner stop
./marchive sources add --url https://example.com/feed.xml --provider podcast --dest ~/Marchive/show --every 1h
./marchive schedules run 1
./marchive runs list
./marchive runs cancel 4
./marchive browser login
```

`browser login` opens Marchive’s Chrome profile. Sign in to sites you already subscribe to, then quit Chrome. Later captures reuse that profile. Marchive stores the page Chrome actually receives. It does not try to defeat a paywall.

## What it stores

- **Webpage.** A snapshot (HTML, readable text, metadata, full-page screenshot, MHTML). A new folder is written only when the text changes.
- **Article index.** New same-site articles from a section or homepage. An optional CSS selector picks the links.
- **Podcast.** New episodes, identified by the RSS GUID, then the enclosure URL. Each episode gets the media file and `metadata.json`.
- **Text extract.** Readable text and metadata when the page changes.

The first run of an article list or podcast takes at most 20 new items. Raise “new items per run”, or pass `--max-new 0`, to take more. `0` means no limit.

## Data

`MARCHIVE_HOME` or `--data-dir` overrides the Application Support directory. That is useful for a second library or a test run.

## Releases

Pushing a `v*` tag builds cgo-free archives for Linux, macOS, and Windows (amd64 and arm64) and attaches them to a GitHub release. `marchive --version` prints the version baked into that binary.
