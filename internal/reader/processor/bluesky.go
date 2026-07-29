// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package processor // import "miniflux.app/v2/internal/reader/processor"

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/reader/fetcher"
	"miniflux.app/v2/internal/reader/rewrite"
	"miniflux.app/v2/internal/urllib"
)

// blueskyImagesRuleName is the opt-in rewrite rule enabling this processor.
const blueskyImagesRuleName = "add_bluesky_images"

// Bluesky RSS items contain no media markup whatsoever: no <enclosure>, no
// media namespace. Post images exist only as <meta property="og:image"> tags on
// the entry web page, so recovering them costs one request per new entry, which
// is why this is opt-in per feed rather than always on.
//
// The CDN serves variants under /img/<variant>/plain/<did>/<cid>. Only the
// "feed_" variants are post images: a text-only post advertises the author
// avatar instead, which must never become an enclosure.
const (
	blueskyThumbnailPrefix = "https://cdn.bsky.app/img/feed_thumbnail/"
	blueskyFullsizePrefix  = "https://cdn.bsky.app/img/feed_fullsize/"
)

func shouldFetchBlueskyImages(feed *model.Feed, entry *model.Entry) bool {
	if !rewrite.HasRule(feed.RewriteRules, blueskyImagesRuleName) {
		return false
	}

	// Guard against the rule being enabled on a feed aggregating other sites.
	return urllib.DomainWithoutWWW(entry.URL) == "bsky.app"
}

func fetchBlueskyImages(requestBuilder *fetcher.RequestBuilder, entry *model.Entry) error {
	responseHandler := fetcher.NewResponseHandler(requestBuilder.ExecuteRequest(entry.URL))
	defer responseHandler.Close()

	if localizedError := responseHandler.LocalizedError(); localizedError != nil {
		return localizedError.Error()
	}

	doc, docErr := goquery.NewDocumentFromReader(responseHandler.Body(config.Opts.HTTPClientMaxBodySize()))
	if docErr != nil {
		return docErr
	}

	appendBlueskyImageEnclosures(entry, doc)
	return nil
}

// appendBlueskyImageEnclosures appends the post images advertised by the
// document, skipping URLs the entry already carries.
func appendBlueskyImageEnclosures(entry *model.Entry, doc *goquery.Document) {
	duplicates := make(map[string]bool, len(entry.Enclosures))
	for _, enclosure := range entry.Enclosures {
		duplicates[enclosure.URL] = true
	}

	doc.Find(`meta[property="og:image"]`).Each(func(_ int, selection *goquery.Selection) {
		content, exists := selection.Attr("content")
		if !exists {
			return
		}

		imageURL, ok := blueskyFullsizeImageURL(strings.TrimSpace(content))
		if !ok || duplicates[imageURL] {
			return
		}

		duplicates[imageURL] = true

		entry.Enclosures = append(entry.Enclosures, &model.Enclosure{
			URL: imageURL,
			// The URL has no file extension, so IsImage() depends entirely on
			// the MIME type being set here.
			MimeType: "image/jpeg",
		})
	})
}

// blueskyFullsizeImageURL keeps only post images and upgrades them to the
// full-size variant. The "@jpeg" suffix pins the encoding: without it the CDN
// content-negotiates and may answer with WebP, contradicting the MIME type
// stored on the enclosure.
func blueskyFullsizeImageURL(rawURL string) (string, bool) {
	switch {
	case strings.HasPrefix(rawURL, blueskyFullsizePrefix):
	case strings.HasPrefix(rawURL, blueskyThumbnailPrefix):
		rawURL = blueskyFullsizePrefix + strings.TrimPrefix(rawURL, blueskyThumbnailPrefix)
	default:
		// Avatars, link-card previews and anything not served by the CDN.
		return "", false
	}

	// The last segment is the content identifier, optionally already suffixed
	// with a format specifier.
	lastSegment := rawURL[strings.LastIndexByte(rawURL, '/')+1:]
	if lastSegment == "" {
		return "", false
	}

	if !strings.ContainsRune(lastSegment, '@') {
		rawURL += "@jpeg"
	}

	return rawURL, true
}
