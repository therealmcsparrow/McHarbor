// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
)

// consoleInputBuffer bounds the frames queued for one console; a client that
// outruns the simulated guest loses keystrokes rather than wedging the agent.
const consoleInputBuffer = 128

// guestView is the snapshot of a guest a console renders, taken once at open
// so the session never reads simulator state without the lock.
type guestView struct {
	kind      string
	name      string
	family    string
	osName    string
	kernel    string
	ips       []string
	vcpus     int
	memoryMB  int64
	startedAt time.Time
	hostName  string
}

type consoleSession struct {
	id     string
	in     chan []byte
	cancel context.CancelFunc
	once   sync.Once
}

func (c *consoleSession) stop() { c.once.Do(c.cancel) }

func (s *Sim) handleConsole(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send fakeagent.Sender) {
	switch msg.Type {
	case coreagent.MsgConsoleStart:
		s.openConsole(ctx, host, msg, send)
	case coreagent.MsgConsoleInput:
		if msg.Console == nil || len(msg.Console.Data) == 0 {
			return
		}
		s.mu.Lock()
		sess := s.sessions[msg.ID]
		s.mu.Unlock()
		if sess == nil {
			return
		}
		data := append([]byte(nil), msg.Console.Data...)
		select {
		case sess.in <- data:
		default:
			s.logger.Debug("demo console input dropped", "session", msg.ID)
		}
	case coreagent.MsgConsoleEnd:
		s.mu.Lock()
		sess := s.sessions[msg.ID]
		delete(s.sessions, msg.ID)
		s.mu.Unlock()
		if sess != nil {
			sess.stop()
		}
	}
}

func consoleEnd(id, reason string) *coreagent.WSMessage {
	return &coreagent.WSMessage{Type: coreagent.MsgConsoleEnd, ID: id, Console: &coreagent.ConsolePayload{SessionID: id, Error: reason}}
}

func (s *Sim) openConsole(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send fakeagent.Sender) {
	req := msg.Console
	if req == nil {
		req = &coreagent.ConsolePayload{}
	}
	view, reason := s.viewFor(host, req.GuestKind, req.TargetID)
	if reason == "" && req.Kind == coreagent.ConsoleKindSPICE {
		reason = "SPICE consoles are not available in demo mode; open the VNC console instead."
	}
	if reason != "" {
		s.deliver(send, consoleEnd(msg.ID, reason))
		return
	}
	sessCtx, cancel := context.WithCancel(ctx)
	sess := &consoleSession{id: msg.ID, in: make(chan []byte, consoleInputBuffer), cancel: cancel}
	s.mu.Lock()
	s.sessions[msg.ID] = sess
	s.mu.Unlock()

	write := func(data []byte) bool {
		out := &coreagent.WSMessage{Type: coreagent.MsgConsoleOutput, ID: msg.ID, Console: &coreagent.ConsolePayload{SessionID: msg.ID, Data: data}}
		if err := send(out); err != nil {
			sess.stop()
			return false
		}
		return true
	}
	finish := func(reason string) {
		s.mu.Lock()
		_, live := s.sessions[msg.ID]
		delete(s.sessions, msg.ID)
		s.mu.Unlock()
		if live {
			s.deliver(send, consoleEnd(msg.ID, reason))
		}
		sess.stop()
	}
	go func() {
		defer func() {
			s.mu.Lock()
			if s.sessions[msg.ID] == sess {
				delete(s.sessions, msg.ID)
			}
			s.mu.Unlock()
			sess.stop()
		}()
		if req.Kind == coreagent.ConsoleKindVNC {
			runRFB(sessCtx, view, sess.in, write, finish)
			return
		}
		runText(sessCtx, view, req.Kind, sess.in, write, finish)
	}()
}

// viewFor snapshots the guest a console targets, or says why it cannot open.
func (s *Sim) viewFor(host *estate.Host, guestKind, id string) (guestView, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hs := s.hostFor(host)
	if hs == nil {
		return guestView{}, "This demo host runs no guests."
	}
	if guestKind == "ct" {
		c, ok := hs.cts[id]
		if !ok {
			return guestView{}, "The container is not present on this demo host."
		}
		if c.State != stateRunning && c.State != stateFrozen {
			return guestView{}, "The container is not running."
		}
		v := guestView{kind: "ct", name: c.Name, family: "linux", osName: c.OS.pretty, kernel: hs.host.Kernel,
			vcpus: c.CPUs, memoryMB: c.MemoryMax >> 20, startedAt: c.StartedAt, hostName: hs.host.Hostname}
		for _, n := range c.NICs {
			v.ips = append(v.ips, n.IP)
		}
		return v, ""
	}
	g, ok := hs.vms[id]
	if !ok {
		return guestView{}, "The virtual machine is not present on this demo host."
	}
	if g.State != stateRunning && g.State != statePaused {
		return guestView{}, "The virtual machine is not running."
	}
	v := guestView{kind: "vm", name: g.Name, family: g.OS.family, osName: g.OS.name, kernel: g.OS.kernel,
		vcpus: g.VCPUs, memoryMB: g.MemoryMB, startedAt: g.StartedAt, hostName: hs.host.Hostname}
	for _, n := range g.NICs {
		if n.IP != "" {
			v.ips = append(v.ips, n.IP)
		}
	}
	return v, ""
}

// textConsole is a line-editing serial or tty console with a login step for
// VMs and a small set of shell commands, labelled as a demo throughout.
type textConsole struct {
	view     guestView
	stage    int
	user     string
	line     []byte
	escaping bool
	write    func([]byte) bool
}

const (
	stageLogin = iota
	stagePassword
	stageShell
)

func runText(ctx context.Context, view guestView, kind string, in <-chan []byte, write func([]byte) bool, finish func(string)) {
	t := &textConsole{view: view, write: write, user: "root"}
	banner := "\r\n\x1b[33mMcHarbor demo mode - simulated " + kind + " console for " + view.name + ". Nothing here is real.\x1b[0m\r\n\r\n"
	if view.kind == "vm" {
		t.stage = stageLogin
		banner += view.osName + " " + view.name + " ttyS0\r\n\r\n"
		if view.family == "windows" {
			banner = "\r\n\x1b[33mMcHarbor demo mode - simulated Special Administration Console for " + view.name + ".\x1b[0m\r\n\r\nComputer is booting, SAC started and initialized.\r\n\r\n"
			t.stage, t.user = stageShell, "SAC"
		}
	} else {
		t.stage = stageShell
	}
	if !t.out(banner + t.prompt()) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-in:
			if !t.feed(data) {
				finish("")
				return
			}
		}
	}
}

func (t *textConsole) out(s string) bool { return t.write([]byte(s)) }

func (t *textConsole) prompt() string {
	switch t.stage {
	case stageLogin:
		return t.view.name + " login: "
	case stagePassword:
		return "Password: "
	}
	if t.user == "SAC" {
		return "SAC>"
	}
	mark := "#"
	if t.user != "root" {
		mark = "$"
	}
	return fmt.Sprintf("\x1b[1;32m%s@%s\x1b[0m:\x1b[1;34m~\x1b[0m%s ", t.user, t.view.name, mark)
}

// feed consumes raw terminal bytes and reports false once the user has exited.
func (t *textConsole) feed(data []byte) bool {
	for _, b := range data {
		if t.escaping {
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' {
				t.escaping = false
			}
			continue
		}
		switch {
		case b == 0x1b:
			t.escaping = true
		case b == '\r' || b == '\n':
			if !t.out("\r\n") {
				return false
			}
			line := strings.TrimSpace(string(t.line))
			t.line = t.line[:0]
			if !t.submit(line) {
				return false
			}
		case b == 0x7f || b == 0x08:
			if len(t.line) > 0 {
				t.line = t.line[:len(t.line)-1]
				if t.stage != stagePassword && !t.out("\b \b") {
					return false
				}
			}
		case b == 0x03:
			t.line = t.line[:0]
			if !t.out("^C\r\n" + t.prompt()) {
				return false
			}
		case b == 0x04:
			if len(t.line) == 0 && t.stage == stageShell {
				return false
			}
		case b >= 0x20 && b < 0x7f:
			t.line = append(t.line, b)
			if t.stage != stagePassword && !t.out(string(b)) {
				return false
			}
		}
	}
	return true
}

func (t *textConsole) submit(line string) bool {
	switch t.stage {
	case stageLogin:
		if line != "" {
			t.user = line
			t.stage = stagePassword
		}
		return t.out(t.prompt())
	case stagePassword:
		t.stage = stageShell
		return t.out("\r\nLast login: " + time.Now().Add(-26*time.Hour).UTC().Format("Mon Jan  2 15:04:05 2006") + " from 10.0.0.12 (demo)\r\n" + t.prompt())
	}
	if line == "exit" || line == "logout" {
		if t.view.kind == "vm" && t.user != "SAC" {
			t.stage, t.user = stageLogin, "root"
			return t.out("\r\n" + t.prompt())
		}
		return false
	}
	reply := t.run(line)
	if reply != "" {
		reply = strings.ReplaceAll(reply, "\n", "\r\n") + "\r\n"
	}
	return t.out(reply + t.prompt())
}

func (t *textConsole) run(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	v := t.view
	if t.user == "SAC" {
		switch strings.ToLower(fields[0]) {
		case "help", "?":
			return "ch   Channel management commands\ni    List network information\nid   Display computer identification\nrestart  (not available in demo mode)"
		case "id":
			return "Computer Name: " + strings.ToUpper(v.name) + "\nOS: " + v.osName + "\nDemo: yes"
		case "i":
			return "Net: " + strings.Join(v.ips, ", ")
		}
		return "Unrecognized command. Type ? for help. (demo console)"
	}
	uptime := time.Since(v.startedAt).Truncate(time.Minute)
	switch fields[0] {
	case "help":
		return "Demo console. Try: whoami hostname uname -a uptime ip a df -h free -h cat /etc/os-release ps date clear exit"
	case "whoami":
		return t.user
	case "hostname":
		return v.name
	case "uname":
		if len(fields) > 1 && fields[1] == "-a" {
			return "Linux " + v.name + " " + v.kernel + " #1 SMP PREEMPT_DYNAMIC x86_64 GNU/Linux"
		}
		return "Linux"
	case "uptime":
		return fmt.Sprintf(" %s up %s,  1 user,  load average: 0.42, 0.37, 0.31", time.Now().Format("15:04:05"), uptime)
	case "ip":
		out := "1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536\n    inet 127.0.0.1/8 scope host lo"
		for i, ip := range v.ips {
			out += fmt.Sprintf("\n%d: eth%d: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500\n    inet %s/24 scope global eth%d", i+2, i, ip, i)
		}
		return out
	case "df":
		return "Filesystem      Size  Used Avail Use% Mounted on\n/dev/vda1        78G   31G   44G  42% /\ntmpfs           2.0G     0  2.0G   0% /dev/shm"
	case "free":
		return fmt.Sprintf("               total        used        free\nMem:           %dMi       %dMi       %dMi", v.memoryMB, v.memoryMB*2/5, v.memoryMB*3/5)
	case "cat":
		if len(fields) > 1 && fields[1] == "/etc/os-release" {
			return "PRETTY_NAME=\"" + v.osName + "\"\nDEMO=\"simulated by McHarbor\""
		}
		return "cat: " + strings.Join(fields[1:], " ") + ": No such file or directory"
	case "ps":
		return "  PID TTY          TIME CMD\n    1 ?        00:00:03 init\n  412 ttyS0    00:00:00 bash\n  977 ttyS0    00:00:00 ps"
	case "ls":
		return ""
	case "date":
		return time.Now().UTC().Format("Mon Jan  2 15:04:05 UTC 2006")
	case "clear":
		return "\x1b[H\x1b[2J"
	}
	return fields[0] + ": command not found (demo console)"
}
