// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fakeagent

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// shell is a line-editing terminal that answers a small set of commands from
// the simulated host, so a terminal tab in demo mode behaves like one.
type shell struct {
	mu        sync.Mutex
	host      *estate.Host
	container *estate.Container
	user      string
	cwd       string
	line      []byte
	write     func([]byte)
	end       func()
	closed    bool
}

func newShell(host *estate.Host, c *estate.Container, write func([]byte), end func()) *shell {
	sh := &shell{host: host, container: c, write: write, end: end, user: "root", cwd: "/"}
	if c == nil {
		sh.user = "admin"
		sh.cwd = "/home/admin"
	}
	return sh
}

func (sh *shell) hostname() string {
	if sh.container != nil {
		return sh.container.Hostname
	}
	return strings.Split(sh.host.Hostname, ".")[0]
}

func (sh *shell) prompt() string {
	cwd := sh.cwd
	if sh.user == "admin" && strings.HasPrefix(cwd, "/home/admin") {
		cwd = "~" + strings.TrimPrefix(cwd, "/home/admin")
	}
	mark := "#"
	if sh.user != "root" {
		mark = "$"
	}
	return fmt.Sprintf("\x1b[1;32m%s@%s\x1b[0m:\x1b[1;34m%s\x1b[0m%s ", sh.user, sh.hostname(), cwd, mark)
}

func (sh *shell) greet() {
	target := "host " + sh.host.Name
	if sh.container != nil {
		target = "container " + sh.container.Name
	}
	sh.out("\x1b[33mMcHarbor demo mode — this is a simulated shell on " + target + ". Nothing here is real.\x1b[0m\r\nType 'help' for the commands it understands.\r\n\r\n" + sh.prompt())
}

func (sh *shell) out(s string) { sh.write([]byte(s)) }

func (sh *shell) close() {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.closed = true
}

// input feeds raw terminal bytes: printable runes echo, backspace edits,
// Enter runs the line, Ctrl-C abandons it and Ctrl-D on an empty line exits.
func (sh *shell) input(data []byte) {
	sh.mu.Lock()
	if sh.closed {
		sh.mu.Unlock()
		return
	}
	var echo bytes.Buffer
	var run []string
	exit := false
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch {
		case b == '\r' || b == '\n':
			echo.WriteString("\r\n")
			run = append(run, string(sh.line))
			sh.line = sh.line[:0]
		case b == 0x7f || b == 0x08:
			if len(sh.line) > 0 {
				sh.line = sh.line[:len(sh.line)-1]
				echo.WriteString("\b \b")
			}
		case b == 0x03:
			sh.line = sh.line[:0]
			echo.WriteString("^C\r\n" + sh.prompt())
		case b == 0x04:
			if len(sh.line) == 0 {
				exit = true
			}
		case b == 0x1b:
			for i+1 < len(data) && (data[i+1] == '[' || (data[i+1] >= '0' && data[i+1] <= '9') || data[i+1] == ';') {
				i++
			}
			if i+1 < len(data) {
				i++
			}
		case b >= 0x20:
			sh.line = append(sh.line, b)
			echo.WriteByte(b)
		}
	}
	sh.mu.Unlock()
	if echo.Len() > 0 {
		sh.write(echo.Bytes())
	}
	for _, cmd := range run {
		if sh.execute(strings.TrimSpace(cmd)) {
			exit = true
			break
		}
		sh.out(sh.prompt())
	}
	if exit {
		sh.out("logout\r\n")
		sh.close()
		sh.end()
	}
}

func (sh *shell) execute(cmd string) bool {
	if cmd == "" {
		return false
	}
	fields := strings.Fields(cmd)
	name, args := fields[0], fields[1:]
	now := time.Now()
	switch name {
	case "exit", "logout":
		return true
	case "help":
		sh.out("Commands: ls, cd, pwd, whoami, id, hostname, uname, cat /etc/os-release, ps, top, df, free, uptime, date, env, echo, nproc, ip, clear, exit\r\n")
		if sh.container == nil {
			sh.out("Host only: docker ps, systemctl status <unit>\r\n")
		}
	case "pwd":
		sh.out(sh.cwd + "\r\n")
	case "cd":
		switch {
		case len(args) == 0 || args[0] == "~":
			sh.cwd = map[bool]string{true: "/root", false: "/home/admin"}[sh.user == "root"]
		case args[0] == "..":
			if i := strings.LastIndex(strings.TrimSuffix(sh.cwd, "/"), "/"); i > 0 {
				sh.cwd = sh.cwd[:i]
			} else {
				sh.cwd = "/"
			}
		case strings.HasPrefix(args[0], "/"):
			sh.cwd = args[0]
		default:
			sh.cwd = strings.TrimSuffix(sh.cwd, "/") + "/" + args[0]
		}
	case "ls":
		sh.out(strings.Join(sh.listing(), "  ") + "\r\n")
	case "whoami":
		sh.out(sh.user + "\r\n")
	case "id":
		if sh.user == "root" {
			sh.out("uid=0(root) gid=0(root) groups=0(root)\r\n")
		} else {
			sh.out("uid=1000(admin) gid=1000(admin) groups=1000(admin),27(sudo),999(docker)\r\n")
		}
	case "hostname":
		sh.out(sh.hostname() + "\r\n")
	case "uname":
		if len(args) > 0 && args[0] == "-a" {
			sh.out(fmt.Sprintf("Linux %s %s #1 SMP PREEMPT_DYNAMIC %s %s GNU/Linux\r\n", sh.hostname(), sh.host.Kernel, now.Format("Mon Jan 2 15:04:05 UTC 2006"), sh.host.Arch))
		} else {
			sh.out("Linux\r\n")
		}
	case "cat":
		if len(args) > 0 && args[0] == "/etc/os-release" {
			sh.out(fmt.Sprintf("PRETTY_NAME=%q\r\nID=%s\r\nVERSION_ID=%q\r\n", prettyName(sh.host), sh.host.Distro, sh.host.OSVersion))
		} else if len(args) > 0 {
			sh.out("cat: " + args[0] + ": Permission denied (demo mode)\r\n")
		}
	case "ps", "top":
		sh.out("  PID USER     TIME  COMMAND\r\n")
		if sh.container != nil {
			sh.out(fmt.Sprintf("    1 root     1:12  %s\r\n", sh.container.Command))
			sh.out("   42 root     0:00  sh\r\n")
		} else {
			sh.out("    1 root     0:41  /sbin/init\r\n  612 root     2:17  /usr/bin/dockerd -H fd://\r\n  640 root     1:03  /usr/bin/containerd\r\n  901 root     0:12  /usr/local/bin/mcharbor-agent\r\n 1204 admin    0:00  -bash\r\n")
		}
	case "df":
		used := sh.host.DiskUsed >> 30
		total := sh.host.DiskTotal >> 30
		sh.out(fmt.Sprintf("Filesystem      Size  Used Avail Use%% Mounted on\r\n/dev/nvme0n1p2  %4dG %4dG %4dG  %2d%% /\r\n", total, used, total-used, used*100/max(total, 1)))
	case "free":
		hs := sh.host.SampleHost(now)
		sh.out(fmt.Sprintf("               total        used        free\r\nMem:     %11d %11d %11d\r\n", sh.host.MemTotal>>20, hs.MemUsed>>20, (sh.host.MemTotal-hs.MemUsed)>>20))
	case "uptime":
		hs := sh.host.SampleHost(now)
		sh.out(fmt.Sprintf(" %s up %d days,  load average: %.2f, %.2f, %.2f\r\n", now.Format("15:04:05"), hs.Uptime/86400, hs.Load1, hs.Load5, hs.Load15))
	case "nproc":
		sh.out(strconv.Itoa(sh.host.CPUs) + "\r\n")
	case "date":
		sh.out(now.UTC().Format("Mon Jan  2 15:04:05 UTC 2006") + "\r\n")
	case "env":
		if sh.container != nil {
			sh.out(strings.Join(sh.container.Env, "\r\n") + "\r\n")
		} else {
			sh.out("HOME=/home/admin\r\nSHELL=/bin/bash\r\nUSER=admin\r\n")
		}
	case "echo":
		sh.out(strings.Join(args, " ") + "\r\n")
	case "clear":
		sh.out("\x1b[2J\x1b[H")
	case "ip":
		sh.out("1: lo: <LOOPBACK,UP> mtu 65536\r\n    inet 127.0.0.1/8 scope host lo\r\n2: eth0: <BROADCAST,MULTICAST,UP> mtu 1500\r\n    inet " + sh.address() + " scope global eth0\r\n")
	case "docker":
		if sh.container != nil {
			sh.out("sh: docker: not found\r\n")
			break
		}
		sh.out("CONTAINER ID   IMAGE                                  STATUS          NAMES\r\n")
		ctrs := sh.host.Containers()
		for i, c := range ctrs {
			if i == 15 {
				sh.out(fmt.Sprintf("... and %d more\r\n", len(ctrs)-15))
				break
			}
			sh.out(fmt.Sprintf("%s   %-38s %-15s %s\r\n", c.ID[:12], trunc(c.Image, 38), string(c.State), c.Name))
		}
	case "systemctl":
		unit := "docker"
		if len(args) > 1 {
			unit = args[1]
		}
		sh.out(fmt.Sprintf("● %s.service\r\n     Loaded: loaded (/lib/systemd/system/%s.service; enabled)\r\n     Active: active (running) since %s\r\n", strings.TrimSuffix(unit, ".service"), strings.TrimSuffix(unit, ".service"), sh.host.BootedAt.Format(time.RFC1123)))
	default:
		sh.out("sh: " + name + ": command not found\r\n")
	}
	return false
}

func (sh *shell) address() string {
	if sh.container != nil {
		for _, ip := range sh.container.Networks {
			return ip + "/16"
		}
	}
	return hostIP(sh.host) + "/24"
}

func (sh *shell) listing() []string {
	if sh.container == nil {
		return []string{"backups", "compose", "notes.md", "scripts"}
	}
	dirs := []string{"bin", "dev", "etc", "home", "lib", "proc", "root", "run", "sys", "tmp", "usr", "var"}
	for _, m := range sh.container.Mounts {
		if top := strings.Split(strings.TrimPrefix(m.Destination, "/"), "/")[0]; top != "" {
			dirs = append(dirs, top)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func prettyName(h *estate.Host) string {
	switch h.Distro {
	case "ubuntu":
		return "Ubuntu " + h.OSVersion + " LTS"
	case "debian":
		return "Debian GNU/Linux " + h.OSVersion + " (bookworm)"
	case "rocky":
		return "Rocky Linux " + h.OSVersion + " (Blue Onyx)"
	case "fedora":
		return "Fedora Linux " + h.OSVersion + " (Server Edition)"
	case "alpine":
		return "Alpine Linux v" + h.OSVersion
	}
	return "Linux"
}

func itoa(n int) string { return strconv.Itoa(n) }

// handleExec runs an interactive container shell over the exec_* messages.
func (s *session) handleExec(msg *coreagent.WSMessage) {
	switch msg.Type {
	case coreagent.MsgExecStart:
		if msg.ExecStart == nil {
			return
		}
		id := msg.ID
		host := s.agent.cfg.Host
		e, err := host.Exec(msg.ExecStart.ExecID)
		if err != nil {
			s.reply(&coreagent.WSMessage{Type: coreagent.MsgExecEnd, ID: id})
			return
		}
		c, err := host.Container(e.ContainerID)
		if err != nil {
			s.reply(&coreagent.WSMessage{Type: coreagent.MsgExecEnd, ID: id})
			return
		}
		host.SetExecRunning(e.ID)
		sh := newShell(host, &c, func(b []byte) {
			s.reply(&coreagent.WSMessage{Type: coreagent.MsgExecOutput, ID: id, StreamChunk: &coreagent.WSStreamChunk{Data: b}})
		}, func() {
			host.FinishExec(e.ID, 0)
			s.dropShell(id)
			s.reply(&coreagent.WSMessage{Type: coreagent.MsgExecEnd, ID: id})
		})
		s.mu.Lock()
		s.shells[id] = sh
		s.mu.Unlock()
		sh.greet()
	case coreagent.MsgExecInput:
		if sh := s.shell(msg.ID); sh != nil && msg.StreamChunk != nil {
			sh.input(msg.StreamChunk.Data)
		}
	case coreagent.MsgExecEnd:
		if sh := s.shell(msg.ID); sh != nil {
			sh.close()
			s.dropShell(msg.ID)
		}
	}
}

func (s *session) shell(id string) *shell {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shells[id]
}

func (s *session) dropShell(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.shells, id)
}

const execIDPrefix = "MCHARBOR_EXEC_ID="

// handleHostTerminal runs the host shell. The server addresses follow-up frames
// by the exec id announced in the first output line, not by the request id.
func (s *session) handleHostTerminal(msg *coreagent.WSMessage) {
	switch msg.Type {
	case coreagent.MsgHostTerminalStart:
		reqID := msg.ID
		execID := "demo-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		host := s.agent.cfg.Host
		sh := newShell(host, nil, func(b []byte) {
			s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostTerminalOutput, ID: reqID, StreamChunk: &coreagent.WSStreamChunk{Data: b}})
		}, func() {
			s.dropShell(execID)
			s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostTerminalEnd, ID: reqID})
		})
		s.mu.Lock()
		s.shells[execID] = sh
		s.mu.Unlock()
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostTerminalOutput, ID: reqID, StreamChunk: &coreagent.WSStreamChunk{Data: []byte(execIDPrefix + execID + "\n")}})
		sh.greet()
	case coreagent.MsgHostTerminalInput:
		if msg.StreamChunk == nil {
			return
		}
		data := msg.StreamChunk.Data
		if !bytes.HasPrefix(data, []byte(execIDPrefix)) {
			return
		}
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			return
		}
		if sh := s.shell(string(data[len(execIDPrefix):nl])); sh != nil {
			sh.input(data[nl+1:])
		}
	case coreagent.MsgHostTerminalEnd:
		if msg.HostTerminalEnd == nil {
			return
		}
		if sh := s.shell(msg.HostTerminalEnd.ExecID); sh != nil {
			sh.close()
			s.dropShell(msg.HostTerminalEnd.ExecID)
		}
	}
}
