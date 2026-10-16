// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/demo"
)

// symlinkTree builds root/real/dir, root/real/file, root/linkdir -> real, root/real/inner ->
// /etc-like target, and a dangling link, and returns the host-style paths the cases bind.
func symlinkTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"real/dir", "target"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "real", "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "target"), filepath.Join(root, "real", "inner")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "real", "dangling")); err != nil {
		t.Fatal(err)
	}
	return root
}

// symlinkCases are the sources the check is asked about, and which of them are linked.
var symlinkCases = []struct {
	src    string
	linked bool
}{
	{"/real/dir", false},
	{"/real/file", false},
	{"/real/dir/missing/deeper", false},
	{"/absent/entirely", false},
	{"/linkdir", true},
	{"/linkdir/dir", true},
	{"/real/inner", true},
	{"/real/inner/sub", true},
	{"/real/dangling", true},
	{"/real//dir/", false},
}

func wantLinked() map[string]bool {
	want := map[string]bool{}
	for _, c := range symlinkCases {
		if c.linked {
			want[path.Clean(c.src)] = true
		}
	}
	return want
}

func caseSources() []string {
	var out []string
	for _, c := range symlinkCases {
		out = append(out, c.src)
	}
	return cleanBindSources(out)
}

// TestBindCheckScriptFindsEverySymlinkComponent runs the helper's shell script against a real
// tree: a link anywhere in the path is reported, a missing tail is not, and the run ends with
// the done line the parser requires.
func TestBindCheckScriptFindsEverySymlinkComponent(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh on this machine")
	}
	root := symlinkTree(t)
	paths := caseSources()
	args := append([]string{"-c", bindCheckScript, "mcharbor-bind-check", filepath.ToSlash(root)}, paths...)
	out, err := exec.Command(sh, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	got, err := parseBindCheckOutput(string(out), paths)
	if err != nil {
		t.Fatalf("parsing %q: %v", out, err)
	}
	if !reflect.DeepEqual(got, wantLinked()) {
		t.Fatalf("script reported %v, want %v (output %q)", got, wantLinked(), out)
	}
}

// TestLocalBindSymlinksMatchesTheScript is the native short-circuit: the same tree, the same answer.
func TestLocalBindSymlinksMatchesTheScript(t *testing.T) {
	root := symlinkTree(t)
	got, err := localBindSymlinks(root, caseSources())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantLinked()) {
		t.Fatalf("local check reported %v, want %v", got, wantLinked())
	}
}

func TestParseBindCheckOutputFailsClosed(t *testing.T) {
	paths := []string{"/a", "/b"}
	got, err := parseBindCheckOutput("symlink 1\r\n"+bindCheckDone+"\r\n", paths)
	if err != nil || !reflect.DeepEqual(got, map[string]bool{"/b": true}) {
		t.Fatalf("tty output: %v %v", got, err)
	}
	for name, out := range map[string]string{
		"no done line":   "symlink 0\n",
		"empty":          "",
		"unknown source": "unknown 0\n" + bindCheckDone,
		"out of range":   "symlink 2\n" + bindCheckDone,
		"garbage":        "sh: test: not found\n" + bindCheckDone,
	} {
		if _, err := parseBindCheckOutput(out, paths); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

func TestBindDeviceOf(t *testing.T) {
	for _, tc := range []struct {
		driver string
		opts   map[string]string
		dev    string
	}{
		{"local", map[string]string{"type": "none", "o": "bind", "device": "/srv/x"}, "/srv/x"},
		{"", map[string]string{"o": "rbind,ro", "device": "/srv/y"}, "/srv/y"},
		{"local", map[string]string{"type": "nfs", "o": "addr=1.2.3.4", "device": ":/export"}, ""},
		{"local", map[string]string{"type": "none", "o": "bind", "device": "relative"}, ""},
		{"rexray", map[string]string{"device": "/srv/z"}, ""},
	} {
		dev, ok := bindDeviceOf(tc.driver, tc.opts)
		if dev != tc.dev || ok != (tc.dev != "") {
			t.Errorf("%s %v: got %q %v, want %q", tc.driver, tc.opts, dev, ok, tc.dev)
		}
	}
}

// recordingProbe answers from linked and records every call, so a test can count helper runs.
type recordingProbe struct {
	linked map[string]bool
	err    error
	calls  [][]string
}

func (p *recordingProbe) probe(paths []string) (map[string]bool, error) {
	p.calls = append(p.calls, append([]string(nil), paths...))
	if p.err != nil {
		return nil, p.err
	}
	out := map[string]bool{}
	for _, s := range paths {
		if p.linked[s] {
			out[s] = true
		}
	}
	return out, nil
}

const symlinkCompose = `services:
  web:
    image: nginx
    volumes:
      - /srv/site:/usr/share/nginx/html
  sneaky:
    image: docker:cli
    volumes:
      - /tmp/x/s:/var/run/docker.sock
      - data:/data
  disguised:
    image: alpine
    volumes:
      - dev:/dev-ish
    secrets:
      - token
volumes:
  data: {}
  dev:
    driver_opts: {type: none, o: bind, device: /srv/dev}
secrets:
  token:
    file: /srv/secrets/token
`

// TestComposeSymlinkedBindIsReportedOnItsService checks one probe call covers every service's
// sources, and only the service whose source is linked carries host_bind_symlink.
func TestComposeSymlinkedBindIsReportedOnItsService(t *testing.T) {
	p := &recordingProbe{linked: map[string]bool{"/tmp/x/s": true}}
	lookup := func(string) (string, map[string]string, bool, error) { return "", nil, false, nil }
	got, err := ComposeHostAccessReasonsProbe([]byte(symlinkCompose), "/stacks/demo", nil, SelfHost{Hosted: true}, lookup, p.probe)
	if err != nil {
		t.Fatal(err)
	}
	want := []ComposeHostAccess{{Service: "sneaky", Reasons: []string{HostAccessBindSymlink}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if len(p.calls) != 1 {
		t.Fatalf("probe ran %d times, want once per file", len(p.calls))
	}
	sources := append([]string(nil), p.calls[0]...)
	sort.Strings(sources)
	if wantSrc := []string{"/srv/dev", "/srv/secrets/token", "/srv/site", "/tmp/x/s"}; !reflect.DeepEqual(sources, wantSrc) {
		t.Fatalf("probe was asked about %v, want %v", sources, wantSrc)
	}
}

// TestComposeSymlinkCheckFailsClosed covers every way the compose check cannot answer: the
// probe errors, or an existing volume cannot be inspected. Both refuse as unresolved_source.
func TestComposeSymlinkCheckFailsClosed(t *testing.T) {
	unresolved := []ComposeHostAccess{{Reasons: []string{HostAccessUnresolved}}}
	okLookup := func(string) (string, map[string]string, bool, error) { return "", nil, false, nil }
	failing := &recordingProbe{err: errors.New("helper timed out")}
	got, err := ComposeHostAccessReasonsProbe([]byte(symlinkCompose), "/stacks/demo", nil, SelfHost{Hosted: true}, okLookup, failing.probe)
	if err != nil || !reflect.DeepEqual(got, unresolved) {
		t.Fatalf("probe error: got %+v %v, want %+v", got, err, unresolved)
	}
	// The denylist pass reads the volume first and succeeds; the source listing reads it
	// again, and that read is the one that fails.
	lookups := 0
	badLookup := func(string) (string, map[string]string, bool, error) {
		lookups++
		if lookups > 1 {
			return "", nil, false, errors.New("daemon gone")
		}
		return "", nil, false, nil
	}
	got, err = ComposeHostAccessReasonsProbe([]byte(symlinkCompose), "/stacks/demo", nil, SelfHost{Hosted: true}, badLookup, (&recordingProbe{}).probe)
	if err != nil || !reflect.DeepEqual(got, unresolved) {
		t.Fatalf("lookup error: got %+v %v, want %+v", got, err, unresolved)
	}
}

// TestComposeAlreadyRefusedSkipsTheProbe keeps the helper off a file the denylist refuses anyway.
func TestComposeAlreadyRefusedSkipsTheProbe(t *testing.T) {
	p := &recordingProbe{}
	compose := "services:\n  sock:\n    image: docker\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n      - /srv/x:/x\n"
	got, err := ComposeHostAccessReasonsProbe([]byte(compose), "/stacks/demo", nil, SelfHost{Hosted: true}, nil, p.probe)
	if err != nil || len(got) == 0 {
		t.Fatalf("socket compose: %+v %v", got, err)
	}
	if len(p.calls) != 0 {
		t.Fatalf("probe ran on a file already refused: %v", p.calls)
	}
	plain := "services:\n  web:\n    image: nginx\n"
	if got, err := ComposeHostAccessReasonsProbe([]byte(plain), "/stacks/demo", nil, SelfHost{Hosted: true}, nil, p.probe); err != nil || got != nil || len(p.calls) != 0 {
		t.Fatalf("a file with no binds: %+v %v, %d probe calls", got, err, len(p.calls))
	}
}

// TestBindProbeIsOffInDemoMode runs in a child process, because demo mode cannot be left once
// a process enters it: there, a probe whose check would fail answers "no symlink".
func TestBindProbeIsOffInDemoMode(t *testing.T) {
	if os.Getenv("MCHARBOR_TEST_DEMO_PROBE") == "1" {
		dir := t.TempDir()
		m, err := demo.NewMarker("tester")
		if err != nil {
			t.Fatal(err)
		}
		if err := demo.WriteMarker(dir, m); err != nil {
			t.Fatal(err)
		}
		if _, err := demo.Load(dir); err != nil {
			t.Fatal(err)
		}
		pool := NewClientPool(nil, nil, nil)
		pool.SetBindSymlinkCheck(func(context.Context, string, SelfHost, []string) (map[string]bool, error) {
			return nil, errors.New("the helper must not run in demo mode")
		})
		linked, err := pool.BindSymlinkProbeFor(context.Background(), "envA", SelfHost{Hosted: true})([]string{"/tmp/x/s"})
		if err != nil || len(linked) != 0 {
			t.Fatalf("demo probe: %v %v", linked, err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBindProbeIsOffInDemoMode$")
	cmd.Env = append(os.Environ(), "MCHARBOR_TEST_DEMO_PROBE=1")
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "PASS") {
		t.Fatalf("demo child: %v\n%s", err, out)
	}
}

// TestSetBindSymlinkCheckSeam proves the probe asks the override with cleaned sources, and
// that relative sources (named volumes) never reach it.
func TestSetBindSymlinkCheckSeam(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)
	var asked []string
	pool.SetBindSymlinkCheck(func(_ context.Context, _ string, _ SelfHost, paths []string) (map[string]bool, error) {
		asked = paths
		return map[string]bool{"/tmp/x/s": true}, nil
	})
	t.Cleanup(func() { pool.SetBindSymlinkCheck(nil) })
	reasons := bindSymlinkReasons(pool.BindSymlinkProbeFor(context.Background(), "envA", SelfHost{Hosted: true}), []string{"/tmp/x/s/", "named", "/tmp/x/s"})
	if !reflect.DeepEqual(reasons, []string{HostAccessBindSymlink}) || !reflect.DeepEqual(asked, []string{"/tmp/x/s"}) {
		t.Fatalf("reasons %v, asked %v", reasons, asked)
	}
}
