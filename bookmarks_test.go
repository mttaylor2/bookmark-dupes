package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const chromeFixture = `{
	"roots": {
		"bookmark_bar": {
			"type": "folder",
			"name": "Bookmarks Bar",
			"children": [
				{"type": "url", "name": "Example", "url": "https://example.com/page"},
				{"type": "folder", "name": "Sub", "children": [
					{"type": "url", "name": "Example dup", "url": "http://example.com/page/"}
				]}
			]
		},
		"other": {
			"type": "folder",
			"name": "Other Bookmarks",
			"children": [
				{"type": "url", "name": "Unique", "url": "https://unique.example.com/"}
			]
		},
		"synced": {
			"type": "folder",
			"name": "Mobile Bookmarks",
			"children": []
		}
	}
}`

const firefoxFixture = `{
	"type": "text/x-moz-place-container",
	"title": "",
	"children": [
		{
			"type": "text/x-moz-place-container",
			"title": "Bookmarks Toolbar",
			"children": [
				{"type": "text/x-moz-place", "title": "Example FF", "uri": "https://example.org/a"}
			]
		},
		{
			"type": "text/x-moz-place-container",
			"title": "Other Bookmarks",
			"children": [
				{"type": "text/x-moz-place", "title": "Example FF dup", "uri": "http://example.org/a/"}
			]
		}
	]
}`

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"chrome", chromeFixture, "chrome", false},
		{"firefox", firefoxFixture, "firefox", false},
		{"neither", `{"foo": "bar"}`, "", true},
		{"invalid json", `not json`, "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := detectFormat([]byte(c.raw))
			if c.wantErr {
				if err == nil {
					t.Fatalf("detectFormat(%q) = %q, nil; want error", c.name, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectFormat(%q) returned error: %v", c.name, err)
			}
			if got != c.want {
				t.Fatalf("detectFormat(%q) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestLoadChromeEntries(t *testing.T) {
	entries, err := loadChromeEntries([]byte(chromeFixture), "fixture")
	if err != nil {
		t.Fatalf("loadChromeEntries: %v", err)
	}

	want := []entry{
		{Name: "Example", URL: "https://example.com/page", Path: "Bookmarks Bar"},
		{Name: "Example dup", URL: "http://example.com/page/", Path: "Bookmarks Bar/Sub"},
		{Name: "Unique", URL: "https://unique.example.com/", Path: "Other Bookmarks"},
	}

	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, e, want[i])
		}
	}
}

func TestLoadFirefoxEntries(t *testing.T) {
	entries, err := loadFirefoxEntries([]byte(firefoxFixture), "fixture")
	if err != nil {
		t.Fatalf("loadFirefoxEntries: %v", err)
	}

	want := []entry{
		{Name: "Example FF", URL: "https://example.org/a", Path: "Bookmarks Toolbar"},
		{Name: "Example FF dup", URL: "http://example.org/a/", Path: "Other Bookmarks"},
	}

	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, e, want[i])
		}
	}
}

func TestLoadEntriesDispatchesOnFormat(t *testing.T) {
	dir := t.TempDir()

	chromePath := filepath.Join(dir, "chrome.json")
	if err := os.WriteFile(chromePath, []byte(chromeFixture), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	entries, err := loadEntries(chromePath)
	if err != nil {
		t.Fatalf("loadEntries(chrome): %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("loadEntries(chrome) returned %d entries, want 3", len(entries))
	}

	firefoxPath := filepath.Join(dir, "firefox.json")
	if err := os.WriteFile(firefoxPath, []byte(firefoxFixture), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	entries, err = loadEntries(firefoxPath)
	if err != nil {
		t.Fatalf("loadEntries(firefox): %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("loadEntries(firefox) returned %d entries, want 2", len(entries))
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"https folds to http", "https://Example.com/page", "http://example.com/page"},
		{"trailing slash stripped", "http://example.com/page/", "http://example.com/page"},
		{"root slash kept", "http://example.com/", "http://example.com/"},
		{
			"tracking params stripped",
			"https://example.com/article?utm_source=newsletter&id=5",
			"http://example.com/article?id=5",
		},
		{
			"non-tracking params kept",
			"https://example.com/article?id=5&utm_campaign=x",
			"http://example.com/article?id=5",
		},
		{"fragment kept", "http://example.com/page#section", "http://example.com/page#section"},
		{"unparseable falls back to raw", "http://example.com/%zz", "http://example.com/%zz"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeURL(c.in)
			if got != c.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestWriteDedupedRemovesFirstOccurrenceKeepsRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Bookmarks")
	if err := os.WriteFile(path, []byte(chromeFixture), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	removed, backup, err := writeDeduped(path)
	if err != nil {
		t.Fatalf("writeDeduped: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if backup != path+".bak" {
		t.Fatalf("backup = %q, want %q", backup, path+".bak")
	}

	backupRaw, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backupRaw) != chromeFixture {
		t.Errorf("backup content changed from original")
	}

	entries, err := loadEntries(path)
	if err != nil {
		t.Fatalf("loadEntries after write: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries after dedup, want 2: %+v", len(entries), entries)
	}
	if entries[0].URL != "https://example.com/page" {
		t.Errorf("first occurrence should be kept, got %+v", entries[0])
	}
	if entries[1].URL != "https://unique.example.com/" {
		t.Errorf("unrelated entry should be untouched, got %+v", entries[1])
	}
}

func TestWriteDedupedNoDuplicatesLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Bookmarks")

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(chromeFixture), &doc); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	roots := doc["roots"].(map[string]interface{})
	// Drop the folder containing the duplicate so nothing collides.
	bar := roots["bookmark_bar"].(map[string]interface{})
	children := bar["children"].([]interface{})
	bar["children"] = children[:1]
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	removed, backup, err := writeDeduped(path)
	if err != nil {
		t.Fatalf("writeDeduped: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
	if backup != "" {
		t.Fatalf("backup = %q, want empty when nothing was removed", backup)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup file should not have been created")
	}
}

func TestFolderCounts(t *testing.T) {
	entries := []entry{
		{Name: "a", URL: "https://a.example.com/", Path: "Bookmarks Bar/To Read"},
		{Name: "b", URL: "https://b.example.com/", Path: "Bookmarks Bar/To Read"},
		{Name: "c", URL: "https://c.example.com/", Path: "Other Bookmarks"},
		{Name: "d", URL: "https://d.example.com/", Path: "Other Bookmarks"},
		{Name: "e", URL: "https://e.example.com/", Path: "Other Bookmarks"},
		{Name: "f", URL: "https://f.example.com/", Path: ""},
	}

	// count desc, path asc for ties.
	want := []folderCount{
		{Path: "Other Bookmarks", Count: 3},
		{Path: "Bookmarks Bar/To Read", Count: 2},
		{Path: "(root)", Count: 1},
	}

	got := folderCounts(entries)
	if len(got) != len(want) {
		t.Fatalf("got %d folders, want %d: %+v", len(got), len(want), got)
	}
	for i, f := range got {
		if f != want[i] {
			t.Errorf("folder %d = %+v, want %+v", i, f, want[i])
		}
	}
}

func TestWriteDedupedRejectsFirefoxFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "firefox.json")
	if err := os.WriteFile(path, []byte(firefoxFixture), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if _, _, err := writeDeduped(path); err == nil {
		t.Fatal("writeDeduped on a Firefox export should have failed")
	}
}
