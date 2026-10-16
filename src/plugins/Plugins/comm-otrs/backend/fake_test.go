// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func init() { allowHTTP = true }

type fakeGI struct {
	mu        sync.Mutex
	logins    int
	sessionOK string
	calls     []string
	bodies    []map[string]any
	handlers  map[string]func(body map[string]any) any
}

func (f *fakeGI) locked(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *fakeGI) count(key string) int {
	n := 0
	f.locked(func() {
		for _, c := range f.calls {
			if c == key {
				n++
			}
		}
	})
	return n
}

func newFakeGI(t *testing.T) (*fakeGI, *httptest.Server) {
	f := &fakeGI{sessionOK: "S1", handlers: map[string]func(map[string]any) any{}}
	f.handlers["POST /Session"] = func(b map[string]any) any {
		f.logins++
		if b["UserLogin"] != "mcharbor" || b["Password"] != "pw" {
			return map[string]any{"Error": map[string]any{"ErrorCode": "SessionCreate.AuthFail", "ErrorMessage": "SessionCreate: Authorization failing!"}}
		}
		return map[string]any{"SessionID": f.sessionOK}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/otrs/nph-genericinterface.pl/Webservice/McHarborConnectorREST"
		if !strings.HasPrefix(r.URL.Path, prefix) || r.URL.RawQuery != "" {
			http.Error(w, "bad route", http.StatusNotFound)
			return
		}
		route := strings.TrimPrefix(r.URL.Path, prefix)
		if strings.HasPrefix(route, "/Ticket/") {
			route = "/Ticket/:id"
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		f.mu.Lock()
		key := r.Method + " " + route
		f.calls = append(f.calls, key)
		f.bodies = append(f.bodies, body)
		h, ok := f.handlers[key]
		if ok && key != "POST /Session" && body["SessionID"] != f.sessionOK {
			h = func(map[string]any) any {
				return map[string]any{"Error": map[string]any{"ErrorCode": "TicketCreate.AuthFail", "ErrorMessage": "TicketCreate: Authorization failing!"}}
			}
		}
		var resp any
		if ok {
			resp = h(body)
		}
		f.mu.Unlock()
		if !ok {
			http.Error(w, "no route", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func testConfig(url string) map[string]string {
	return map[string]string{
		"instance_url": url, "flavour": "znuny", "webservice": "McHarborConnectorREST",
		"user_login": "mcharbor", "password": "pw", "default_assignment_group": "Raw",
		"customer_user": "mcharbor@example.com", "dedup_field": "McHarborDedupKey",
	}
}
