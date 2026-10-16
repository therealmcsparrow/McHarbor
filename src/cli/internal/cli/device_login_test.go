// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func deviceServer(t *testing.T, answers []func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	polls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/device/code":
			if _, err := w.Write([]byte(`{"success":true,"data":{"deviceCode":"secret","userCode":"BCDF-GHJK","verificationPath":"/device","expiresIn":600,"interval":10}}`)); err != nil {
				t.Error(err)
			}
		case "/api/device/token":
			if polls >= len(answers) {
				t.Fatalf("polled %d times, more than the script", polls+1)
			}
			answers[polls](w)
			polls++
		default:
			http.NotFound(w, r)
		}
	}))
}

func answer(t *testing.T, status int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}
}

func runFlow(t *testing.T, srv *httptest.Server) (string, []time.Duration, string, error) {
	t.Helper()
	a, err := newAPI(&Target{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var slept []time.Duration
	flow := &deviceFlow{api: a, out: &out, origin: srv.URL, sleep: func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}}
	key, err := flow.run(context.Background())
	return key, slept, out.String(), err
}

func TestDeviceLoginWaitsBacksOffAndReturnsTheKey(t *testing.T) {
	srv := deviceServer(t, []func(http.ResponseWriter){
		answer(t, 400, `{"success":false,"code":"err.auth.required","data":{"error":"authorization_pending"}}`),
		answer(t, 429, `{"success":false,"code":"err.rate_limit_exceeded","error":"Too many requests"}`),
		answer(t, 429, `{"success":false,"code":"err.rate_limit_exceeded","data":{"error":"slow_down"}}`),
		answer(t, 200, `{"success":true,"data":{"id":"k1","key":"mch_new"}}`),
	})
	defer srv.Close()

	key, slept, out, err := runFlow(t, srv)
	if err != nil || key != "mch_new" {
		t.Fatalf("key %q, err %v", key, err)
	}
	if !strings.Contains(out, srv.URL+"/device?code=BCDF-GHJK") || !strings.Contains(out, "BCDF-GHJK") {
		t.Fatalf("the user is not told where to approve: %q", out)
	}
	want := []time.Duration{10 * time.Second, 10 * time.Second, 15 * time.Second, 20 * time.Second}
	if len(slept) != len(want) {
		t.Fatalf("slept %v, want %v", slept, want)
	}
	for i := range want {
		if slept[i] != want[i] {
			t.Fatalf("slept %v, want %v: a bare 429 and slow_down must both lengthen the interval", slept, want)
		}
	}
}

func TestDeviceLoginStopsWithTheRightExitCode(t *testing.T) {
	cases := map[string]struct {
		body string
		code int
	}{
		"denied":  {`{"success":false,"code":"err.forbidden","data":{"error":"access_denied"}}`, ExitForbidden},
		"expired": {`{"success":false,"code":"err.auth.required","data":{"error":"expired_token"}}`, ExitTimeout},
		"invalid": {`{"success":false,"code":"err.invalid_body","data":{"error":"invalid_grant"}}`, ExitAuth},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := deviceServer(t, []func(http.ResponseWriter){answer(t, 400, c.body)})
			defer srv.Close()
			_, _, _, err := runFlow(t, srv)
			var e *Error
			if !errors.As(err, &e) || e.Code != c.code {
				t.Fatalf("err = %v, want exit code %d", err, c.code)
			}
		})
	}
}

func TestDeviceLoginOpensTheApprovalPageAndSurvivesNoBrowser(t *testing.T) {
	for _, openErr := range []error{nil, errors.New("no display")} {
		srv := deviceServer(t, []func(http.ResponseWriter){answer(t, 200, `{"success":true,"data":{"id":"k1","key":"mch_new"}}`)})
		a, err := newAPI(&Target{URL: srv.URL})
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		var opened []string
		flow := &deviceFlow{api: a, out: &out, origin: srv.URL,
			sleep: func(context.Context, time.Duration) error { return nil },
			open:  func(u string) error { opened = append(opened, u); return openErr }}
		key, err := flow.run(context.Background())
		srv.Close()
		if err != nil || key != "mch_new" {
			t.Fatalf("open error %v: key %q, err %v", openErr, key, err)
		}
		if len(opened) != 1 || opened[0] != srv.URL+"/device?code=BCDF-GHJK" {
			t.Fatalf("opened %v", opened)
		}
		if openErr != nil && !strings.Contains(out.String(), "Open the link yourself") {
			t.Fatalf("a failed open was not explained: %q", out.String())
		}
	}
}
