// bookmark-dupes answers one question: which URLs show up more than once in
// your Chrome bookmarks, and where?
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	filePath := flag.String("file", defaultBookmarksPath(), "path to Chrome's Bookmarks JSON file")
	jsonOutput := flag.Bool("json", false, "print duplicates as JSON instead of text")
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "bookmark-dupes: could not guess a Bookmarks file location, pass one with -file")
		os.Exit(1)
	}

	entries, err := loadEntries(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bookmark-dupes: %v\n", err)
		os.Exit(1)
	}

	byURL := make(map[string][]entry)
	for _, e := range entries {
		key := normalizeURL(e.URL)
		byURL[key] = append(byURL[key], e)
	}

	var dupeURLs []string
	for url, es := range byURL {
		if len(es) > 1 {
			dupeURLs = append(dupeURLs, url)
		}
	}
	sort.Strings(dupeURLs)

	if *jsonOutput {
		printJSON(dupeURLs, byURL)
		return
	}

	if len(dupeURLs) == 0 {
		fmt.Println("no duplicate bookmarks found")
		return
	}

	for _, url := range dupeURLs {
		es := byURL[url]
		fmt.Printf("%s (%d copies)\n", url, len(es))
		for _, e := range es {
			name := e.Name
			if name == "" {
				name = "(untitled)"
			}
			line := "  - " + name
			if e.Path != "" {
				line += " [" + e.Path + "]"
			}
			if e.URL != url {
				line += " (" + e.URL + ")"
			}
			fmt.Println(line)
		}
	}
}

// jsonEntry and jsonGroup define the -json output shape. Kept separate from
// the internal entry type so changes to the internal representation don't
// silently change the output format that scripts may depend on.
type jsonEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Path string `json:"path"`
}

type jsonGroup struct {
	Key     string      `json:"key"`
	Count   int         `json:"count"`
	Entries []jsonEntry `json:"entries"`
}

func printJSON(dupeURLs []string, byURL map[string][]entry) {
	groups := make([]jsonGroup, 0, len(dupeURLs))
	for _, url := range dupeURLs {
		es := byURL[url]
		entries := make([]jsonEntry, 0, len(es))
		for _, e := range es {
			entries = append(entries, jsonEntry{Name: e.Name, URL: e.URL, Path: e.Path})
		}
		groups = append(groups, jsonGroup{Key: url, Count: len(es), Entries: entries})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(groups); err != nil {
		fmt.Fprintf(os.Stderr, "bookmark-dupes: %v\n", err)
		os.Exit(1)
	}
}
