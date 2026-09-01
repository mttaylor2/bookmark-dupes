# bookmark-dupes

I have thousands of Chrome bookmarks going back years, spread across dozens
of folders, and I know for a fact I've bookmarked the same page in two
different folders more times than I'd like to admit. This is a small
command-line tool that answers exactly one question: which URLs appear more
than once in my bookmarks, and where are the copies?

It reads Chrome's `Bookmarks` file directly (it's plain JSON, no database
involved) and reports exact URL duplicates grouped by folder path.

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

Matching is on the literal URL string. `example.com/page` and
`example.com/page/` count as different bookmarks, as do the same page with
and without tracking query parameters. Firefox isn't supported yet either
(it keeps bookmarks in a SQLite database rather than JSON, so it needs
different handling).

## License

MIT, see LICENSE.
