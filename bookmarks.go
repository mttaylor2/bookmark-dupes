package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

// node mirrors the shape Chrome uses for its Bookmarks JSON file. A node is
// either a folder (with children) or a bookmark (with a url). Chrome doesn't
// distinguish these with separate structs, so neither do we.
type node struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Children []node `json:"children"`
}

type bookmarksFile struct {
	Roots struct {
		BookmarkBar node `json:"bookmark_bar"`
		Other       node `json:"other"`
		Synced      node `json:"synced"`
	} `json:"roots"`
}

// entry is a single bookmark flattened out of the folder tree, with its
// folder path kept around so we can tell the user where to go find it.
type entry struct {
	Name string
	URL  string
	Path string
}

func loadEntries(path string) ([]entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var bf bookmarksFile
	if err := json.Unmarshal(raw, &bf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	var out []entry
	walk(bf.Roots.BookmarkBar, "", &out)
	walk(bf.Roots.Other, "", &out)
	walk(bf.Roots.Synced, "", &out)
	return out, nil
}

func walk(n node, path string, out *[]entry) {
	if n.Type == "url" {
		*out = append(*out, entry{Name: n.Name, URL: n.URL, Path: path})
		return
	}

	childPath := path
	if n.Name != "" {
		if path == "" {
			childPath = n.Name
		} else {
			childPath = path + "/" + n.Name
		}
	}

	for _, c := range n.Children {
		walk(c, childPath, out)
	}
}

// defaultBookmarksPath guesses where Chrome keeps its bookmarks file for the
// current OS and user. It's a guess, not a guarantee: other Chromium
// profiles (Brave, Edge, a non-Default Chrome profile) live elsewhere, which
// is exactly what the -file flag is for.
func defaultBookmarksPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch runtime.GOOS {
	case "darwin":
		return home + "/Library/Application Support/Google/Chrome/Default/Bookmarks"
	case "linux":
		return home + "/.config/google-chrome/Default/Bookmarks"
	case "windows":
		return home + `\AppData\Local\Google\Chrome\User Data\Default\Bookmarks`
	default:
		return ""
	}
}
