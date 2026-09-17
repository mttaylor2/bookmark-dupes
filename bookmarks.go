package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"sort"
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

// folderCount is one row of the -stats report: a folder path and how many
// bookmarks (not subfolders) sit directly in it.
type folderCount struct {
	Path  string
	Count int
}

// folderCounts tallies bookmarks per folder path, busiest folder first (ties
// broken alphabetically so the output order is stable across runs).
// Bookmarks sitting at the root of a browser's bookmark list, outside any
// named folder, are grouped under "(root)".
func folderCounts(entries []entry) []folderCount {
	byPath := make(map[string]int)
	for _, e := range entries {
		path := e.Path
		if path == "" {
			path = "(root)"
		}
		byPath[path]++
	}

	counts := make([]folderCount, 0, len(byPath))
	for path, n := range byPath {
		counts = append(counts, folderCount{Path: path, Count: n})
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Count != counts[j].Count {
			return counts[i].Count > counts[j].Count
		}
		return counts[i].Path < counts[j].Path
	})
	return counts
}

// writeDeduped removes duplicate bookmarks from a Chrome Bookmarks file and
// writes the result back to disk, keeping the first occurrence of each
// duplicated URL (bookmark bar, then other, then synced - the same order
// used to build the dupe report, so "first" means the same thing in both
// places). The original file is copied to path+".bak" first: this overwrites
// a file Chrome itself owns, and a mistake here is expensive to undo by hand.
//
// This only supports Chrome's format. The Firefox JSON this tool reads is a
// one-off export, not a live file - Firefox keeps its real bookmarks in a
// SQLite database, so there's nothing sensible to write back to.
func writeDeduped(path string) (removed int, backupPath string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, "", fmt.Errorf("reading %s: %w", path, err)
	}

	format, err := detectFormat(raw)
	if err != nil {
		return 0, "", fmt.Errorf("parsing %s: %w", path, err)
	}
	if format != "chrome" {
		return 0, "", fmt.Errorf("-write only supports Chrome's Bookmarks format, not a Firefox export")
	}

	info, err := os.Stat(path)
	if err != nil {
		return 0, "", fmt.Errorf("stat %s: %w", path, err)
	}

	// Decode generically rather than into our node struct: node only knows
	// about type/name/url/children, and re-encoding it would silently drop
	// every field Chrome actually relies on (id, guid, date_added, ...).
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]interface{}
	if err := dec.Decode(&doc); err != nil {
		return 0, "", fmt.Errorf("parsing %s: %w", path, err)
	}

	roots, ok := doc["roots"].(map[string]interface{})
	if !ok {
		return 0, "", fmt.Errorf("parsing %s: missing roots", path)
	}

	seen := make(map[string]bool)
	for _, key := range []string{"bookmark_bar", "other", "synced"} {
		r, ok := roots[key].(map[string]interface{})
		if !ok {
			continue
		}
		_, n := removeDuplicateURLs(r, seen)
		removed += n
	}

	if removed == 0 {
		return 0, "", nil
	}

	// Chrome stores an MD5 checksum of the bookmark tree and uses a mismatch
	// to detect the file was edited outside Chrome. Dropping it here means
	// Chrome just recomputes it next time it saves, instead of us having to
	// reverse-engineer and reproduce its checksum algorithm.
	delete(doc, "checksum")

	backupPath = path + ".bak"
	if err := os.WriteFile(backupPath, raw, info.Mode().Perm()); err != nil {
		return 0, "", fmt.Errorf("writing backup %s: %w", backupPath, err)
	}

	out, err := json.MarshalIndent(doc, "", "   ")
	if err != nil {
		return 0, "", fmt.Errorf("encoding deduplicated bookmarks: %w", err)
	}
	if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
		return 0, "", fmt.Errorf("writing %s: %w", path, err)
	}

	return removed, backupPath, nil
}

// removeDuplicateURLs walks a folder node (decoded as generic JSON, so
// whatever fields Chrome put there besides type/url/children ride along
// unchanged) and drops every bookmark whose normalized URL was already seen
// earlier in the walk, keeping the first occurrence. It reports whether the
// node it was called on should be kept by its parent, and how many bookmarks
// it removed from underneath it.
func removeDuplicateURLs(n map[string]interface{}, seen map[string]bool) (keep bool, removed int) {
	if t, _ := n["type"].(string); t == "url" {
		u, _ := n["url"].(string)
		key := normalizeURL(u)
		if seen[key] {
			return false, 1
		}
		seen[key] = true
		return true, 0
	}

	children, _ := n["children"].([]interface{})
	kept := children[:0]
	for _, c := range children {
		cm, ok := c.(map[string]interface{})
		if !ok {
			kept = append(kept, c)
			continue
		}
		keepChild, r := removeDuplicateURLs(cm, seen)
		removed += r
		if keepChild {
			kept = append(kept, c)
		}
	}
	n["children"] = kept
	return true, removed
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
