// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
)

func TestStateHidesAPrereleaseTheSwitchNoLongerAllows(t *testing.T) {
	h, database := channelsHandler(t)
	checker := NewSelfUpdateChecker(database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetSelfUpdateChecker(checker)
	checker.mu.Lock()
	checker.state = SelfUpdateState{
		CurrentVersion: "2026.11.01", LatestVersion: "2026.11.02-rc.1", UpdateAvailable: true,
		LatestIsPrerelease: true, ReleaseURL: "https://example.invalid/r", ReleaseNotes: "notes",
	}
	checker.mu.Unlock()

	read := func() SelfUpdateState {
		t.Helper()
		rec := httptest.NewRecorder()
		h.HandleState(rec, httptest.NewRequest(http.MethodGet, "/api/updates/state", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		var env struct {
			Data SelfUpdateState `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return env.Data
	}

	on := true
	if _, _, err := updatechannel.Apply(context.Background(), database, updatechannel.Patch{McHarborPrereleases: &on}); err != nil {
		t.Fatal(err)
	}
	if st := read(); st.LatestVersion != "2026.11.02-rc.1" || !st.UpdateAvailable {
		t.Fatalf("with the switch on the offer stays: %+v", st)
	}
	off := false
	if _, _, err := updatechannel.Apply(context.Background(), database, updatechannel.Patch{McHarborPrereleases: &off}); err != nil {
		t.Fatal(err)
	}
	st := read()
	if st.LatestVersion != "" || st.UpdateAvailable || st.LatestIsPrerelease || st.ReleaseURL != "" || st.ReleaseNotes != "" {
		t.Fatalf("a prerelease is still offered with the switch off: %+v", st)
	}
	if st.CurrentVersion != "2026.11.01" {
		t.Fatalf("the current version must survive: %+v", st)
	}
}
