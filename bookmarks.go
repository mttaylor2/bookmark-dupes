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

	format, err := detectFormat(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if format == "firefox" {
		return loadFirefoxEntries(raw, path)
	}
	return loadChromeEntries(raw, path)
}

// detectFormat tells Chrome's Bookmarks format from a Firefox JSON export.
// Both are plain JSON but shaped differently: Chrome nests everything under
// a "roots" object, while a Firefox export is itself the root node and
// carries a moz-place "type" (or, in older exports, "root") directly.
func detectFormat(raw []byte) (string, error) {
	var probe struct {
		Roots json.RawMessage `json:"roots"`
		Type  string          `json:"type"`
		Root  string          `json:"root"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", err
	}
	switch {
	case probe.Roots != nil:
		return "chrome", nil
	case probe.Type != "" || probe.Root != "":
		return "firefox", nil
	default:
		return "", fmt.Errorf("unrecognized bookmarks file format")
	}
}

func loadChromeEntries(raw []byte, path string) ([]entry, error) {
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

// firefoxNode mirrors the shape of an uncompressed Firefox bookmarks JSON
// backup. Firefox's automatic backups are jsonlz4-compressed and need
// decompressing before this tool can read them - see the README. Firefox
// tags every node with a "type" string instead of splitting folders and
// bookmarks into separate fields the way Chrome does.
type firefoxNode struct {
	Type     string        `json:"type"`
	Title    string        `json:"title"`
	URI      string        `json:"uri"`
	Children []firefoxNode `json:"children"`
}

const firefoxBookmarkType = "text/x-moz-place"

func loadFirefoxEntries(raw []byte, path string) ([]entry, error) {
	var root firefoxNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	var out []entry
	walkFirefox(root, "", &out)
	return out, nil
}

func walkFirefox(n firefoxNode, path string, out *[]entry) {
	if n.Type == firefoxBookmarkType {
		if n.URI == "" {
			return
		}
		*out = append(*out, entry{Name: n.Title, URL: n.URI, Path: path})
		return
	}

	childPath := path
	if n.Title != "" {
		if path == "" {
			childPath = n.Title
		} else {
			childPath = path + "/" + n.Title
		}
	}

	for _, c := range n.Children {
		walkFirefox(c, childPath, out)
	}
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
