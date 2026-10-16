// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type fakeTicket struct {
	ID         int
	Name       string
	Status     int
	ExternalID string
	Deleted    bool
	Urgency    int
	Impact     int
	Priority   int
	Type       int
	Entity     int
	Category   int
	Group      int
	Content    string
	Followups  int
	Solutions  int
}

// fakeGLPI serves the v1 REST API and the v2 HL API over one in-memory ticket map, behind TLS so
// the plugin's https-only URL check is exercised for real.
type fakeGLPI struct {
	t   *testing.T
	srv *httptest.Server

	mu            sync.Mutex
	tickets       map[int]*fakeTicket
	nextID        int
	sessions      map[string]bool
	sessionSeq    int
	bearers       map[string]bool
	bearerSeq     int
	lastInitAuth  string
	lastInitApp   string
	killed        int
	inits         int
	created       int
	tokenCalls    int
	lastGrant     string
	lastScope     string
	lastFilter    string
	lastCreate    map[string]any
	expiresIn     int
	activeEntity  []int
	expireSession bool
	revokeBearers bool
	looseFilter   bool
	forbidden     map[int]bool

	failTicketPost int
	multiStatus    string
}

func newFakeGLPI(t *testing.T) *fakeGLPI {
	t.Helper()
	f := &fakeGLPI{
		t:            t,
		tickets:      map[int]*fakeTicket{},
		nextID:       100,
		sessions:     map[string]bool{},
		bearers:      map[string]bool{},
		expiresIn:    3600,
		activeEntity: []int{0, 1},
	}
	f.srv = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	f.srv.StartTLS()
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGLPI) config(api string) map[string]string {
	return map[string]string{
		"name":                "glpi",
		"instance_url":        f.srv.URL,
		"api_version":         api,
		"app_token":           "AT",
		"user_token":          "UT",
		"oauth_client_id":     "CID",
		"oauth_client_secret": "CSECRET",
		"oauth_username":      "api",
		"oauth_password":      "PW",
	}
}

func (f *fakeGLPI) seed(tk fakeTicket) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	tk.ID = f.nextID
	f.tickets[tk.ID] = &tk
	return tk.ID
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("atoi %q: %v", s, err)
	}
	return n
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi0(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

func v1Err(w http.ResponseWriter, code int, vcode, msg string) {
	writeJSON(w, code, []string{vcode, msg})
}

func (f *fakeGLPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.HasPrefix(r.URL.Path, "/apirest.php/"):
		f.serveV1(w, r, strings.TrimPrefix(r.URL.Path, "/apirest.php/"))
	case r.URL.Path == "/api.php/token":
		f.serveToken(w, r)
	case strings.HasPrefix(r.URL.Path, "/api.php/v2.0/"):
		f.serveV2(w, r, strings.TrimPrefix(r.URL.Path, "/api.php/v2.0"))
	default:
		http.NotFound(w, r)
	}
}

func decodeInput(r *http.Request) map[string]any {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil
	}
	if in, ok := body["input"].(map[string]any); ok {
		return in
	}
	return body
}

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case map[string]any:
		return num(n["id"])
	}
	return 0
}

func (f *fakeGLPI) v1Ticket(tk *fakeTicket) map[string]any {
	del := 0
	if tk.Deleted {
		del = 1
	}
	return map[string]any{"id": tk.ID, "name": tk.Name, "status": tk.Status, "externalid": tk.ExternalID, "is_deleted": del, "date_mod": "2026-09-25 12:00:00"}
}

func (f *fakeGLPI) serveV1(w http.ResponseWriter, r *http.Request, p string) {
	if p == "initSession" {
		f.lastInitAuth, f.lastInitApp = r.Header.Get("Authorization"), r.Header.Get("App-Token")
		if f.lastInitAuth != "user_token UT" {
			v1Err(w, http.StatusUnauthorized, "ERROR_GLPI_LOGIN_USER_TOKEN", "parameter user_token seems invalid: "+strings.TrimPrefix(f.lastInitAuth, "user_token "))
			return
		}
		f.inits++
		f.sessionSeq++
		s := fmt.Sprintf("S%d", f.sessionSeq)
		f.sessions[s] = true
		writeJSON(w, http.StatusOK, map[string]string{"session_token": s})
		return
	}
	s := r.Header.Get("Session-Token")
	if !f.sessions[s] || r.Header.Get("App-Token") != "AT" {
		v1Err(w, http.StatusUnauthorized, "ERROR_SESSION_TOKEN_INVALID", "session_token seems invalid")
		return
	}
	if p == "killSession" {
		delete(f.sessions, s)
		f.killed++
		writeJSON(w, http.StatusOK, true)
		return
	}
	if f.expireSession {
		f.expireSession = false
		delete(f.sessions, s)
		v1Err(w, http.StatusUnauthorized, "ERROR_SESSION_TOKEN_INVALID", "session_token seems invalid")
		return
	}
	switch {
	case p == "getActiveEntities":
		ents := []map[string]int{}
		for _, id := range f.activeEntity {
			ents = append(ents, map[string]int{"id": id})
		}
		writeJSON(w, http.StatusOK, map[string]any{"active_entity": map[string]any{"id": 0, "active_entity_recursive": true, "active_entities": ents}})
	case p == "Ticket/" && r.Method == http.MethodGet:
		want := r.URL.Query().Get("searchText[externalid]")
		rows := []map[string]any{}
		for _, tk := range f.tickets {
			if want != "" && strings.Contains(tk.ExternalID, want) && !tk.Deleted {
				rows = append(rows, f.v1Ticket(tk))
			}
		}
		writeJSON(w, http.StatusOK, rows)
	case p == "Ticket/" && r.Method == http.MethodPost:
		if f.failTicketPost != 0 {
			v1Err(w, f.failTicketPost, "ERROR", "boom")
			return
		}
		if f.multiStatus != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMultiStatus)
			if _, err := w.Write([]byte(f.multiStatus)); err != nil {
				f.t.Error(err)
			}
			return
		}
		in := decodeInput(r)
		f.lastCreate = in
		id := f.createLocked(in, "externalid", "entities_id", "itilcategories_id")
		writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": ""})
	case strings.HasPrefix(p, "Ticket/"):
		id := atoi0(strings.TrimPrefix(p, "Ticket/"))
		tk := f.tickets[id]
		if tk == nil {
			v1Err(w, http.StatusNotFound, "ERROR_ITEM_NOT_FOUND", "Item not found")
			return
		}
		if f.forbidden[id] {
			v1Err(w, http.StatusForbidden, "ERROR_RIGHT_MISSING", "You don't have permission to perform this action.")
			return
		}
		if r.Method == http.MethodPut {
			tk.Status = num(decodeInput(r)["status"])
			writeJSON(w, http.StatusOK, []map[string]any{{strconv.Itoa(id): true, "message": ""}})
			return
		}
		writeJSON(w, http.StatusOK, f.v1Ticket(tk))
	case p == "ITILFollowup/" || p == "ITILSolution/":
		in := decodeInput(r)
		tk := f.tickets[num(in["items_id"])]
		if tk == nil || in["itemtype"] != "Ticket" {
			v1Err(w, http.StatusBadRequest, "ERROR_GLPI_ADD", "bad parent")
			return
		}
		f.timelineLocked(w, tk, p == "ITILSolution/", func() { v1Err(w, http.StatusBadRequest, "ERROR_GLPI_ADD", "A solution already exists") })
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGLPI) createLocked(in map[string]any, extKey, entKey, catKey string) int {
	f.created++
	f.nextID++
	tk := &fakeTicket{
		ID: f.nextID, Status: 1,
		Urgency: num(in["urgency"]), Impact: num(in["impact"]), Priority: num(in["priority"]), Type: num(in["type"]),
		Entity: num(in[entKey]), Category: num(in[catKey]), Group: num(in["_groups_id_assign"]),
	}
	tk.Name, _ = in["name"].(string)
	tk.Content, _ = in["content"].(string)
	tk.ExternalID, _ = in[extKey].(string)
	f.tickets[tk.ID] = tk
	return tk.ID
}

func (f *fakeGLPI) timelineLocked(w http.ResponseWriter, tk *fakeTicket, solution bool, refuse func()) {
	if solution {
		if tk.Status >= 5 {
			refuse()
			return
		}
		tk.Solutions++
		tk.Status = 5
	} else {
		tk.Followups++
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": 1})
}

func (f *fakeGLPI) serveToken(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	f.tokenCalls++
	f.lastGrant, f.lastScope = in["grant_type"], in["scope"]
	if in["client_id"] != "CID" || in["client_secret"] != "CSECRET" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client", "error_description": "Client authentication failed"})
		return
	}
	if in["username"] != "api" || in["password"] != "PW" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "The user credentials were incorrect."})
		return
	}
	f.bearerSeq++
	tok := fmt.Sprintf("B%d", f.bearerSeq)
	f.bearers[tok] = true
	writeJSON(w, http.StatusOK, map[string]any{"token_type": "Bearer", "access_token": tok, "expires_in": f.expiresIn})
}

func (f *fakeGLPI) v2Ticket(tk *fakeTicket) map[string]any {
	return map[string]any{"id": tk.ID, "name": tk.Name, "status": map[string]any{"id": tk.Status, "name": "x"}, "external_id": tk.ExternalID, "is_deleted": tk.Deleted, "date_mod": "2026-09-25T12:00:00+00:00"}
}

func hlErr(w http.ResponseWriter, code int, title string) {
	writeJSON(w, code, map[string]any{"status": "ERROR", "title": title, "detail": nil})
}

func (f *fakeGLPI) serveV2(w http.ResponseWriter, r *http.Request, p string) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if f.revokeBearers {
		f.revokeBearers = false
		f.bearers = map[string]bool{}
	}
	if !f.bearers[tok] {
		hlErr(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 2 || parts[0] != "Assistance" || parts[1] != "Ticket" {
		hlErr(w, http.StatusNotFound, "Not found")
		return
	}
	if len(parts) == 2 {
		if r.Method == http.MethodPost {
			in := decodeInput(r)
			f.lastCreate = in
			id := f.createLocked(in, "external_id", "entity", "category")
			w.Header().Set("Location", fmt.Sprintf("/Assistance/Ticket/%d", id))
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "href": fmt.Sprintf("/Assistance/Ticket/%d", id)})
			return
		}
		f.lastFilter = r.URL.Query().Get("filter")
		var ext string
		for _, clause := range strings.Split(f.lastFilter, ";") {
			if v, ok := strings.CutPrefix(clause, "external_id=="); ok {
				ext = v
			}
		}
		rows := []map[string]any{}
		for _, tk := range f.tickets {
			if f.looseFilter && strings.Contains(tk.ExternalID, ext) || (ext == "" || tk.ExternalID == ext) && tk.Status < 5 {
				rows = append(rows, f.v2Ticket(tk))
			}
		}
		writeJSON(w, http.StatusOK, rows)
		return
	}
	id := atoi0(parts[2])
	tk := f.tickets[id]
	if tk == nil {
		hlErr(w, http.StatusNotFound, "Not found")
		return
	}
	switch {
	case len(parts) == 3 && r.Method == http.MethodPatch:
		st, ok := decodeInput(r)["status"].(map[string]any)
		if !ok {
			hlErr(w, http.StatusBadRequest, "status must be an object")
			return
		}
		tk.Status = num(st["id"])
		writeJSON(w, http.StatusOK, f.v2Ticket(tk))
	case len(parts) == 3:
		writeJSON(w, http.StatusOK, f.v2Ticket(tk))
	case len(parts) == 5 && parts[3] == "Timeline" && (parts[4] == "Followup" || parts[4] == "Solution"):
		f.timelineLocked(w, tk, parts[4] == "Solution", func() { hlErr(w, http.StatusBadRequest, "A solution already exists") })
	default:
		hlErr(w, http.StatusNotFound, "Not found")
	}
}
