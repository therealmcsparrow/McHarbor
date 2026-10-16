// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package dhcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// sweepInterval is how often expired leases are returned to the pool. Half the
// shortest lease is frequent enough to keep a small range usable and rare
// enough to stay invisible.
const sweepInterval = 15 * time.Minute

// Manager runs one DHCP server per managed network.
//
// Servers are keyed by network id rather than by bridge, because a network can
// be edited to a different subnet on the same bridge and the old server must
// stop offering the old range before the new one starts.
type Manager struct {
	stateDir string
	onLease  func(Lease)
	logger   *slog.Logger

	mu      sync.Mutex
	servers map[string]*instance
	closed  bool
}

// instance is one running server and the state it hands out.
type instance struct {
	network  Network
	server   *server4.Server
	pool     *pool
	stopOnce sync.Once
	done     chan struct{}
}

// NewManager returns a Manager persisting leases under stateDir.
//
// onLease may be nil and is called from the server's own goroutine, so it must
// not block: it sits on the path between a guest's REQUEST and its ACK.
func NewManager(stateDir string, onLease func(Lease), logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		stateDir: stateDir,
		onLease:  onLease,
		logger:   logger,
		servers:  make(map[string]*instance),
	}
}

// Ensure starts serving a network, replacing any server already running for it.
//
// Called on every network create and on agent start, so it is idempotent by
// construction: an unchanged network is left running rather than restarted,
// because a restart drops in-flight exchanges.
func (m *Manager) Ensure(network Network) error {
	if err := validate(&network); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("dhcp: manager is shut down")
	}

	if existing, ok := m.servers[network.NetworkID]; ok {
		if existing.network.equal(network) {
			return nil
		}
		existing.stop()
		delete(m.servers, network.NetworkID)
	}

	if err := os.MkdirAll(m.stateDir, 0o750); err != nil {
		return fmt.Errorf("dhcp: lease directory: %w", err)
	}
	leases, err := newPool(network, m.leaseFile(network.NetworkID))
	if err != nil {
		return err
	}

	inst := &instance{network: network, pool: leases, done: make(chan struct{})}

	// Bound to the bridge, so a host with several managed networks runs
	// several servers on port 67 without them answering each other's guests.
	server, err := server4.NewServer(network.Bridge, &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: dhcpv4.ServerPort,
	}, m.handler(inst))
	if err != nil {
		return fmt.Errorf("dhcp: listening on %s: %w", network.Bridge, err)
	}
	inst.server = server

	go func() {
		defer close(inst.done)
		if err := server.Serve(); err != nil {
			m.logger.Debug("dhcp server stopped", "network", network.NetworkID, "error", err)
		}
	}()
	go m.sweep(inst)

	m.servers[network.NetworkID] = inst
	// Written only once the server is up, so a config on disk always describes
	// something that worked at least once. It is what lets RestoreAll bring the
	// network back after the agent restarts: the bridge and the taps are kernel
	// state that outlives the process, and until this existed the DHCP server
	// was the one piece that did not.
	if err := m.saveNetwork(network); err != nil {
		m.logger.Warn("dhcp network state not saved; it will not survive an agent restart",
			"network", network.NetworkID, "error", err)
	}
	m.logger.Info("dhcp serving managed network",
		"network", network.NetworkID, "bridge", network.Bridge, "range", leases.rangeString())
	return nil
}

// RestoreAll starts a server for every network this host was serving when it
// last ran. Networks whose bridge has since gone are reported and skipped.
func (m *Manager) RestoreAll() (int, error) {
	entries, err := filepath.Glob(filepath.Join(m.stateDir, "network-*.json"))
	if err != nil {
		return 0, fmt.Errorf("dhcp: reading saved networks: %w", err)
	}

	restored := 0
	for _, path := range entries {
		network, err := readNetworkFile(path)
		if err != nil {
			m.logger.Warn("dhcp saved network could not be read", "path", path, "error", err)
			continue
		}
		if err := m.Ensure(network); err != nil {
			m.logger.Warn("dhcp network could not be restored",
				"network", network.NetworkID, "bridge", network.Bridge, "error", err)
			continue
		}
		restored++
	}
	return restored, nil
}

// networkFile is where one network's addressing persists across agent restarts.
func (m *Manager) networkFile(networkID string) string {
	return filepath.Join(m.stateDir, "network-"+networkID+".json")
}

// savedNetwork is the on-disk shape. Plain strings, because net.IPNet does not
// round-trip through encoding/json in any form worth depending on.
type savedNetwork struct {
	NetworkID  string   `json:"networkId"`
	Bridge     string   `json:"bridge"`
	ServerIP   string   `json:"serverIp"`
	Subnet     string   `json:"subnet"`
	RangeStart string   `json:"rangeStart,omitempty"`
	RangeEnd   string   `json:"rangeEnd,omitempty"`
	LeaseTime  string   `json:"leaseTime,omitempty"`
	DNS        []string `json:"dns,omitempty"`
}

func (m *Manager) saveNetwork(network Network) error {
	saved := savedNetwork{
		NetworkID: network.NetworkID,
		Bridge:    network.Bridge,
		ServerIP:  network.ServerIP.String(),
		Subnet:    network.Subnet.String(),
		LeaseTime: network.LeaseTime.String(),
	}
	if network.RangeStart != nil {
		saved.RangeStart = network.RangeStart.String()
	}
	if network.RangeEnd != nil {
		saved.RangeEnd = network.RangeEnd.String()
	}
	for _, dns := range network.DNS {
		saved.DNS = append(saved.DNS, dns.String())
	}

	payload, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("dhcp: encoding network state: %w", err)
	}
	if err := os.MkdirAll(m.stateDir, 0o750); err != nil {
		return fmt.Errorf("dhcp: network state directory: %w", err)
	}
	if err := os.WriteFile(m.networkFile(network.NetworkID), payload, 0o640); err != nil {
		return fmt.Errorf("dhcp: writing network state: %w", err)
	}
	return nil
}

func readNetworkFile(path string) (Network, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Network{}, err
	}
	var saved savedNetwork
	if err := json.Unmarshal(data, &saved); err != nil {
		return Network{}, err
	}

	_, subnet, err := net.ParseCIDR(saved.Subnet)
	if err != nil {
		return Network{}, fmt.Errorf("subnet %q: %w", saved.Subnet, err)
	}
	network := Network{
		NetworkID:  saved.NetworkID,
		Bridge:     saved.Bridge,
		ServerIP:   net.ParseIP(saved.ServerIP),
		Subnet:     subnet,
		RangeStart: net.ParseIP(saved.RangeStart),
		RangeEnd:   net.ParseIP(saved.RangeEnd),
	}
	if saved.LeaseTime != "" {
		if lease, err := time.ParseDuration(saved.LeaseTime); err == nil {
			network.LeaseTime = lease
		}
	}
	for _, dns := range saved.DNS {
		if parsed := net.ParseIP(dns); parsed != nil {
			network.DNS = append(network.DNS, parsed)
		}
	}
	return network, nil
}

// Remove stops serving a network and forgets its leases.
func (m *Manager) Remove(networkID string) error {
	m.mu.Lock()
	inst, ok := m.servers[networkID]
	delete(m.servers, networkID)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	inst.stop()
	if err := os.Remove(m.leaseFile(networkID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("dhcp: removing lease state: %w", err)
	}
	if err := os.Remove(m.networkFile(networkID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("dhcp: removing network state: %w", err)
	}
	return nil
}

// Serving reports whether a network currently has a server.
func (m *Manager) Serving(networkID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.servers[networkID]
	return ok
}

// Close stops every server.
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	servers := make([]*instance, 0, len(m.servers))
	for _, inst := range m.servers {
		servers = append(servers, inst)
	}
	m.servers = make(map[string]*instance)
	m.mu.Unlock()

	for _, inst := range servers {
		inst.stop()
	}
	return nil
}

// leaseFile is where one network's leases persist across agent restarts.
func (m *Manager) leaseFile(networkID string) string {
	return filepath.Join(m.stateDir, "dhcp-"+networkID+".json")
}

// handler builds the packet handler for one network.
func (m *Manager) handler(inst *instance) server4.Handler {
	return func(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
		if req == nil {
			return
		}
		reply, lease := m.respond(inst, req)
		if reply != nil {
			if _, err := conn.WriteTo(reply.ToBytes(), replyAddr(req, peer, reply)); err != nil {
				m.logger.Debug("dhcp reply failed", "network", inst.network.NetworkID,
					"mac", req.ClientHWAddr.String(), "error", err)
				return
			}
		}
		if lease != nil && m.onLease != nil {
			m.onLease(*lease)
		}
	}
}

// respond produces the reply for one request, and the lease to report when the
// exchange actually assigned or freed an address.
func (m *Manager) respond(inst *instance, req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, *Lease) {
	mac := req.ClientHWAddr.String()
	network := inst.network

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		ip, err := inst.pool.offer(mac)
		if err != nil {
			m.logger.Warn("dhcp pool exhausted",
				"network", network.NetworkID, "mac", mac, "error", err)
			return nil, nil
		}
		return m.buildReply(network, req, ip, dhcpv4.MessageTypeOffer), nil

	case dhcpv4.MessageTypeRequest:
		ip, renewed, err := inst.pool.commit(mac, requestedIP(req), hostname(req), leaseTime(network))
		if err != nil {
			// A NAK is the honest answer to "I want an address you cannot
			// have": the guest restarts discovery instead of retrying an
			// address that will never work.
			return m.buildNak(network, req), nil
		}
		action := ActionGranted
		if renewed {
			action = ActionRenewed
		}
		return m.buildReply(network, req, ip, dhcpv4.MessageTypeAck), &Lease{
			NetworkID: network.NetworkID,
			MAC:       mac,
			IP:        ip.String(),
			Hostname:  hostname(req),
			Action:    action,
			ExpiresAt: time.Now().Add(leaseTime(network)),
		}

	case dhcpv4.MessageTypeRelease:
		released := inst.pool.release(mac)
		if released == "" {
			return nil, nil
		}
		return nil, &Lease{
			NetworkID: network.NetworkID,
			MAC:       mac,
			IP:        released,
			Action:    ActionReleased,
		}

	case dhcpv4.MessageTypeDecline:
		// The guest found the address already in use, so it is poisoned rather
		// than free: handing it to the next guest reproduces the conflict.
		inst.pool.decline(mac)
		return nil, nil

	default:
		return nil, nil
	}
}

// buildReply assembles an OFFER or an ACK.
func (m *Manager) buildReply(network Network, req *dhcpv4.DHCPv4, ip net.IP, kind dhcpv4.MessageType) *dhcpv4.DHCPv4 {
	reply, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		m.logger.Debug("dhcp reply build failed", "network", network.NetworkID, "error", err)
		return nil
	}
	reply.YourIPAddr = ip
	reply.ServerIPAddr = network.ServerIP
	reply.UpdateOption(dhcpv4.OptMessageType(kind))
	reply.UpdateOption(dhcpv4.OptServerIdentifier(network.ServerIP))
	reply.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime(network)))
	reply.UpdateOption(dhcpv4.OptSubnetMask(network.Subnet.Mask))
	reply.UpdateOption(dhcpv4.OptRouter(network.ServerIP))
	reply.UpdateOption(dhcpv4.OptDNS(resolvers(network)...))
	return reply
}

// buildNak assembles a NAK.
func (m *Manager) buildNak(network Network, req *dhcpv4.DHCPv4) *dhcpv4.DHCPv4 {
	reply, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil
	}
	reply.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
	reply.UpdateOption(dhcpv4.OptServerIdentifier(network.ServerIP))
	return reply
}

// sweep returns expired leases to the pool until the server stops.
func (m *Manager) sweep(inst *instance) {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-inst.done:
			return
		case <-ticker.C:
			for _, expired := range inst.pool.expire() {
				if m.onLease != nil {
					m.onLease(Lease{
						NetworkID: inst.network.NetworkID,
						MAC:       expired.MAC,
						IP:        expired.IP,
						Action:    ActionExpired,
					})
				}
			}
		}
	}
}

// stop shuts one server down exactly once and waits for its goroutine.
func (i *instance) stop() {
	i.stopOnce.Do(func() {
		if i.server != nil {
			errlog.Close(i.server)
		}
		if i.pool != nil {
			errlog.Log(i.pool.save())
		}
	})
	<-i.done
}

// equal reports whether two definitions would serve identically, so an
// unchanged network is not restarted.
func (n Network) equal(other Network) bool {
	if n.Bridge != other.Bridge || !n.ServerIP.Equal(other.ServerIP) {
		return false
	}
	if n.Subnet == nil || other.Subnet == nil || n.Subnet.String() != other.Subnet.String() {
		return false
	}
	if !n.RangeStart.Equal(other.RangeStart) || !n.RangeEnd.Equal(other.RangeEnd) {
		return false
	}
	return leaseTime(n) == leaseTime(other)
}

// leaseTime resolves a network's lease duration.
func leaseTime(n Network) time.Duration {
	if n.LeaseTime > 0 {
		return n.LeaseTime
	}
	return DefaultLeaseTime
}

// resolvers picks the DNS servers offered to guests.
func resolvers(n Network) []net.IP {
	if len(n.DNS) > 0 {
		return n.DNS
	}
	return []net.IP{n.ServerIP}
}

// requestedIP reads the address a client wants, from either the option or the
// ciaddr field depending on which renewal state it is in.
func requestedIP(req *dhcpv4.DHCPv4) net.IP {
	if ip := req.RequestedIPAddress(); ip != nil && !ip.IsUnspecified() {
		return ip
	}
	if req.ClientIPAddr != nil && !req.ClientIPAddr.IsUnspecified() {
		return req.ClientIPAddr
	}
	return nil
}

// hostname returns the guest-supplied name, which is untrusted.
func hostname(req *dhcpv4.DHCPv4) string {
	return req.HostName()
}

// replyAddr picks where to send a reply.
//
// A client with no address yet cannot receive a unicast reply, so a broadcast
// request is answered on the broadcast address; anything else goes back to the
// peer the request came from.
func replyAddr(req *dhcpv4.DHCPv4, peer net.Addr, reply *dhcpv4.DHCPv4) net.Addr {
	if req.GatewayIPAddr != nil && !req.GatewayIPAddr.IsUnspecified() {
		return &net.UDPAddr{IP: req.GatewayIPAddr, Port: dhcpv4.ServerPort}
	}
	if req.ClientIPAddr != nil && !req.ClientIPAddr.IsUnspecified() {
		return &net.UDPAddr{IP: req.ClientIPAddr, Port: dhcpv4.ClientPort}
	}
	if req.IsBroadcast() || reply.YourIPAddr == nil {
		return &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
	}
	if udp, ok := peer.(*net.UDPAddr); ok && udp.IP != nil && !udp.IP.IsUnspecified() {
		return udp
	}
	return &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
}

// validate fills in what the caller left out and rejects what cannot work.
func validate(n *Network) error {
	if n.NetworkID == "" || n.Bridge == "" {
		return fmt.Errorf("dhcp: a network needs an id and a bridge")
	}
	if n.Subnet == nil {
		return fmt.Errorf("dhcp: network %s has no subnet", n.NetworkID)
	}
	if n.ServerIP == nil || !n.Subnet.Contains(n.ServerIP) {
		return fmt.Errorf("dhcp: gateway %v is not inside %s", n.ServerIP, n.Subnet)
	}
	if n.RangeStart == nil || n.RangeEnd == nil {
		n.RangeStart, n.RangeEnd = deriveRange(n.Subnet, n.ServerIP)
	}
	if n.RangeStart == nil || n.RangeEnd == nil {
		return fmt.Errorf("dhcp: %s is too small to hand out addresses", n.Subnet)
	}
	return nil
}

// persisted is the on-disk lease file.
type persisted struct {
	Leases []record `json:"leases"`
}

// record is one stored lease.
type record struct {
	MAC       string    `json:"mac"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
	Declined  bool      `json:"declined,omitempty"`
	// Committed separates a lease the client actually took from an address
	// merely reserved between OFFER and REQUEST. Without it a first ACK looks
	// like a renewal, because the offer already put a future expiry on the
	// record.
	Committed bool `json:"committed,omitempty"`
}

// loadRecords reads persisted leases, treating an unreadable file as an empty
// one: losing lease history hands out a different address, which a guest
// recovers from, while refusing to start hands out none at all.
func loadRecords(path string) []record {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state persisted
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil
	}
	return state.Leases
}
