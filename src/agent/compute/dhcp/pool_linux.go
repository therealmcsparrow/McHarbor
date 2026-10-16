// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package dhcp

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// offerGrace is how long an address stays reserved between OFFER and REQUEST.
//
// Without it, two guests booting together are offered the same address: the
// first has not committed yet, so a naive free-address scan hands the second
// the same one and both ACK it.
const offerGrace = 2 * time.Minute

// pool allocates addresses for one network and persists what it handed out.
type pool struct {
	mu       sync.Mutex
	network  Network
	path     string
	start    uint32
	end      uint32
	reserved map[uint32]struct{}
	byMAC    map[string]*record
}

// newPool builds a pool over a network's range, restoring any leases the agent
// handed out before it restarted.
//
// Restoring matters more than it looks: a guest that renews after an agent
// restart must get the same address back, or it changes address while running
// and every connection to it breaks.
func newPool(network Network, path string) (*pool, error) {
	start, err := toUint32(network.RangeStart)
	if err != nil {
		return nil, err
	}
	end, err := toUint32(network.RangeEnd)
	if err != nil {
		return nil, err
	}
	if end < start {
		return nil, fmt.Errorf("dhcp: range %v-%v runs backwards", network.RangeStart, network.RangeEnd)
	}

	p := &pool{
		network:  network,
		path:     path,
		start:    start,
		end:      end,
		reserved: make(map[uint32]struct{}),
		byMAC:    make(map[string]*record),
	}
	if gateway, err := toUint32(network.ServerIP); err == nil {
		p.reserved[gateway] = struct{}{}
	}
	for _, stored := range loadRecords(path) {
		entry := stored
		if ip := net.ParseIP(entry.IP); ip != nil {
			if value, err := toUint32(ip); err == nil && value >= start && value <= end {
				p.byMAC[entry.MAC] = &entry
			}
		}
	}
	return p, nil
}

// offer reserves an address for a client that has not committed yet.
func (p *pool) offer(mac string) (net.IP, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, ok := p.byMAC[mac]; ok && !existing.Declined {
		existing.ExpiresAt = maxTime(existing.ExpiresAt, time.Now().Add(offerGrace))
		return net.ParseIP(existing.IP), nil
	}

	value, err := p.freeLocked()
	if err != nil {
		return nil, err
	}
	ip := fromUint32(value)
	p.byMAC[mac] = &record{MAC: mac, IP: ip.String(), ExpiresAt: time.Now().Add(offerGrace)}
	return ip, nil
}

// commit turns an offer into a lease, or refuses an address the client cannot
// have. It reports whether this was a renewal rather than a first assignment.
func (p *pool) commit(mac string, requested net.IP, hostname string, lease time.Duration) (net.IP, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	existing, held := p.byMAC[mac]
	if requested != nil {
		value, err := toUint32(requested)
		if err != nil || value < p.start || value > p.end {
			return nil, false, fmt.Errorf("dhcp: %v is outside this network's range", requested)
		}
		if owner, taken := p.ownerLocked(value); taken && owner != mac {
			return nil, false, fmt.Errorf("dhcp: %v is leased to %s", requested, owner)
		}
		if !held || existing.IP != requested.String() {
			p.byMAC[mac] = &record{MAC: mac, IP: requested.String()}
			existing = p.byMAC[mac]
			held = false
		}
	}
	if existing == nil {
		value, err := p.freeLocked()
		if err != nil {
			return nil, false, err
		}
		p.byMAC[mac] = &record{MAC: mac, IP: fromUint32(value).String()}
		existing = p.byMAC[mac]
		held = false
	}

	renewed := held && existing.Committed &&
		!existing.ExpiresAt.IsZero() && existing.ExpiresAt.After(time.Now())
	existing.Hostname = hostname
	existing.Declined = false
	existing.Committed = true
	existing.ExpiresAt = time.Now().Add(lease)
	errlog.Log(p.saveLocked())
	return net.ParseIP(existing.IP), renewed, nil
}

// release frees a client's address and returns it, or "" when it held none.
func (p *pool) release(mac string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	existing, ok := p.byMAC[mac]
	if !ok {
		return ""
	}
	delete(p.byMAC, mac)
	errlog.Log(p.saveLocked())
	return existing.IP
}

// decline marks a client's address unusable.
//
// The record is kept rather than deleted: the address is in use by something
// that is not a McHarbor guest, so it must stay out of the pool until someone
// removes the network.
func (p *pool) decline(mac string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, ok := p.byMAC[mac]; ok {
		existing.Declined = true
		existing.ExpiresAt = time.Time{}
		errlog.Log(p.saveLocked())
	}
}

// expire drops leases whose time has passed and returns what it dropped.
func (p *pool) expire() []record {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	var dropped []record
	for mac, entry := range p.byMAC {
		if entry.Declined || entry.ExpiresAt.IsZero() || entry.ExpiresAt.After(now) {
			continue
		}
		dropped = append(dropped, *entry)
		delete(p.byMAC, mac)
	}
	if len(dropped) > 0 {
		errlog.Log(p.saveLocked())
	}
	return dropped
}

// save flushes the lease table.
func (p *pool) save() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.saveLocked()
}

// rangeString describes the pool for a log line.
func (p *pool) rangeString() string {
	return fmt.Sprintf("%v-%v", fromUint32(p.start), fromUint32(p.end))
}

// freeLocked finds the lowest unused address in the range.
func (p *pool) freeLocked() (uint32, error) {
	taken := make(map[uint32]struct{}, len(p.byMAC)+len(p.reserved))
	for value := range p.reserved {
		taken[value] = struct{}{}
	}
	now := time.Now()
	for _, entry := range p.byMAC {
		ip := net.ParseIP(entry.IP)
		if ip == nil {
			continue
		}
		value, err := toUint32(ip)
		if err != nil {
			continue
		}
		// An expired lease is free, but a declined one never is.
		if entry.Declined || entry.ExpiresAt.IsZero() || entry.ExpiresAt.After(now) {
			taken[value] = struct{}{}
		}
	}
	for value := p.start; value <= p.end; value++ {
		if _, used := taken[value]; !used {
			return value, nil
		}
	}
	return 0, fmt.Errorf("dhcp: every address in %s is in use", p.rangeString())
}

// ownerLocked reports which client holds an address, if any.
func (p *pool) ownerLocked(value uint32) (string, bool) {
	now := time.Now()
	for mac, entry := range p.byMAC {
		ip := net.ParseIP(entry.IP)
		if ip == nil {
			continue
		}
		held, err := toUint32(ip)
		if err != nil || held != value {
			continue
		}
		if entry.Declined || entry.ExpiresAt.IsZero() || entry.ExpiresAt.After(now) {
			return mac, true
		}
	}
	return "", false
}

// saveLocked writes the lease table through a temporary file, so a crash
// mid-write leaves the previous table rather than a truncated one.
func (p *pool) saveLocked() error {
	state := persisted{Leases: make([]record, 0, len(p.byMAC))}
	for _, entry := range p.byMAC {
		state.Leases = append(state.Leases, *entry)
	}
	sort.Slice(state.Leases, func(i, j int) bool { return state.Leases[i].IP < state.Leases[j].IP })

	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("dhcp: encoding leases: %w", err)
	}
	temp := p.path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return fmt.Errorf("dhcp: writing leases: %w", err)
	}
	if err := os.Rename(temp, p.path); err != nil {
		return fmt.Errorf("dhcp: replacing leases: %w", err)
	}
	return nil
}

// deriveRange picks a pool from a subnet, skipping the network address, the
// broadcast address and the gateway.
func deriveRange(subnet *net.IPNet, gateway net.IP) (net.IP, net.IP) {
	base, err := toUint32(subnet.IP.Mask(subnet.Mask))
	if err != nil {
		return nil, nil
	}
	ones, bits := subnet.Mask.Size()
	if bits != 32 || ones > 30 {
		return nil, nil
	}
	size := uint32(1) << uint(bits-ones)
	first := base + 1
	last := base + size - 2

	// A gateway at the bottom of the subnet is the convention McHarbor's own
	// defaults follow, so stepping over it keeps the pool contiguous.
	if value, err := toUint32(gateway); err == nil && value == first {
		first++
	}
	if first > last {
		return nil, nil
	}
	return fromUint32(first), fromUint32(last)
}

// toUint32 converts an IPv4 address to its numeric form.
func toUint32(ip net.IP) (uint32, error) {
	v4 := ip.To4()
	if v4 == nil {
		return 0, fmt.Errorf("dhcp: %v is not an IPv4 address", ip)
	}
	return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3]), nil
}

// fromUint32 converts a numeric address back to an IP.
func fromUint32(value uint32) net.IP {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

// maxTime returns the later of two times.
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
