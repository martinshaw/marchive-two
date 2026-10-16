# Marchive

Marchive keeps a local copy of a webpage, the new articles on a news page, a podcast feed, or the text of a page. One Go binary runs the scheduler, the capture jobs, and the UI.

Version 0.2.2.

## Contents

- [Requirements](#requirements)
- [Install](#install)
- [Build](#build)
- [Run](#run)
- [Interface](#interface)
- [Sources](#sources)
- [Captures](#captures)
- [Settings](#settings)
- [Chrome](#chrome)
- [Data](#data)
- [Command line](#command-line)
- [Releases](#releases)

## Requirements

- macOS, Linux, or Windows
- Go 1.24 or newer, only to build from source
- Google Chrome, for webpage, article, and text captures

Podcast feeds and episode files are downloaded with Go’s HTTP client. They do not open Chrome.

Chrome is not copied into the binary. Marchive looks for Google Chrome, then Chromium, on the usual application paths and on `PATH`.

## Install

Download an archive from the [GitHub releases](https://github.com/martinshaw/marchive-two/releases). Each release has cgo-free builds for Linux, macOS, and Windows, on amd64 and arm64, plus `SHA256SUMS`.

## Build

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o marchive ./cmd/marchive
```

`CGO_ENABLED=0` keeps the binary free of native library dependencies. The SQLite driver is pure Go. `-trimpath` removes local paths from the binary. `-s` drops the symbol table and `-w` drops DWARF debugging information.

```bash
make test
make build
```

## Run

```bash
./marchive
```

That starts the scheduler and opens the UI at `http://127.0.0.1:<port>/`. A second `marchive` finds the running process and opens the UI again.

`marchive serve` starts the same process without opening a browser. One process owns a data directory. A lock file stops a second copy from using it.

The runner is off until you start it. Stopping it cancels queued captures and leaves the UI open. Cancel stops the capture that is already running.

## Interface

The window follows a desktop layout: a menu bar, a toolbar, a sidebar, and tables.

- Sidebar counts update as sources and captures change.
- Tables can be filtered, sorted, and resized. A clipped cell shows its full text in a tooltip.
- Status chips on captures and items show progress. A capture fills as its items finish. A job that is still working, with nothing finished yet, shows a moving bar.
- The library refreshes in place. Adding, editing, removing, or updating a source or capture does not jump the scroll position. A hidden window polls less often.
- Shortcuts use Control, shown as ⌃: ⌃N new source, ⌃R run, ⌃E edit, ⌃O open files, ⌃, settings, ⌃1 sources, ⌃2 captures.

## Sources

Each source has a kind and a schedule.

- **Webpage.** HTML, readable text, metadata, a full-page screenshot, MHTML, and `words.json`. A new folder is written only when the text changes.
- **Article index.** New same-site articles from a section or homepage. An optional CSS selector picks the links. Each article can keep the same page files, including `words.json`.
- **Podcast.** New episodes, identified by the RSS GUID, then the enclosure URL. Each episode can keep the audio file and `metadata.json`.
- **Text extract.** Readable text and metadata when the page changes.

For each kind, choose **All data** or the individual parts to keep. Webpage and article captures use Chrome. Podcast audio does not.

The archive folder defaults to `Marchive` in your home directory. The first run of an article list or podcast takes at most 20 new items. Raise “new items per run”, or pass `--max-new 0`, to take more. `0` means no limit. The shortest schedule interval is 30 seconds.

Changing how often a source is checked does not move an existing next run. The new interval is used when a capture finishes.

## Captures

A capture is one run of a schedule. Its items are the pages or episodes from that run. The status chip shows queued, running, succeeded, failed, cancelled, or skipped.

Article and episode failures are stored on the item. The rest of the run continues. A capture that crashes the process is marked failed the next time Marchive starts.

## Settings

Settings are saved in the database.

- **Podcast downloads at once.** How many episode files download together inside one capture, from 1 to 6.
- **Browser captures at once.** Stays at one, so captures can share the signed-in Chrome profile.
- **UI port.** Random picks a free localhost port each time Marchive starts. A fixed port, from 1 to 65535, is used the next time it starts. The page shows the port this session is using.
- **Open on login.** Starts Marchive when you sign in.
- **Restart on crash.** That same service starts Marchive again if it crashes.

Login and crash recovery use one per-user service:

| System | Service |
| --- | --- |
| macOS | LaunchAgent `co.martinshaw.marchive` |
| Linux | systemd user service `marchive.service` (`systemctl --user`) |
| Windows | scheduled task `Marchive` |

The service runs `marchive serve` with the current data directory. Turning both settings off removes the service. A copy that is already running stays up until you quit it.

## Chrome

Webpage, article, and text captures render the page in Chrome. On macOS, Marchive starts Chrome through Launch Services and attaches to it, so a command-line parent does not trip Chrome’s startup registration. If Chrome quits while it is starting, that capture fails once instead of launching Chrome again.

If Chrome is missing, the UI can download the current stable release and show the install progress.

**Sign in** opens Marchive’s own Chrome profile. Sign in to sites you already subscribe to, then quit Chrome. Later captures reuse that profile. Marchive stores the page Chrome can already open. It does not try to defeat a paywall.

## Data

A flag wins, then `MARCHIVE_HOME`, then the per-user folder:

| System | Data directory |
| --- | --- |
| macOS | `~/Library/Application Support/Marchive` |
| Linux | `$XDG_DATA_HOME/Marchive`, or `~/.local/share/Marchive` |
| Windows | `%AppData%\Marchive` |

That folder holds `marchive.db`, the Chrome profile, the lock, the current port, and `marchive.log`. Captured files go in each schedule’s archive folder, not in this data directory.

`--data-dir` is useful for a second library or a test run.

## Command line

```bash
./marchive
./marchive serve
./marchive runner start
./marchive runner stop
./marchive runner status
./marchive sources list
./marchive sources add --url https://example.com/feed.xml --provider podcast --dest ~/Marchive/show --every 1h
./marchive sources delete 1
./marchive schedules run 1
./marchive schedules set 1 --every 6h
./marchive runs list
./marchive runs cancel 4
./marchive browser login
```

`marchive --version` prints the version baked into the binary.

## Releases

Pushing a `v*` tag builds the release archives and attaches them to a GitHub release.
