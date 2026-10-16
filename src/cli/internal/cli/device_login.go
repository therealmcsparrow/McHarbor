// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"time"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

// deviceFlow is the device sign-in's dependencies, separated so tests can run
// it against a fake server without waiting real seconds.
type deviceFlow struct {
	api    *API
	out    io.Writer
	sleep  func(ctx context.Context, d time.Duration) error
	origin string
	// open shows the approval page; nil when the user asked not to.
	open func(url string) error
}

type deviceStart struct {
	DeviceCode       string `json:"deviceCode"`
	UserCode         string `json:"userCode"`
	VerificationPath string `json:"verificationPath"`
	ExpiresIn        int    `json:"expiresIn"`
	Interval         int    `json:"interval"`
}

type deviceKey struct {
	Key string `json:"key"`
}

// openBrowser starts the platform's URL handler without waiting for it. On
// Linux without a display there is nothing to open, so it says so.
func openBrowser(ctx context.Context, url string) error {
	// The handler is started without being waited on and may outlive the login
	// flow, so it inherits the values but not the cancellation.
	ctx = context.WithoutCancel(ctx)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return errors.New("no display")
		}
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			return
		}
	}()
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// run asks the server for a code, tells the user where to approve it, and
// polls until the server hands over the new key or refuses.
func (f *deviceFlow) run(ctx context.Context) (string, error) {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	osUser := ""
	if u, err := user.Current(); err == nil {
		osUser = u.Username
	}
	resp, err := f.api.c.DeviceauthStart(ctx, client.DeviceauthStartJSONRequestBody{Host: host, OsUser: osUser})
	raw, err := f.api.do("deviceauthStart", resp, err)
	if err != nil {
		return "", err
	}
	var start deviceStart
	if err := json.Unmarshal(raw, &start); err != nil || start.DeviceCode == "" {
		return "", &Error{Code: ExitRemote, Operation: "deviceauthStart", Message: "the server did not return a device code"}
	}

	link := f.origin + start.VerificationPath + "?code=" + url.QueryEscape(start.UserCode)
	emitf(f.out, "Open %s\nand confirm the code %s. Waiting for approval (Ctrl-C to cancel)...\n", link, start.UserCode)
	if f.open != nil {
		if err := f.open(link); err != nil {
			emitf(f.out, "(Could not open a browser: %v. Open the link yourself.)\n", err)
		}
	}

	interval := time.Duration(start.Interval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	deadline := time.Duration(start.ExpiresIn) * time.Second
	for waited := time.Duration(0); waited <= deadline; waited += interval {
		if err := f.sleep(ctx, interval); err != nil {
			return "", &Error{Code: ExitFailure, Operation: "deviceauthToken", Message: "cancelled"}
		}
		key, slowDown, err := f.poll(ctx, start.DeviceCode)
		if err != nil {
			return "", err
		}
		if key != "" {
			return key, nil
		}
		if slowDown {
			interval += 5 * time.Second
		}
	}
	return "", &Error{Code: ExitTimeout, Operation: "deviceauthToken", Message: "the code expired before it was approved; run login --device again"}
}

// poll makes one token request. A 429 from the auth rate limiter is treated
// like slow_down, because both mean the same thing to a patient client.
func (f *deviceFlow) poll(ctx context.Context, deviceCode string) (key string, slowDown bool, err error) {
	resp, err := f.api.c.DeviceauthToken(ctx, client.DeviceauthTokenJSONRequestBody{DeviceCode: deviceCode})
	if err != nil {
		return "", false, transportError("deviceauthToken", err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, transportError("deviceauthToken", err)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", false, apiError("deviceauthToken", resp, body)
	}
	if resp.StatusCode == http.StatusOK {
		var k deviceKey
		if err := json.Unmarshal(env.Data, &k); err != nil || k.Key == "" {
			return "", false, &Error{Code: ExitRemote, Operation: "deviceauthToken", Message: "the server approved the sign-in but returned no key"}
		}
		return k.Key, false, nil
	}
	var reason struct {
		Error string `json:"error"`
	}
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &reason); err != nil {
			reason.Error = ""
		}
	}
	switch {
	case reason.Error == "authorization_pending":
		return "", false, nil
	case reason.Error == "slow_down" || resp.StatusCode == http.StatusTooManyRequests:
		return "", true, nil
	case reason.Error == "access_denied":
		return "", false, &Error{Code: ExitForbidden, Operation: "deviceauthToken", Status: resp.StatusCode, Message: "the sign-in was denied"}
	case reason.Error == "expired_token":
		return "", false, &Error{Code: ExitTimeout, Operation: "deviceauthToken", Status: resp.StatusCode, Message: "the code expired before it was approved; run login --device again"}
	case reason.Error == "invalid_grant":
		return "", false, &Error{Code: ExitAuth, Operation: "deviceauthToken", Status: resp.StatusCode, Message: "the server no longer knows this sign-in; run login --device again"}
	}
	return "", false, apiError("deviceauthToken", resp, body)
}
