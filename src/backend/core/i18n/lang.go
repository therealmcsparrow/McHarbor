// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

import (
	"context"
	"strings"
)

// Lang represents a supported language.
type Lang string

const (
	EN Lang = "en"
	NL Lang = "nl"
	DE Lang = "de"
	ES Lang = "es"
	FR Lang = "fr"
	PT Lang = "pt"
	ZH Lang = "zh"
)

// Default is the fallback language.
const Default = EN

type ctxKey struct{}

// supported holds all available languages.
var supported = []Lang{EN, NL, DE, ES, FR, PT, ZH}

// Supported returns the list of supported languages.
func Supported() []Lang { return supported }

// IsSupported returns true if lang is a supported language.
func IsSupported(lang Lang) bool {
	for _, l := range supported {
		if l == lang {
			return true
		}
	}
	return false
}

// ParseAcceptLanguage extracts the best matching Lang from an Accept-Language header value.
// Falls back to Default if no match is found.
func ParseAcceptLanguage(header string) Lang {
	if header == "" {
		return Default
	}

	// Parse tags in priority order (Accept-Language is already quality-sorted by convention,
	// but browsers usually send highest quality first).
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(part)
		// Strip quality value (e.g., "en-US;q=0.9" → "en-US")
		if idx := strings.IndexByte(tag, ';'); idx >= 0 {
			tag = strings.TrimSpace(tag[:idx])
		}
		// Normalize to lowercase base language
		tag = strings.ToLower(tag)
		if idx := strings.IndexByte(tag, '-'); idx >= 0 {
			tag = tag[:idx]
		}
		lang := Lang(tag)
		if IsSupported(lang) {
			return lang
		}
	}

	return Default
}

// WithLang stores a Lang in the context.
func WithLang(ctx context.Context, lang Lang) context.Context {
	return context.WithValue(ctx, ctxKey{}, lang)
}

// FromContext retrieves the Lang from context, defaulting to EN.
func FromContext(ctx context.Context) Lang {
	if lang, ok := ctx.Value(ctxKey{}).(Lang); ok {
		return lang
	}
	return Default
}

// FromRequest is a convenience helper that extracts Lang from an HTTP request's context.
func FromRequest(r interface{ Context() context.Context }) Lang {
	return FromContext(r.Context())
}
