# Marchive

Marchive keeps a local copy of a webpage, the new articles on a news page, a podcast feed, the text of a page, posts from X, new Hacker News stories, search results from Google and DuckDuckGo, or a YouTube video. One Go binary runs the scheduler, the capture jobs, and the UI.

Version 0.2.5.

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
- Go 1.24 or newer, and Node.js 22, only to build from source
- Google Chrome, for webpage, article, text, X, Hacker News screenshot, and search captures

Podcast feeds and episode files are downloaded with Go’s HTTP client. They do not open Chrome.

Chrome is not copied into the binary. Marchive looks for Google Chrome, then Chromium, on the usual application paths and on `PATH`.

## Install

Download an archive from the [GitHub releases](https://github.com/martinshaw/marchive-two/releases). Each release has cgo-free builds for Linux, macOS, and Windows, on amd64 and arm64, plus `SHA256SUMS`.

## Build

```bash
make build
```

`make build` installs the web dependencies, builds the React page into `internal/httpapi/ui/dist`, then compiles the binary. The Go compiler embeds that page.

`CGO_ENABLED=0` keeps the binary free of native library dependencies. The SQLite driver is pure Go. `-trimpath` removes local paths from the binary. `-s` drops the symbol table and `-w` drops DWARF debugging information.

```bash
make test
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
- **X trending.** New posts from a trending, news, sports, entertainment, or other X page. Each post keeps its text, author, hashtags, and words, plus any images, audio, and video attached to it. Sign in with Chrome on the Marchive profile first. News, Sports, and Entertainment list topics; Marchive opens a few of those pages and saves the posts.
- **Hacker News entry.** New stories from Newest, the front page, Best, Ask, Show, or Jobs. Each story keeps its text, comments, score, author, and word index. When the story links to a website, Marchive also saves a screenshot and the readable text of that site. The word index includes the story, the comments, and the linked site.
- **Google search.** New organic results from a Google search address. Each result keeps its title, address, snippet, details, and word index, plus a screenshot of the results page and of the linked page.
- **DuckDuckGo.** New organic results from a DuckDuckGo search address, saved the same way as Google results.
- **YouTube video.** One watch, shorts, or youtu.be address. Each video can keep its description, a subtitle file, a low, medium, or high picture, an audio file (best, M4A, or Opus), and comments. Comments can be all of them or a chosen number. yt-dlp downloads those files. Chrome saves a picture of the watch page and, when you have signed in on the Marchive profile, passes those cookies to yt-dlp.

For each kind, choose **All data** or the individual parts to keep. Webpage, article, X, Hacker News screenshot, search, and YouTube watch-page captures use Chrome. Podcast audio does not. Hacker News story text and comments come from the Hacker News API and do not need Chrome unless a screenshot is selected. YouTube files come from yt-dlp.

## yt-dlp

YouTube captures need the yt-dlp program. Adding a YouTube source asks for it before the source is saved. Settings and the Marchive menu can download it too. The current release for this computer is stored in Marchive's data folder, next to the Chrome profile. Update replaces that copy with the current release. The download can be cancelled while it is in progress.

The archive folder defaults to `Marchive` in your home directory. The first run of an article list or podcast takes at most 20 new items. Raise “new items per run”, or pass `--max-new 0`, to take more. `0` means no limit. The shortest schedule interval is 30 seconds.

Changing how often a source is checked does not move an existing next run. The new interval is used when a capture finishes.

## Captures

A capture is one run of a schedule. Its items are the pages or episodes from that run. The status chip shows queued, running, succeeded, failed, cancelled, or skipped.

Article and episode failures are stored on the item. The rest of the run continues. A capture that crashes the process is marked failed the next time Marchive starts.

## Settings

Settings are saved in the database.

- **Podcast downloads at once.** How many episode files download together inside one capture, from 1 to 6.
- **Browser captures at once.** Stays at one, so captures can share the signed-in Chrome profile.
- **Item time limit.** Seconds allowed for one page, file, or request. 0 means no time limit. The longest stored limit is 24 hours.
- **Capture time limit.** Seconds allowed for the whole capture. 0 means no time limit. The longest stored limit is 24 hours.
- **Item attempts.** How many times to try one page, file, or request, from 1 to 10. 1 means a single attempt.
- **Capture attempts.** How many times to try the whole capture, from 1 to 10. 1 means a single attempt. Cancelling a capture does not use another attempt.
- **UI port.** Random picks a free localhost port each time Marchive starts. A fixed port, from 1 to 65535, is used the next time it starts. The page shows the port this session is using. Install Marchive from the browser after choosing a fixed port, so the installed app keeps opening the same address.
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

Webpage, article, text, X, Hacker News screenshot, and search captures render the page in Chrome. On macOS, Marchive starts Chrome through Launch Services and attaches to it, so a command-line parent does not trip Chrome’s startup registration. If Chrome quits while it is starting, that capture fails once instead of launching Chrome again.

If Chrome is missing, the UI can download the current stable release and show the install progress.

**Sign in** opens the Marchive profile stored in the app data folder. Chrome shows that profile under the name Marchive. Sign in to sites you already subscribe to, then quit Chrome. Later captures reuse that profile. Marchive stores the page Chrome can already open. It does not try to defeat a paywall.

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

`marchive cli-setup` asks for a fixed UI port and the login service. Open on login and restart on crash both default to yes, so the same setup works over SSH. The port is used the next time Marchive starts.

```bash
./marchive
./marchive serve
./marchive cli-setup
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
