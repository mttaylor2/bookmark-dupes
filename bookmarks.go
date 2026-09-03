package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
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

// trackingParams are query string keys that change per share link without
// changing what page loads. Dropping them means an article bookmarked once
// from a newsletter link and once from a tweet still counts as one duplicate.
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"utm_id":       true,
	"gclid":        true,
	"fbclid":       true,
	"msclkid":      true,
	"mc_cid":       true,
	"mc_eid":       true,
	"igshid":       true,
	"yclid":        true,
	"ref":          true,
	"ref_src":      true,
	"si":           true,
}

// normalizeURL builds a comparison key for a bookmark URL. It folds https
// into http, drops a trailing slash from the path, strips known tracking
// query parameters, and lowercases the host - none of which change what
// page the browser actually loads, so treating them as equal catches
// duplicates that a literal string comparison would miss. The original URL
// is kept on the entry for display; this key is only used for grouping.
func normalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == "https" {
		scheme = "http"
	}

	path := u.Path
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}

	q := u.Query()
	for key := range q {
		if trackingParams[strings.ToLower(key)] {
			q.Del(key)
		}
	}

	var key strings.Builder
	key.WriteString(scheme)
	key.WriteString("://")
	key.WriteString(strings.ToLower(u.Host))
	key.WriteString(path)
	if encoded := q.Encode(); encoded != "" {
		key.WriteByte('?')
		key.WriteString(encoded)
	}
	if u.Fragment != "" {
		key.WriteByte('#')
		key.WriteString(u.Fragment)
	}
	return key.String()
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
