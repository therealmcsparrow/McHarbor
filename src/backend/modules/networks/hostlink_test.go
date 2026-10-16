// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/moby/moby/client"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

func TestValidateHostLinkName(t *testing.T) {
	cases := []struct {
		name string
		want error
	}{
		{"mcvlan0", nil},
		{"host-lan", nil},
		{"a.b_c-d", nil},
		{"", ErrHostLinkNameInvalid},
		{"-lead", ErrHostLinkNameInvalid},
		{"has space", ErrHostLinkNameInvalid},
		{"sixteen-chars-xx", ErrHostLinkNameInvalid},
		{"x;reboot", ErrHostLinkNameInvalid},
		{"mcharbor-h1", ErrHostLinkNameReserved},
		{"MCHT1", ErrHostLinkNameReserved},
		{"mchv5", ErrHostLinkNameReserved},
		{"mchd9", ErrHostLinkNameReserved},
	}
	for _, c := range cases {
		err := validateHostLinkName(c.name)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("validateHostLinkName(%q) = %v, want %v", c.name, err, c.want)
		}
	}
}

func macvlanRequest(hl *HostLinkRequest, v6 bool) CreateRequest {
	configs := []IPAMConfigRequest{{Subnet: "192.168.1.0/24", IPRange: "192.168.1.128/25", Gateway: "192.168.1.1"}}
	if v6 {
		configs = append(configs, IPAMConfigRequest{Subnet: "2001:db8:1::/64"})
	}
	return CreateRequest{
		Name:       "lan",
		Driver:     "macvlan",
		EnableIPv6: v6,
		Options:    map[string]string{"parent": "eth0"},
		IPAM:       &IPAMRequest{Config: configs},
		HostLink:   hl,
	}
}

func TestValidateHostLinkRefusals(t *testing.T) {
	cases := []struct {
		name string
		edit func(*CreateRequest)
		hl   HostLinkRequest
		v6   bool
		want error
	}{
		{"ok v4", nil, HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24"}, false, nil},
		{"ok v4 + v6", nil, HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24", IPv6: "2001:db8:1::10/64"}, true, nil},
		{"ok slaac", nil, HostLinkRequest{Name: "mcvlan0", IPv6Auto: true}, true, nil},
		{"not macvlan", func(r *CreateRequest) { r.Driver = "ipvlan" }, HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24"}, false, ErrHostLinkNeedsMacvlan},
		{"no parent", func(r *CreateRequest) { r.Options = nil }, HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24"}, false, ErrHostLinkNeedsMacvlan},
		{"passthru", func(r *CreateRequest) { r.Options["macvlan_mode"] = "passthru" }, HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24"}, false, ErrHostLinkPassthru},
		{"bad name", nil, HostLinkRequest{Name: "bad name", IPv4: "192.168.1.10/24"}, false, ErrHostLinkNameInvalid},
		{"named like parent", nil, HostLinkRequest{Name: "eth0", IPv4: "192.168.1.10/24"}, false, ErrHostLinkNameInvalid},
		{"reserved", nil, HostLinkRequest{Name: "mcharbor-x", IPv4: "192.168.1.10/24"}, false, ErrHostLinkNameReserved},
		{"outside subnet", nil, HostLinkRequest{Name: "mcvlan0", IPv4: "10.0.0.10/24"}, false, ErrHostLinkAddressBad},
		{"inside range", nil, HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.200/24"}, false, ErrHostLinkAddressRange},
		{"v6 in v4 field", nil, HostLinkRequest{Name: "mcvlan0", IPv4: "2001:db8:1::10/64"}, true, ErrHostLinkAddressBad},
		{"v6 without enable", nil, HostLinkRequest{Name: "mcvlan0", IPv6: "2001:db8:1::10/64"}, false, ErrHostLinkAddressBad},
		{"slaac without enable", nil, HostLinkRequest{Name: "mcvlan0", IPv6Auto: true}, false, ErrHostLinkAddressBad},
		{"v6 and slaac", nil, HostLinkRequest{Name: "mcvlan0", IPv6: "2001:db8:1::10/64", IPv6Auto: true}, true, ErrHostLinkAddressBad},
		{"no address", nil, HostLinkRequest{Name: "mcvlan0"}, false, ErrHostLinkAddressBad},
	}
	for _, c := range cases {
		req := macvlanRequest(&c.hl, c.v6)
		if c.edit != nil {
			c.edit(&req)
		}
		ipam, err := req.IPAM.toSDK(req.EnableIPv6)
		if err != nil {
			t.Fatalf("%s: ipam: %v", c.name, err)
		}
		_, err = validateHostLink(req, ipam)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestHostLinkLabelsAreStrippedThenWritten(t *testing.T) {
	forged := map[string]string{"keep": "me", hostLinkLabelName: "evil", hostLinkLabelPrefix + "other": "x"}
	stripped := stripHostLinkLabels(forged)
	if !reflect.DeepEqual(stripped, map[string]string{"keep": "me"}) {
		t.Fatalf("forged host-link labels survived: %v", stripped)
	}
	spec := hostLinkSpec{Name: "mcvlan0", IPv4: netip.MustParsePrefix("192.168.1.10/24"), IPv6Auto: true}
	got := hostLinkLabels(stripped, spec)
	want := map[string]string{"keep": "me", hostLinkLabelName: "mcvlan0", hostLinkLabelIPv4: "192.168.1.10/24", hostLinkLabelIPv6: "auto"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
	info := hostLinkFromLabels(got)
	if info == nil || info.Name != "mcvlan0" || !info.IPv6Auto || info.IPv6 != "" || info.IPv4 != "192.168.1.10/24" {
		t.Errorf("round trip = %+v", info)
	}
	if hostLinkFromLabels(map[string]string{"keep": "me"}) != nil {
		t.Error("a network without the name label reported a host link")
	}
}

func TestHostLinkAddressesAreReservedInTheirOwnFamilyBlock(t *testing.T) {
	req := macvlanRequest(&HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24", IPv6: "2001:db8:1::10/64"}, true)
	ipam, err := req.IPAM.toSDK(true)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := validateHostLink(req, ipam)
	if err != nil {
		t.Fatal(err)
	}
	reserveHostAddresses(ipam, spec)
	if got := ipam.Config[0].AuxAddress[hostLinkAuxName]; got != netip.MustParseAddr("192.168.1.10") {
		t.Errorf("v4 block aux = %v", got)
	}
	if got := ipam.Config[1].AuxAddress[hostLinkAuxName]; got != netip.MustParseAddr("2001:db8:1::10") {
		t.Errorf("v6 block aux = %v", got)
	}
}

func TestHostLinkRouteDecisionTable(t *testing.T) {
	cases := []struct {
		name           string
		f              hostLinkFacts
		route, unavail string
	}{
		{"agent ok", hostLinkFacts{AgentEnv: true, AgentConnected: true, AgentSupports: true}, routeAgent, ""},
		{"agent offline", hostLinkFacts{AgentEnv: true}, routeNone, UnavailableAgentUnreachable},
		{"agent old", hostLinkFacts{AgentEnv: true, AgentConnected: true}, routeNone, UnavailableAgentOutdated},
		{"agent env never goes native", hostLinkFacts{AgentEnv: true, NativeHost: true, LocalSocket: true, NativeAvailable: true}, routeNone, UnavailableAgentUnreachable},
		{"native local", hostLinkFacts{NativeHost: true, LocalSocket: true, NativeAvailable: true}, routeNative, ""},
		{"native but remote socket", hostLinkFacts{NativeHost: true, NativeAvailable: true}, routeNone, UnavailableContainerized},
		{"native without ip", hostLinkFacts{NativeHost: true, LocalSocket: true}, routeNone, UnavailableContainerized},
		{"containerized", hostLinkFacts{LocalSocket: true, NativeAvailable: true}, routeNone, UnavailableContainerized},
	}
	for _, c := range cases {
		route, unavail := hostLinkRoute(c.f)
		if route != c.route || unavail != c.unavail {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, route, unavail, c.route, c.unavail)
		}
	}
}

// fakeLinker records every operation into a log shared with the fake daemon.
type fakeLinker struct {
	log  *callLog
	fail *coreagent.HostLinkPayload
}

func (f *fakeLinker) HostLink(_ context.Context, p coreagent.HostLinkPayload) (*coreagent.HostLinkPayload, error) {
	f.log.add("link " + p.Op + " " + p.Name + " owner=" + p.OwnerID + " parent=" + p.Parent)
	if f.fail != nil {
		return f.fail, nil
	}
	return &coreagent.HostLinkPayload{Success: true}, nil
}

type callLog struct {
	mu    sync.Mutex
	lines []string
}

func (c *callLog) add(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, s)
}

func (c *callLog) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

// daemonService returns a Service backed by a fake Docker daemon that logs each request.
func daemonService(t *testing.T, log *callLog, labels string) *Service {
	t.Helper()
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := strings.Index(r.URL.Path, "/networks")
		route := r.Method + " " + r.URL.Path[idx:]
		if r.Method == http.MethodPost {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Errorf("read body: %v", readErr)
			}
			if strings.Contains(string(body), hostLinkLabelName) {
				route += " [labelled]"
			}
			if strings.Contains(string(body), hostLinkAuxName) {
				route += " [aux]"
			}
		}
		log.add(route)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			writeBody(t, w, `{"Id":"net1","Warning":""}`)
		case http.MethodGet:
			writeBody(t, w, `{"Id":"net1","Name":"lan","Driver":"macvlan","Options":{"parent":"eth0"},"Labels":{`+labels+`}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(daemon.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool := docker.NewClientPool(nil, nil, nil)
	pool.SetClient("env", cli)
	return &Service{pool: pool}
}

func linkerSeam(l hostLinker, reason string) func(context.Context, string) (hostLinker, string) {
	return func(context.Context, string) (hostLinker, string) { return l, reason }
}

func TestCreateRefusesBeforeTouchingDockerWhenNoExecutor(t *testing.T) {
	log := &callLog{}
	svc := daemonService(t, log, "")
	svc.linkerFor = linkerSeam(nil, UnavailableContainerized)
	req := macvlanRequest(&HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24"}, false)
	_, err := svc.Create(context.Background(), "env", req)
	var un *HostLinkUnavailableError
	if !errors.As(err, &un) || un.Reason != UnavailableContainerized || !errors.Is(err, ErrHostLinkUnavailable) {
		t.Fatalf("err = %v, want unavailable/containerized", err)
	}
	if got := log.all(); len(got) != 0 {
		t.Errorf("docker was called before the refusal: %v", got)
	}
}

func TestCreateEnsuresTheLinkAfterTheNetworkAndStoresLabels(t *testing.T) {
	log := &callLog{}
	svc := daemonService(t, log, "")
	svc.linkerFor = linkerSeam(&fakeLinker{log: log}, "")
	req := macvlanRequest(&HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24"}, false)
	req.Labels = map[string]string{hostLinkLabelName: "forged"}
	if _, err := svc.Create(context.Background(), "env", req); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /networks/create [labelled] [aux]", "link ensure mcvlan0 owner=net1 parent=eth0"}
	if got := log.all(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestCreateRollsBackTheNetworkWhenEnsureFails(t *testing.T) {
	cases := []struct {
		name string
		fail coreagent.HostLinkPayload
		want error
	}{
		{"failure", coreagent.HostLinkPayload{Error: "ip: boom", ErrorCode: "permission_denied"}, ErrHostLinkFailed},
		{"name taken", coreagent.HostLinkPayload{Error: "in use", ErrorCode: coreagent.HostLinkErrNameTaken}, ErrHostLinkNameTaken},
	}
	for _, c := range cases {
		log := &callLog{}
		svc := daemonService(t, log, "")
		fail := c.fail
		svc.linkerFor = linkerSeam(&fakeLinker{log: log, fail: &fail}, "")
		req := macvlanRequest(&HostLinkRequest{Name: "mcvlan0", IPv4: "192.168.1.10/24"}, false)
		_, err := svc.Create(context.Background(), "env", req)
		if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.fail.Error) {
			t.Errorf("%s: err = %v, want %v carrying the agent's text", c.name, err, c.want)
		}
		want := []string{"POST /networks/create [labelled] [aux]", "link ensure mcvlan0 owner=net1 parent=eth0", "DELETE /networks/net1"}
		if got := log.all(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: calls = %v, want %v", c.name, got, want)
		}
	}
}

const labelledNet = `"com.mcharbor.host-link.name":"mcvlan0","com.mcharbor.host-link.ipv4":"192.168.1.10/24"`

func TestRemoveDeletesTheLinkBeforeTheNetwork(t *testing.T) {
	log := &callLog{}
	svc := daemonService(t, log, labelledNet)
	svc.linkerFor = linkerSeam(&fakeLinker{log: log}, "")
	if err := svc.Remove(context.Background(), "env", "net1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /networks/net1", "link remove mcvlan0 owner=net1 parent=", "DELETE /networks/net1"}
	if got := log.all(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestRemoveKeepsTheNetworkWhenTheLinkCannotBeRemoved(t *testing.T) {
	log := &callLog{}
	svc := daemonService(t, log, labelledNet)
	svc.linkerFor = linkerSeam(&fakeLinker{log: log, fail: &coreagent.HostLinkPayload{Error: "busy"}}, "")
	if err := svc.Remove(context.Background(), "env", "net1"); !errors.Is(err, ErrHostLinkFailed) {
		t.Fatalf("err = %v, want host link failure", err)
	}
	for _, line := range log.all() {
		if strings.HasPrefix(line, "DELETE") {
			t.Errorf("network was deleted although its link was not: %v", log.all())
		}
	}
}

func TestRemoveAbortsWhenTheAgentIsUnreachable(t *testing.T) {
	log := &callLog{}
	svc := daemonService(t, log, labelledNet)
	svc.linkerFor = linkerSeam(nil, UnavailableAgentUnreachable)
	if err := svc.Remove(context.Background(), "env", "net1"); !errors.Is(err, ErrHostLinkUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestRemoveOfAPlainNetworkNeverAsksForAnExecutor(t *testing.T) {
	log := &callLog{}
	svc := daemonService(t, log, "")
	svc.linkerFor = func(context.Context, string) (hostLinker, string) {
		t.Fatal("executor resolved for a network without a host link")
		return nil, ""
	}
	if err := svc.Remove(context.Background(), "env", "net1"); err != nil {
		t.Fatal(err)
	}
}

// fakeIP stands in for ip(8) and sysfs so the native executor's command plan can be asserted.
type fakeIP struct {
	cmds    []string
	aliases map[string]string
	listing string
	failOn  string
	failMsg string
	sysctls []string
}

func (f *fakeIP) linker() *nativeHostLinker {
	return &nativeHostLinker{
		ip: func(_ context.Context, args ...string) (string, error) {
			line := strings.Join(args, " ")
			f.cmds = append(f.cmds, line)
			if f.failOn != "" && strings.Contains(line, f.failOn) {
				return f.failMsg, errors.New("exit status 2")
			}
			if strings.HasPrefix(line, "-4 ") && strings.Contains(line, "addr show") {
				return f.listing, nil
			}
			return "", nil
		},
		readAlias: func(name string) (string, bool, error) {
			a, ok := f.aliases[name]
			return a, ok, nil
		},
		writeSysctl: func(path, value string) error {
			f.sysctls = append(f.sysctls, path+"="+value)
			return nil
		},
	}
}

func ensurePayload() coreagent.HostLinkPayload {
	return coreagent.HostLinkPayload{
		Op: coreagent.HostLinkOpEnsure, Owner: coreagent.HostLinkOwnerDocker, OwnerID: "net1",
		Name: "mcvlan0", Parent: "eth0", IPv4CIDR: "192.168.1.10/24",
	}
}

func TestNativeEnsurePlan(t *testing.T) {
	f := &fakeIP{aliases: map[string]string{"eth0": ""}}
	res := hostLinkOf(t, f.linker(), ensurePayload())
	if !res.Success {
		t.Fatalf("ensure failed: %+v", res)
	}
	want := []string{
		"link add mcvlan0 link eth0 type macvlan mode bridge",
		"link set mcvlan0 alias mcharbor:hostlink:docker:net1",
		"addr replace 192.168.1.10/24 dev mcvlan0",
		"-4 -o addr show dev mcvlan0 scope global permanent",
		"-6 -o addr show dev mcvlan0 scope global permanent",
		"link set mcvlan0 up",
	}
	if !reflect.DeepEqual(f.cmds, want) {
		t.Errorf("commands:\n got %q\nwant %q", f.cmds, want)
	}
}

func TestNativeEnsureSLAACWritesSysctls(t *testing.T) {
	f := &fakeIP{aliases: map[string]string{"eth0": ""}}
	p := ensurePayload()
	p.IPv6Enabled = true
	if res := hostLinkOf(t, f.linker(), p); !res.Success {
		t.Fatalf("ensure failed: %+v", res)
	}
	want := []string{
		"/proc/sys/net/ipv6/conf/mcvlan0/disable_ipv6=0",
		"/proc/sys/net/ipv6/conf/mcvlan0/accept_ra=2",
		"/proc/sys/net/ipv6/conf/mcvlan0/autoconf=1",
	}
	if !reflect.DeepEqual(f.sysctls, want) {
		t.Errorf("sysctls = %v, want %v", f.sysctls, want)
	}
}

func TestNativeEnsureRemovesStaleAddresses(t *testing.T) {
	f := &fakeIP{
		aliases: map[string]string{"eth0": "", "mcvlan0": "mcharbor:hostlink:docker:net1"},
		listing: "5: mcvlan0    inet 192.168.1.9/24 scope global mcvlan0\n5: mcvlan0    inet 192.168.1.10/24 scope global mcvlan0\n",
	}
	if res := hostLinkOf(t, f.linker(), ensurePayload()); !res.Success {
		t.Fatalf("ensure failed: %+v", res)
	}
	joined := strings.Join(f.cmds, "|")
	if !strings.Contains(joined, "addr del 192.168.1.9/24 dev mcvlan0") || strings.Contains(joined, "addr del 192.168.1.10/24") {
		t.Errorf("stale address handling wrong: %q", f.cmds)
	}
	if strings.Contains(joined, "link add") {
		t.Errorf("an owned link was created again: %q", f.cmds)
	}
}

func TestNativeRefusesAForeignLinkAndMissingParent(t *testing.T) {
	f := &fakeIP{aliases: map[string]string{"eth0": "", "mcvlan0": "somebody else"}}
	res := hostLinkOf(t, f.linker(), ensurePayload())
	if res.Success || res.ErrorCode != coreagent.HostLinkErrNameTaken || len(f.cmds) != 0 {
		t.Errorf("foreign link: %+v, cmds %v", res, f.cmds)
	}
	f = &fakeIP{aliases: map[string]string{}}
	res = hostLinkOf(t, f.linker(), ensurePayload())
	if res.Success || res.ErrorCode != coreagent.HostLinkErrParentNotFound {
		t.Errorf("missing parent: %+v", res)
	}
}

func TestNativeMapsPermissionAndRollsBackANewLink(t *testing.T) {
	f := &fakeIP{aliases: map[string]string{"eth0": ""}, failOn: "addr replace", failMsg: "RTNETLINK answers: Operation not permitted"}
	res := hostLinkOf(t, f.linker(), ensurePayload())
	if res.Success || res.ErrorCode != coreagent.HostLinkErrPermissionDenied {
		t.Fatalf("permission: %+v", res)
	}
	if last := f.cmds[len(f.cmds)-1]; last != "link del mcvlan0" {
		t.Errorf("a half-made link was left: %q", f.cmds)
	}
}

func TestNativeRemoveChecksOwnership(t *testing.T) {
	rm := coreagent.HostLinkPayload{Op: coreagent.HostLinkOpRemove, Owner: coreagent.HostLinkOwnerDocker, OwnerID: "net1", Name: "mcvlan0"}
	f := &fakeIP{aliases: map[string]string{"mcvlan0": "mcharbor:hostlink:docker:net1"}}
	if res := hostLinkOf(t, f.linker(), rm); !res.Success || !reflect.DeepEqual(f.cmds, []string{"link del mcvlan0"}) {
		t.Errorf("owned: %+v %v", res, f.cmds)
	}
	f = &fakeIP{aliases: map[string]string{"mcvlan0": "mcharbor:hostlink:docker:other"}}
	if res := hostLinkOf(t, f.linker(), rm); res.Success || len(f.cmds) != 0 {
		t.Errorf("foreign link deleted: %+v %v", res, f.cmds)
	}
	f = &fakeIP{aliases: map[string]string{}}
	if res := hostLinkOf(t, f.linker(), rm); !res.Success || len(f.cmds) != 0 {
		t.Errorf("absent link should be a no-op success: %+v %v", res, f.cmds)
	}
}

func TestNativeRejectsInjectionShapedNames(t *testing.T) {
	f := &fakeIP{aliases: map[string]string{"eth0": ""}}
	p := ensurePayload()
	p.Name = "x;reboot"
	if res := hostLinkOf(t, f.linker(), p); res.Success || res.ErrorCode != coreagent.HostLinkErrInvalid || len(f.cmds) != 0 {
		t.Errorf("injection-shaped name ran: %+v %v", res, f.cmds)
	}
	p = ensurePayload()
	p.Parent = "--help"
	if res := hostLinkOf(t, f.linker(), p); res.Success || len(f.cmds) != 0 {
		t.Errorf("flag-shaped parent ran: %+v %v", res, f.cmds)
	}
}

func TestParseAddrListing(t *testing.T) {
	out := "2: eth0    inet 192.168.1.9/24 brd 192.168.1.255 scope global eth0\n3: eth0    inet6 2001:db8::5/64 scope global\n"
	got := parseAddrListing(out)
	if !reflect.DeepEqual(got, []string{"192.168.1.9/24", "2001:db8::5/64"}) {
		t.Errorf("parseAddrListing = %v", got)
	}
}

func writeBody(t *testing.T, w io.Writer, s string) {
	t.Helper()
	if _, err := io.WriteString(w, s); err != nil {
		t.Errorf("write body: %v", err)
	}
}

func hostLinkOf(t *testing.T, l *nativeHostLinker, p coreagent.HostLinkPayload) *coreagent.HostLinkPayload {
	t.Helper()
	res, err := l.HostLink(context.Background(), p)
	if err != nil {
		t.Fatalf("HostLink: %v", err)
	}
	return res
}
