// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"testing"
)

// TestValidateImageRequestWrapsSentinel guards HandleCreateImage, which maps a
// refused request to a 400 with errors.Is instead of matching the message text.
func TestValidateImageRequestWrapsSentinel(t *testing.T) {
	cases := map[string]ImageCreateRequest{
		"kind":     {Kind: "floppy", DisplayName: "x", SourceURL: "https://example.com/a.img"},
		"name":     {Kind: "iso", SourceURL: "https://example.com/a.iso"},
		"url":      {Kind: "iso", DisplayName: "x", SourceURL: "not a url"},
		"insecure": {Kind: "iso", DisplayName: "x", SourceURL: "http://example.com/a.iso"},
	}
	for name, req := range cases {
		if err := validateImageRequest(req); !errors.Is(err, ErrInvalidImage) {
			t.Errorf("%s: got %v, want ErrInvalidImage", name, err)
		}
	}
	if err := validateImageRequest(ImageCreateRequest{Kind: "iso", DisplayName: "x", SourceURL: "https://example.com/a.iso"}); err != nil {
		t.Errorf("valid request refused: %v", err)
	}
}
