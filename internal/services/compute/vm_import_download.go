// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"fmt"
	"strings"

	"github.com/verge-io/govergeos"
)

// downloadDescription stores the source address where a later import can read
// it. files.url is argument and readonly, and a files GET does not return it.
func downloadDescription(prefix, name, rawURL string) (string, error) {
	if strings.ContainsAny(name, "\r\n") {
		return "", fmt.Errorf("name cannot contain a line break")
	}
	rawURL = strings.TrimSpace(rawURL)
	if strings.ContainsAny(rawURL, "\r\n") {
		return "", fmt.Errorf("url cannot contain a line break")
	}
	if err := validateImportURL(rawURL); err != nil {
		return "", err
	}
	return prefix + name + "\n" + rawURL, nil
}

type downloadRecord struct {
	name             string
	rawURL           string
	urlInDescription bool
	marker           bool
}

func parseDownloadDescription(description, prefix string) downloadRecord {
	description = strings.ReplaceAll(description, "\r\n", "\n")
	first, rest, hasRest := strings.Cut(description, "\n")
	first = strings.TrimRight(first, "\r")
	if !strings.HasPrefix(first, prefix) {
		return downloadRecord{}
	}
	name := first[len(prefix):]
	if name == "" {
		return downloadRecord{}
	}
	rec := downloadRecord{name: name, marker: true}
	if !hasRest {
		return rec
	}
	line, _, _ := strings.Cut(rest, "\n")
	rawURL := strings.TrimSpace(strings.TrimRight(line, "\r"))
	if validateImportURL(rawURL) != nil {
		return rec
	}
	rec.rawURL = rawURL
	rec.urlInDescription = true
	return rec
}

func downloadOwned(description, prefix, name string) bool {
	rec := parseDownloadDescription(description, prefix)
	return rec.marker && rec.name == name
}

func sourceURLFromFile(file *vergeos.File, prefix, name string) (string, bool) {
	if file == nil {
		return "", false
	}
	rec := parseDownloadDescription(file.Description, prefix)
	if !rec.marker || rec.name != name {
		return "", false
	}
	if rec.urlInDescription {
		return rec.rawURL, true
	}
	if stored := strings.TrimSpace(file.URL); validateImportURL(stored) == nil {
		return stored, true
	}
	return "", false
}
