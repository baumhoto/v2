// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package processor // import "miniflux.app/v2/internal/reader/processor"

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"miniflux.app/v2/internal/model"
)

func TestBlueskyFullsizeImageURL(t *testing.T) {
	scenarios := []struct {
		name     string
		input    string
		expected string
		accepted bool
	}{
		{
			name:     "thumbnail is upgraded to fullsize and pinned to jpeg",
			input:    "https://cdn.bsky.app/img/feed_thumbnail/plain/did:plc:z72i/bafkreibmn42rzh",
			expected: "https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/bafkreibmn42rzh@jpeg",
			accepted: true,
		},
		{
			name:     "fullsize is kept and pinned to jpeg",
			input:    "https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/bafkreibmn42rzh",
			expected: "https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/bafkreibmn42rzh@jpeg",
			accepted: true,
		},
		{
			name:     "an existing format specifier is preserved",
			input:    "https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/bafkreibmn42rzh@png",
			expected: "https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/bafkreibmn42rzh@png",
			accepted: true,
		},
		{
			// Text-only posts advertise the author avatar as og:image.
			name:  "avatar is rejected",
			input: "https://cdn.bsky.app/img/avatar_thumbnail/plain/did:plc:z72i/bafkreihwihm6kp",
		},
		{
			name:  "avatar fullsize is rejected",
			input: "https://cdn.bsky.app/img/avatar/plain/did:plc:z72i/bafkreihwihm6kp",
		},
		{
			name:  "site logo is rejected",
			input: "https://web-cdn.bsky.app/static/favicon.png",
		},
		{
			name:  "another host reusing the path is rejected",
			input: "https://evil.example.org/img/feed_fullsize/plain/did:plc:z72i/bafkreibmn42rzh",
		},
		{
			name:  "missing content identifier is rejected",
			input: "https://cdn.bsky.app/img/feed_fullsize/",
		},
		{
			name:  "empty input is rejected",
			input: "",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			result, ok := blueskyFullsizeImageURL(scenario.input)
			if ok != scenario.accepted {
				t.Fatalf("blueskyFullsizeImageURL(%q) accepted = %v, want %v", scenario.input, ok, scenario.accepted)
			}
			if result != scenario.expected {
				t.Errorf("blueskyFullsizeImageURL(%q) = %q, want %q", scenario.input, result, scenario.expected)
			}
		})
	}
}

func TestAppendBlueskyImageEnclosures(t *testing.T) {
	page := `<html><head>
		<meta property="og:logo" content="https://web-cdn.bsky.app/static/favicon.png">
		<meta property="og:image" content="https://cdn.bsky.app/img/feed_thumbnail/plain/did:plc:z72i/aaa">
		<meta property="og:image" content="https://cdn.bsky.app/img/feed_thumbnail/plain/did:plc:z72i/bbb">
		<meta property="og:image" content="https://cdn.bsky.app/img/avatar_thumbnail/plain/did:plc:z72i/ccc">
		<meta property="og:title" content="Bluesky">
	</head><body></body></html>`

	entry := &model.Entry{}
	appendBlueskyImageEnclosures(entry, parseTestDocument(t, page))

	expected := []string{
		"https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/aaa@jpeg",
		"https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/bbb@jpeg",
	}

	if len(entry.Enclosures) != len(expected) {
		t.Fatalf("got %d enclosures, want %d: %v", len(entry.Enclosures), len(expected), entry.Enclosures)
	}

	for i, url := range expected {
		if entry.Enclosures[i].URL != url {
			t.Errorf("enclosure %d URL = %q, want %q", i, entry.Enclosures[i].URL, url)
		}
		if entry.Enclosures[i].MimeType != "image/jpeg" {
			t.Errorf("enclosure %d MIME type = %q, want image/jpeg", i, entry.Enclosures[i].MimeType)
		}
		if !entry.Enclosures[i].IsImage() {
			t.Errorf("enclosure %d is not detected as an image", i)
		}
	}
}

func TestAppendBlueskyImageEnclosuresForTextOnlyPost(t *testing.T) {
	page := `<html><head>
		<meta property="og:image" content="https://cdn.bsky.app/img/avatar_thumbnail/plain/did:plc:z72i/ccc">
	</head><body></body></html>`

	entry := &model.Entry{}
	appendBlueskyImageEnclosures(entry, parseTestDocument(t, page))

	if len(entry.Enclosures) != 0 {
		t.Errorf("text-only post produced %d enclosures, want 0: %v", len(entry.Enclosures), entry.Enclosures)
	}
}

func TestAppendBlueskyImageEnclosuresSkipsDuplicates(t *testing.T) {
	page := `<html><head>
		<meta property="og:image" content="https://cdn.bsky.app/img/feed_thumbnail/plain/did:plc:z72i/aaa">
		<meta property="og:image" content="https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/aaa">
	</head><body></body></html>`

	entry := &model.Entry{
		Enclosures: model.EnclosureList{
			{URL: "https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:z72i/aaa@jpeg", MimeType: "image/jpeg"},
		},
	}
	appendBlueskyImageEnclosures(entry, parseTestDocument(t, page))

	if len(entry.Enclosures) != 1 {
		t.Errorf("got %d enclosures, want 1: %v", len(entry.Enclosures), entry.Enclosures)
	}
}

func TestShouldFetchBlueskyImages(t *testing.T) {
	scenarios := []struct {
		name         string
		rewriteRules string
		entryURL     string
		expected     bool
	}{
		{
			name:         "rule enabled on a Bluesky entry",
			rewriteRules: "add_bluesky_images",
			entryURL:     "https://bsky.app/profile/bsky.app/post/3mnslrkd6ok2g",
			expected:     true,
		},
		{
			name:         "rule combined with other rules",
			rewriteRules: `nl2br,add_bluesky_images,replace("a"|"b")`,
			entryURL:     "https://bsky.app/profile/bsky.app/post/3mnslrkd6ok2g",
			expected:     true,
		},
		{
			name:         "rule not enabled",
			rewriteRules: "nl2br",
			entryURL:     "https://bsky.app/profile/bsky.app/post/3mnslrkd6ok2g",
			expected:     false,
		},
		{
			name:         "no rules at all",
			rewriteRules: "",
			entryURL:     "https://bsky.app/profile/bsky.app/post/3mnslrkd6ok2g",
			expected:     false,
		},
		{
			name:         "rule enabled but entry is hosted elsewhere",
			rewriteRules: "add_bluesky_images",
			entryURL:     "https://example.org/post/1",
			expected:     false,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			feed := &model.Feed{RewriteRules: scenario.rewriteRules}
			entry := &model.Entry{URL: scenario.entryURL}

			if result := shouldFetchBlueskyImages(feed, entry); result != scenario.expected {
				t.Errorf("shouldFetchBlueskyImages() = %v, want %v", result, scenario.expected)
			}
		})
	}
}

func parseTestDocument(t *testing.T, html string) *goquery.Document {
	t.Helper()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("unable to parse test document: %v", err)
	}
	return doc
}
