# bookmark-dupes

I have thousands of Chrome bookmarks going back years, spread across dozens
of folders, and I know for a fact I've bookmarked the same page in two
different folders more times than I'd like to admit. This is a small
command-line tool that answers exactly one question: which URLs appear more
than once in my bookmarks, and where are the copies?

It reads Chrome's `Bookmarks` file directly (it's plain JSON, no database
involved) and reports exact URL duplicates grouped by folder path. It also
reads a Firefox bookmarks JSON export - the file format differs from
Chrome's, so `-file` accepts either and the tool figures out which one it's
looking at.

## Usage

```
go run . 
```

With no flags it looks for Chrome's default profile in the usual place for
your OS. If that's wrong, or you want to check a different profile, point it
at the file directly:

```
go run . -file "$HOME/Library/Application Support/Google/Chrome/Profile 2/Bookmarks"
```

Example output:

```
https://news.ycombinator.com/item?id=32154123 (2 copies)
  - HN: a thing I meant to read [Bookmarks Bar/To Read]
  - (untitled) [Imported/Old Reading List]
no duplicate bookmarks found
```

(That second line only shows up if there genuinely are none — it won't print
alongside real duplicates.)

Pass `-json` for machine-readable output instead - a JSON array of groups,
each with the normalized key, a count, and the entries that share it:

```
go run . -json
```

```json
[
  {
    "key": "http://news.ycombinator.com/item?id=32154123",
    "count": 2,
    "entries": [
      { "name": "HN: a thing I meant to read", "url": "https://news.ycombinator.com/item?id=32154123", "path": "Bookmarks Bar/To Read" },
      { "name": "", "url": "https://news.ycombinator.com/item?id=32154123", "path": "Imported/Old Reading List" }
    ]
  }
]
```

With `-json`, no duplicates prints an empty array (`[]`) rather than the
text-mode message.

## Building

```
go build -o bookmark-dupes .
```

Standard library only, no dependencies to fetch.

## Where it looks by default

- macOS: `~/Library/Application Support/Google/Chrome/Default/Bookmarks`
- Linux: `~/.config/google-chrome/Default/Bookmarks`
- Windows: `%USERPROFILE%\AppData\Local\Google\Chrome\User Data\Default\Bookmarks`

If you use a non-default profile, or a Chromium-based browser other than
Chrome (Brave, Edge, Vivaldi), the file lives somewhere else with a similar
layout — use `-file` to point at it.

## Limitations right now

URLs are compared after normalizing: `http` and `https` are treated as the
same scheme, a trailing slash on the path is ignored, the host is
lowercased, and common tracking parameters (`utm_*`, `gclid`, `fbclid`, and
similar) are stripped before comparing. It's still a plain string match
after that, so different query parameters, different paths that redirect to
the same place, or `www.` vs no `www.` still count as separate bookmarks.
When a duplicate group includes URLs that don't match exactly, the actual
URL is shown next to each entry so you can see what varied.

Firefox keeps its live bookmarks in a SQLite database, not JSON, so this
tool can't read your profile directly - export first (in Firefox, "Bookmarks
> Manage Bookmarks > Import and Backup > Backup..."). That produces a
`.jsonlz4` file, which is JSON compressed with a Mozilla-specific framing
around lz4; decompress it to plain JSON before pointing `-file` at it, since
this tool has no decompression support (staying dependency-free means no
lz4 library, and hand-rolling one isn't worth it for a personal tool).

## License

MIT, see LICENSE.
