// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const sampleDeviceOutput = "D|/dev/ttyUSB0|char|188|0\r\n" +
	"U|/dev/ttyUSB0|ID_VENDOR=Silicon_Labs\r\n" +
	"U|/dev/ttyUSB0|ID_MODEL=CP2102_USB_to_UART_Bridge_Controller\r\n" +
	"U|/dev/ttyUSB0|ID_SERIAL=Silicon_Labs_CP2102_0001\r\n" +
	"D|/dev/dri/card0|char|226|0\r\n" +
	"R|/dev/dri/card0|i915\r\n" +
	"D|/dev/sda|block|8|0\r\n" +
	"U|/dev/sda|ID_MODEL=Samsung_SSD_870\r\n" +
	"D|/dev/kvm|char|10|232\r\n" +
	"D|/dev/snd/controlC0|char|116|0\r\n" +
	"D|/dev/net/tun|char|10|200\r\n" +
	"D|/dev/bus/usb/001/002|char|189|1\r\n" +
	"D|/dev/input/event3|char|13|67\r\n" +
	"D|/dev/video0|char|81|0\r\n" +
	"L|/dev/serial/by-path/pci-0000:00:14.0-usb-0:1:1.0-port0|/dev/ttyUSB0\r\n" +
	"L|/dev/serial/by-id/usb-Silicon_Labs_CP2102_0001-if00-port0|/dev/ttyUSB0\r\n" +
	"L|/dev/disk/by-id/wwn-0x5002538e40a1b2c3|/dev/sda\r\n" +
	"L|/dev/disk/by-id/ata-Samsung_SSD_870_S5Y1|/dev/sda\r\n" +
	"L|/dev/dri/by-path/pci-0000:00:02.0-card|/dev/dri/card0\r\n" +
	"OK\r\n"

func findDevice(t *testing.T, list HostDeviceList, devPath string) HostDevice {
	t.Helper()
	for _, device := range list.Devices {
		if device.Path == devPath {
			return device
		}
	}
	t.Fatalf("%s missing from %+v", devPath, list.Devices)
	return HostDevice{}
}

func TestParseHostDevices(t *testing.T) {
	list := parseHostDevices(sampleDeviceOutput)
	cases := []struct {
		path, typ, category, label, stable string
		major, minor                       int
		dangerous                          bool
	}{
		{"/dev/ttyUSB0", "char", DeviceCategorySerial, "Silicon Labs CP2102 USB to UART Bridge Controller", "/dev/serial/by-id/usb-Silicon_Labs_CP2102_0001-if00-port0", 188, 0, false},
		{"/dev/dri/card0", "char", DeviceCategoryGPU, "i915", "/dev/dri/by-path/pci-0000:00:02.0-card", 226, 0, false},
		{"/dev/sda", "block", DeviceCategoryBlock, "Samsung SSD 870", "/dev/disk/by-id/ata-Samsung_SSD_870_S5Y1", 8, 0, true},
		{"/dev/kvm", "char", DeviceCategorySystem, "", "", 10, 232, false},
		{"/dev/snd/controlC0", "char", DeviceCategoryAudio, "", "", 116, 0, false},
		{"/dev/net/tun", "char", DeviceCategoryNetwork, "", "", 10, 200, false},
		{"/dev/bus/usb/001/002", "char", DeviceCategoryUSB, "", "", 189, 1, false},
		{"/dev/input/event3", "char", DeviceCategoryInput, "", "", 13, 67, false},
		{"/dev/video0", "char", DeviceCategoryVideo, "", "", 81, 0, false},
	}
	if len(list.Devices) != len(cases) {
		t.Fatalf("parsed %d devices, want %d: %+v", len(list.Devices), len(cases), list.Devices)
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got := findDevice(t, list, c.path)
			if got.Type != c.typ || got.Category != c.category || got.Label != c.label || got.StablePath != c.stable ||
				got.Major != c.major || got.Minor != c.minor || got.Dangerous != c.dangerous {
				t.Errorf("got %+v, want %+v", got, c)
			}
		})
	}
	if list.Truncated {
		t.Error("a complete listing was marked truncated")
	}
	if first, last := list.Devices[0], list.Devices[len(list.Devices)-1]; first.Category != DeviceCategorySerial || last.Category != DeviceCategoryBlock {
		t.Errorf("devices not grouped in picker order: first %s, last %s", first.Path, last.Path)
	}
	if links := findDevice(t, list, "/dev/ttyUSB0").Links; len(links) != 2 {
		t.Errorf("ttyUSB0 links = %v, want both by-id and by-path", links)
	}
}

func TestParseHostDevicesLabelFallbacks(t *testing.T) {
	list := parseHostDevices("D|/dev/ttyACM0|char|166|0\n" +
		"L|/dev/serial/by-id/usb-Arduino_Uno_1234-if00|/dev/ttyACM0\n" +
		"D|/dev/ttyUSB1|char|188|1\nU|/dev/ttyUSB1|ID_SERIAL=FTDI_FT232R_A1B2\n")
	if got := findDevice(t, list, "/dev/ttyACM0").Label; got != "usb-Arduino_Uno_1234-if00" {
		t.Errorf("by-id fallback label = %q", got)
	}
	if got := findDevice(t, list, "/dev/ttyUSB1").Label; got != "FTDI FT232R A1B2" {
		t.Errorf("ID_SERIAL fallback label = %q", got)
	}
}

func TestParseHostDevicesRejectsAnythingOutsideDev(t *testing.T) {
	list := parseHostDevices(strings.Join([]string{
		"D|/etc/shadow|char|1|1",
		"D|/dev/../etc/passwd|char|1|1",
		"D|/dev/ttyUSB0/../../etc|char|1|1",
		"D|dev/ttyUSB0|char|1|1",
		"D|/dev/ttyUSB0|pipe|1|1",
		"D|/dev/ttyUSB0|char|x|1",
		"D|/dev/ttyUSB0|char|-1|1",
		"D|/dev/ttyUSB0|char|1",
		"D|/dev/tty\x1bUSB0|char|1|1",
		"D|/dev/ttyUSB2|char|188|2",
		"L|/etc/evil|/dev/ttyUSB2",
		"L|/dev/serial/by-id/evil|/etc/shadow",
		"U|/dev/unknown|ID_MODEL=ghost",
		"garbage without separators",
	}, "\n"))
	if len(list.Devices) != 1 || list.Devices[0].Path != "/dev/ttyUSB2" {
		t.Fatalf("accepted %+v, want only /dev/ttyUSB2", list.Devices)
	}
	if links := list.Devices[0].Links; len(links) != 0 {
		t.Fatalf("a link outside /dev was accepted: %v", links)
	}
}

func TestParseHostDevicesIsBounded(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxHostDevices+50; i++ {
		fmt.Fprintf(&b, "D|/dev/input/event%d|char|13|%d\n", i, i)
	}
	fmt.Fprintf(&b, "U|/dev/input/event0|ID_MODEL=%s\n", strings.Repeat("x", 10*maxHostDeviceField))
	list := parseHostDevices(b.String())
	if len(list.Devices) != maxHostDevices || !list.Truncated {
		t.Fatalf("got %d devices, truncated=%v; want %d and truncated", len(list.Devices), list.Truncated, maxHostDevices)
	}
	if label := findDevice(t, list, "/dev/input/event0").Label; len(label) > maxHostDeviceField {
		t.Fatalf("label is %d bytes, cap is %d", len(label), maxHostDeviceField)
	}
	if !parseHostDevices("D|/dev/kvm|char|10|232\nT|1\n").Truncated {
		t.Fatal("the script's own truncation marker was ignored")
	}
	lines := strings.Repeat("noise\n", maxHostDeviceLines+10) + "D|/dev/kvm|char|10|232\n"
	if got := parseHostDevices(lines); len(got.Devices) != 0 || !got.Truncated {
		t.Fatalf("output past the line cap was read: %+v", got)
	}
}

func TestHostDevicesScriptIsReadOnly(t *testing.T) {
	for _, forbidden := range []string{"rm ", "mknod", "chmod", "chown", "> /", ">/host", "tee ", "dd ", "mount "} {
		if strings.Contains(hostDevicesScript, forbidden) {
			t.Errorf("enumeration script contains %q", forbidden)
		}
	}
	if !strings.Contains(hostDevicesScript, "head -n 8000") {
		t.Error("enumeration script output is not bounded")
	}
}

type fakeDeviceLister struct {
	calls int
	env   string
	list  *HostDeviceList
	err   error
}

func (f *fakeDeviceLister) HostDevices(_ context.Context, envID string) (*HostDeviceList, error) {
	f.calls++
	f.env = envID
	return f.list, f.err
}

func openRBACDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "system.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func mountDevicesForTest(t *testing.T, lister *fakeDeviceLister) (http.Handler, *apispec.Registry) {
	t.Helper()
	database := openRBACDB(t)
	app := &router.AppDeps{DB: database, RBACService: rbac.NewService(database), Logger: slog.Default()}
	mux := chi.NewRouter()
	reg := apispec.NewRegistry()
	root := apispec.NewRouter(mux, reg, "/api", apispec.GroupProtected)
	root.Route("/system", func(r *apispec.Router) {
		mountHostDeviceRoutes(r, app, &hostDevicesHandler{app: app, lister: lister})
	})
	return mux, reg
}

func asUser(userID string, req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
}

func TestHostDevicesRouteIsGatedOnHostView(t *testing.T) {
	_, reg := mountDevicesForTest(t, &fakeDeviceLister{})
	ops := reg.Operations()
	if len(ops) != 1 {
		t.Fatalf("recorded %d operations, want 1", len(ops))
	}
	op := ops[0]
	if op.Method != http.MethodGet || strings.TrimSuffix(op.Path, "/") != "/api/system/host-devices" {
		t.Fatalf("recorded %s %s", op.Method, op.Path)
	}
	if keys := op.PermissionKeys(); len(keys) != 1 || keys[0] != string(rbac.PermHostView) {
		t.Fatalf("gated on %v, want host.view", keys)
	}
	if len(op.Perms) != 1 || op.Perms[0].Scope != rbac.ScopeEnvironment {
		t.Fatalf("scope %+v, want environment", op.Perms)
	}
}

func TestHostDevicesRouteRefusesWithoutThePermission(t *testing.T) {
	lister := &fakeDeviceLister{list: &HostDeviceList{Devices: []HostDevice{}}}
	mux, _ := mountDevicesForTest(t, lister)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser("nobody", httptest.NewRequest(http.MethodGet, "/system/host-devices?env=e1", nil)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
	if lister.calls != 0 {
		t.Fatal("a refused request reached the host")
	}
}

func TestHostDevicesRouteAnswersAndHidesErrors(t *testing.T) {
	lister := &fakeDeviceLister{list: parseHostDevicesPtr(sampleDeviceOutput)}
	mux, _ := mountDevicesForTest(t, lister)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser("system", httptest.NewRequest(http.MethodGet, "/system/host-devices?env=e1", nil)))
	if rec.Code != http.StatusOK || lister.env != "e1" {
		t.Fatalf("status %d env %q: %s", rec.Code, lister.env, rec.Body.String())
	}
	var body struct {
		Success bool           `json:"success"`
		Data    HostDeviceList `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.Success || len(body.Data.Devices) != 9 {
		t.Fatalf("body %s (%v)", rec.Body.String(), err)
	}

	lister.err = errors.New("dial unix /var/run/docker.sock: secret-detail")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser("system", httptest.NewRequest(http.MethodGet, "/system/host-devices?env=e1", nil)))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret-detail") || strings.Contains(rec.Body.String(), "docker.sock") {
		t.Fatalf("status %d leaked: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "err.host.devices_failed") {
		t.Fatalf("error code missing: %s", rec.Body.String())
	}
}

func parseHostDevicesPtr(output string) *HostDeviceList {
	list := parseHostDevices(output)
	return &list
}
