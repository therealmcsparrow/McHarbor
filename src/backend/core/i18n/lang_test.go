// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

import "testing"

func TestParseAcceptLanguageSupportsAdditionalLocales(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   Lang
	}{
		{name: "spanish region", header: "es-MX,es;q=0.9,en;q=0.8", want: ES},
		{name: "french", header: "fr-FR,fr;q=0.9,en;q=0.8", want: FR},
		{name: "portuguese brazil", header: "pt-BR,pt;q=0.9,en;q=0.8", want: PT},
		{name: "simplified chinese", header: "zh-CN,zh;q=0.9,en;q=0.8", want: ZH},
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

func TestAdditionalLocaleMessagesAreRegistered(t *testing.T) {
	tests := []struct {
		lang Lang
		want string
	}{
		{lang: ES, want: "No autorizado"},
		{lang: FR, want: "Non autorisé"},
		{lang: PT, want: "Não autorizado"},
		{lang: ZH, want: "未经授权"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			if got := T(tt.lang, ErrUnauthorized); got != tt.want {
				t.Fatalf("T(%q, ErrUnauthorized) = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}
