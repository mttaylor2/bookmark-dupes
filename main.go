// bookmark-dupes answers one question: which URLs show up more than once in
// your Chrome bookmarks, and where?
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	filePath := flag.String("file", defaultBookmarksPath(), "path to Chrome's Bookmarks JSON file")
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
		byURL[e.URL] = append(byURL[e.URL], e)
	}

	var dupeURLs []string
	for url, es := range byURL {
		if len(es) > 1 {
			dupeURLs = append(dupeURLs, url)
		}
	}
	sort.Strings(dupeURLs)

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
			if e.Path == "" {
				fmt.Printf("  - %s\n", name)
			} else {
				fmt.Printf("  - %s [%s]\n", name, e.Path)
			}
		}
	}
}
