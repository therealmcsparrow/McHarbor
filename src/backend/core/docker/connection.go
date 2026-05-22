// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/client"
)

// Connection represents a resolved Docker connection configuration.
type Connection struct {
	Type       string // socket, tcp, tls, ssh, podman
	Host       string // Docker host URL (e.g., unix:///var/run/docker.sock, tcp://host:2375)
	TLSCa     string
	TLSCert   string
	TLSKey    string
	Runtime   string // docker or podman
}

var commonDockerSockets = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/Users/.docker/run/docker.sock",
	"/home/.docker/run/docker.sock",
}

var commonPodmanSockets = []string{
	"/var/run/podman/podman.sock",
	"/run/podman/podman.sock",
}

// DetectDockerSocket tries to find a Docker socket on the system.
func DetectDockerSocket() string {
	envHost := os.Getenv("DOCKER_HOST")
	if envHost != "" && strings.HasPrefix(envHost, "unix://") {
		path := strings.TrimPrefix(envHost, "unix://")
		if fileExists(path) {
			return path
		}
	}

	for _, path := range commonDockerSockets {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// DetectPodmanSocket tries to find a Podman socket on the system.
func DetectPodmanSocket() string {
	envHost := os.Getenv("CONTAINER_HOST")
	if envHost == "" {
		envHost = os.Getenv("DOCKER_HOST")
	}
	if envHost != "" && strings.HasPrefix(envHost, "unix://") {
		path := strings.TrimPrefix(envHost, "unix://")
		if fileExists(path) && strings.Contains(path, "podman") {
			return path
		}
	}

	xdg := os.Getenv("XDG_RUNTIME_DIR")
	if xdg == "" {
		xdg = "/run/user/1000"
	}
	podmanUserSocket := xdg + "/podman/podman.sock"
	if fileExists(podmanUserSocket) {
		return podmanUserSocket
	}

	for _, path := range commonPodmanSockets {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// DetectAnySocket returns the first found Docker or Podman socket.
func DetectAnySocket() (path string, runtime string) {
	if p := DetectDockerSocket(); p != "" {
		return p, "docker"
	}
	if p := DetectPodmanSocket(); p != "" {
		return p, "podman"
	}
	return "", ""
}

// resolveConnection resolves the Docker connection for the given environment ID.
func (p *ClientPool) resolveConnection(envID string) (*Connection, error) {
	if envID == "default" || envID == "" {
		return p.resolveDefault()
	}

	// Look up environment from database
	var connType, socketPath, host, tlsCa, tlsCert, tlsKey, sshHost, sshUser, sshKey sql.NullString
	var port, sshPort sql.NullInt64

	err := p.db.QueryRow(
		`SELECT connection_type, socket_path, host, port, tls_ca, tls_cert, tls_key,
		        ssh_host, ssh_port, ssh_user, ssh_key
		 FROM environments WHERE id = ?`, envID,
	).Scan(&connType, &socketPath, &host, &port, &tlsCa, &tlsCert, &tlsKey, &sshHost, &sshPort, &sshUser, &sshKey)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment not found: %s", envID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying environment: %w", err)
	}

	ct := connType.String
	conn := &Connection{Type: ct, Runtime: "docker"}

	switch ct {
	case "socket", "podman":
		sock := socketPath.String
		if sock == "" {
			if ct == "podman" {
				sock = DetectPodmanSocket()
				if sock == "" {
					sock = "/var/run/podman/podman.sock"
				}
			} else {
				sock = DetectDockerSocket()
				if sock == "" {
					sock = "/var/run/docker.sock"
				}
			}
		}
		conn.Host = "unix://" + sock
		if ct == "podman" {
			conn.Runtime = "podman"
		}

	case "tcp":
		h := host.String
		p := int(port.Int64)
		if p == 0 {
			p = 2375
		}
		conn.Host = fmt.Sprintf("tcp://%s:%d", h, p)

	case "tls":
		h := host.String
		p := int(port.Int64)
		if p == 0 {
			p = 2376
		}
		conn.Host = fmt.Sprintf("tcp://%s:%d", h, p)
		conn.TLSCa = tlsCa.String
		conn.TLSCert = tlsCert.String
		conn.TLSKey = tlsKey.String

	case "ssh":
		h := sshHost.String
		u := sshUser.String
		sp := int(sshPort.Int64)
		if sp == 0 {
			sp = 22
		}
		conn.Host = fmt.Sprintf("ssh://%s@%s:%d", u, h, sp)

	case "agent":
		return nil, fmt.Errorf("agent not connected for environment %s; ensure the agent is running on the remote host", envID)

	default:
		return nil, fmt.Errorf("unsupported connection type: %s", ct)
	}

	return conn, nil
}

// resolveDefault falls back to local socket auto-detection.
// DB-based default resolution is handled by resolveDefaultEnvID in client.go.
func (p *ClientPool) resolveDefault() (*Connection, error) {
	path, runtime := DetectAnySocket()
	if path != "" {
		return &Connection{
			Type:    "socket",
			Host:    "unix://" + path,
			Runtime: runtime,
		}, nil
	}

	return nil, fmt.Errorf("no Docker or Podman connection available; configure an environment or ensure a container socket is accessible")
}

// createClient creates a Docker SDK client from the connection config.
func (c *Connection) createClient() (*client.Client, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	if c.Host != "" {
		opts = append(opts, client.WithHost(c.Host))
	}

	if c.Type == "tls" && c.TLSCa != "" && c.TLSCert != "" && c.TLSKey != "" {
		// For TLS, we'd write temp cert files and use client.WithTLSClientConfig
		// For now, just set the host (full TLS cert handling can be added later)
		opts = append(opts, client.WithHost(c.Host))
	}

	return client.NewClientWithOpts(opts...)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
