// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

const recreateOldNetwork = `{"Id":"net-old","Name":"lan","Driver":"macvlan","Options":{"parent":"eth0"},
"IPAM":{"Driver":"default","Config":[{"Subnet":"192.168.1.0/24","Gateway":"192.168.1.1"}]},
"Containers":{"c1":{"Name":"web","IPv4Address":"192.168.1.50/24","MacAddress":"02:42:c0:a8:01:32"}}}`

const recreateContainer = `{"Id":"c1","Name":"/web","Config":{"Labels":{}},"NetworkSettings":{"Networks":{"lan":{
"IPAddress":"192.168.1.50","MacAddress":"02:42:c0:a8:01:32","Aliases":["web","db-alias"]}}}}`

type recordedCall struct{ route, body string }

// recreateDaemon fakes the Docker API for one network with one container. refuseCreate makes the
// daemon reject a create whose body contains that text, to drive the restore path.
func recreateDaemon(t *testing.T, refuseCreate string) (*Service, func() []recordedCall) {
	t.Helper()
	log := &callLog{}
	var calls []recordedCall
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if i := strings.Index(path, "/networks"); i >= 0 {
			path = path[i:]
		} else if i := strings.Index(path, "/containers"); i >= 0 {
			path = path[i:]
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		route := r.Method + " " + path
		log.add(route)
		calls = append(calls, recordedCall{route: route, body: string(body)})
		w.Header().Set("Content-Type", "application/json")
		switch route {
		case "GET /networks/net-old":
			writeBody(t, w, recreateOldNetwork)
		case "GET /containers/c1/json":
			writeBody(t, w, recreateContainer)
		case "POST /networks/create":
			if refuseCreate != "" && strings.Contains(string(body), refuseCreate) {
				w.WriteHeader(http.StatusBadRequest)
				writeBody(t, w, `{"message":"invalid pool request: Pool overlaps with other one on this address space"}`)
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeBody(t, w, `{"Id":"net-new","Warning":""}`)
		default:
			w.WriteHeader(http.StatusOK)
			writeBody(t, w, `{}`)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(daemon.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool := docker.NewClientPool(nil, nil, nil)
	pool.SetClient("env", cli)
	return &Service{pool: pool}, func() []recordedCall { return calls }
}

func lanRequest(subnet string) CreateRequest {
	return CreateRequest{
		Name: "lan", Driver: "macvlan", Options: map[string]string{"parent": "eth0", "macvlan_mode": "bridge"},
		IPAM: &IPAMRequest{Config: []IPAMConfigRequest{{Subnet: subnet}}},
	}
}

func connectBodies(calls []recordedCall, network string) []string {
	var out []string
	for _, c := range calls {
		if c.route == "POST /networks/"+network+"/connect" {
			out = append(out, c.body)
		}
	}
	return out
}

func TestRecreateKeepsAddressMacAndAliases(t *testing.T) {
	svc, calls := recreateDaemon(t, "")
	res, err := svc.Recreate(context.Background(), "env", "net-old", lanRequest("192.168.1.0/24"))
	if err != nil {
		t.Fatalf("Recreate: %v", err)
	}
	bodies := connectBodies(calls(), "net-new")
	if len(bodies) != 1 {
		t.Fatalf("connects to the new network = %d, want 1 (%v)", len(bodies), calls())
	}
	for _, want := range []string{`"IPv4Address":"192.168.1.50"`, `"MacAddress":"02:42:c0:a8:01:32"`, `"db-alias"`} {
		if !strings.Contains(bodies[0], want) {
			t.Errorf("reconnect body lacks %s: %s", want, bodies[0])
		}
	}
	if len(res.Results) != 1 || !res.Results[0].AddressKept || res.Results[0].PreviousIPv4 != "192.168.1.50" {
		t.Errorf("results = %+v, want web kept on 192.168.1.50", res.Results)
	}
}

func TestRecreateReportsAnAddressOutsideTheNewSubnet(t *testing.T) {
	svc, calls := recreateDaemon(t, "")
	res, err := svc.Recreate(context.Background(), "env", "net-old", lanRequest("10.0.0.0/24"))
	if err != nil {
		t.Fatalf("Recreate: %v", err)
	}
	bodies := connectBodies(calls(), "net-new")
	if len(bodies) != 1 || strings.Contains(bodies[0], "192.168.1.50") {
		t.Errorf("an address outside the new subnet must not be pinned: %v", bodies)
	}
	if res.Results[0].AddressKept {
		t.Errorf("result claims the address was kept: %+v", res.Results[0])
	}
}

func TestRecreateRefusesABadDefinitionBeforeTouchingAnything(t *testing.T) {
	svc, calls := recreateDaemon(t, "")
	_, err := svc.Recreate(context.Background(), "env", "net-old", lanRequest("not-a-subnet"))
	if !errors.Is(err, ErrInvalidSubnet) {
		t.Fatalf("err = %v, want invalid subnet", err)
	}
	for _, c := range calls() {
		if c.route != "GET /networks/net-old" && c.route != "GET /containers/c1/json" {
			t.Errorf("mutating call before the refusal: %s", c.route)
		}
	}
}

func TestRecreateRestoresTheOriginalWhenDockerRefusesTheNewOne(t *testing.T) {
	svc, calls := recreateDaemon(t, "10.9.0.0/16")
	_, err := svc.Recreate(context.Background(), "env", "net-old", lanRequest("10.9.0.0/16"))
	if !errors.Is(err, ErrRecreateRestored) {
		t.Fatalf("err = %v, want restored", err)
	}
	creates := 0
	for _, c := range calls() {
		if c.route == "POST /networks/create" {
			creates++
		}
	}
	if creates != 2 {
		t.Errorf("creates = %d, want the refused one and the restore", creates)
	}
	bodies := connectBodies(calls(), "net-new")
	if len(bodies) != 1 || !strings.Contains(bodies[0], `"IPv4Address":"192.168.1.50"`) {
		t.Errorf("the container must come back on its old address after a restore: %v", bodies)
	}
}
