// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

import "net/http"

// Middleware parses the Accept-Language header and stores the resolved Lang in context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := ParseAcceptLanguage(r.Header.Get("Accept-Language"))
		ctx := WithLang(r.Context(), lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
