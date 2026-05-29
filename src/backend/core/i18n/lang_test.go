// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestSupportedReturnsConfiguredLanguages(t *testing.T) {
	want := []Lang{EN, NL, DE, ES, FR, PT}
	if got := Supported(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Supported() = %#v, want %#v", got, want)
	}
}

func TestParseAcceptLanguageSupportsAdditionalLocales(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   Lang
	}{
		{name: "empty", header: "", want: Default},
		{name: "uppercase base", header: "DE", want: DE},
		{name: "with quality", header: "nl-NL;q=0.7,en;q=0.8", want: NL},
		{name: "trimmed", header: "  fr-FR  ", want: FR},
		{name: "spanish region", header: "es-MX,es;q=0.9,en;q=0.8", want: ES},
		{name: "french", header: "fr-FR,fr;q=0.9,en;q=0.8", want: FR},
		{name: "portuguese brazil", header: "pt-BR,pt;q=0.9,en;q=0.8", want: PT},
		{name: "simplified chinese fallback", header: "zh-CN,zh;q=0.9,en;q=0.8", want: Default},
		{name: "fallback", header: "it-IT,it;q=0.9", want: Default},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseAcceptLanguage(tt.header); got != tt.want {
				t.Fatalf("ParseAcceptLanguage(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestIsSupported(t *testing.T) {
	if !IsSupported(EN) {
		t.Fatal("EN should be supported")
	}
	if IsSupported(Lang("it")) {
		t.Fatal("it should not be supported")
	}
}

func TestContextLanguageHelpers(t *testing.T) {
	if got := FromContext(context.Background()); got != Default {
		t.Fatalf("FromContext(empty) = %q, want %q", got, Default)
	}

	ctx := WithLang(context.Background(), PT)
	if got := FromContext(ctx); got != PT {
		t.Fatalf("FromContext(ctx) = %q, want %q", got, PT)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	if got := FromRequest(req); got != PT {
		t.Fatalf("FromRequest(req) = %q, want %q", got, PT)
	}
}

func TestMiddlewareStoresResolvedLanguage(t *testing.T) {
	var got Lang
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	rec := httptest.NewRecorder()

	Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got != Default {
		t.Fatalf("middleware language = %q, want %q", got, Default)
	}
}

func TestAdditionalLocaleMessagesAreRegistered(t *testing.T) {
	tests := []struct {
		lang Lang
	}{
		{lang: ES},
		{lang: FR},
		{lang: PT},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			if got := T(tt.lang, ErrUnauthorized); got == "" || got == string(ErrUnauthorized) {
				t.Fatalf("T(%q, ErrUnauthorized) = %q, want translated message", tt.lang, got)
			}
		})
	}
}

func TestMessageFallbacks(t *testing.T) {
	if got := T(Lang("it"), ErrUnauthorized); got != messagesEN[ErrUnauthorized] {
		t.Fatalf("T(unsupported, ErrUnauthorized) = %q, want English fallback", got)
	}
	if got := T(EN, MsgCode("missing.code")); got != "missing.code" {
		t.Fatalf("T(EN, missing.code) = %q, want code string", got)
	}
}

func TestTfFormatsTranslatedMessage(t *testing.T) {
	code := MsgCode("test.format")
	previous, hadPrevious := messagesEN[code]
	messagesEN[code] = "hello %s %d"
	defer func() {
		if !hadPrevious {
			delete(messagesEN, code)
			return
		}
		messagesEN[code] = previous
	}()

	if got := Tf(EN, code, "world", 7); got != "hello world 7" {
		t.Fatalf("Tf(EN, test.format) = %q", got)
	}
}
