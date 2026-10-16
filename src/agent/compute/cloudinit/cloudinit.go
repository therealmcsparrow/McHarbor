// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package cloudinit builds the NoCloud seed image a cloud guest reads on first
// boot.
//
// The seed is a small ISO9660 volume labelled CIDATA holding three files —
// `meta-data`, `user-data` and `network-config`. cloud-init inside the guest
// finds it by that label and applies the hostname, SSH keys, user accounts and
// network settings it describes. That is the whole of the fast path: no
// installer, no console interaction, boot to a working login in under two
// minutes.
//
// Two deliberate choices about how it is built:
//
//   - It is written in process with go-diskfs rather than by shelling out to
//     `cloud-localds` or `genisoimage`. That removes two packages from the host
//     dependency floor and, more importantly, means the seed never exists as a
//     temporary file some other tooling on the host could read.
//   - The documents are emitted with yaml.v3, never assembled by string
//     concatenation. A hostname containing a colon, a newline or a leading dash
//     must not be able to reshape the document — and cloud-init acts on that
//     document as root inside the guest.
//
// # What must never be in a seed
//
// The seed image is readable by anything that can read the pool. It carries no
// plaintext password, no API token and no private key. The password field is a
// SHA-512 crypt(3) hash computed on the backend; the plaintext exists only for
// the duration of the create request and is never persisted. This is asserted,
// not merely intended — see the guard in Build.
package cloudinit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// VolumeLabel is what cloud-init's NoCloud datasource looks for. It is not
// configurable: a seed with any other label is simply not found, and the guest
// boots unconfigured with no indication why.
const VolumeLabel = "CIDATA"

// Config is everything the create dialog collected.
type Config struct {
	// InstanceID identifies this boot to cloud-init. It is regenerated on
	// clone, which is what makes a cloned guest re-run first-boot setup instead
	// of inheriting the template's hostname and SSH host keys — the classic
	// clone footgun.
	InstanceID string

	Hostname string
	FQDN     string
	Timezone string

	DefaultUser string
	SSHKeys     []string
	// PasswordHash is a crypt(3) string such as $6$... . A value that does not
	// look like one is refused rather than written.
	PasswordHash string

	NetworkMode   string // dhcp | static
	InterfaceName string
	StaticIP      string // CIDR form, e.g. 192.168.1.50/24
	StaticGateway string
	StaticDNS     []string

	RunCmd []string
	// UserDataExtra is a raw cloud-config fragment for operators who know what
	// they want. It is merged last and parsed first, so a malformed override
	// fails here rather than silently producing a guest that never configures.
	UserDataExtra string

	// InstallQEMUGuestAgent adds the guest agent package, which is what makes
	// filesystem-consistent snapshots and real guest IP reporting possible.
	InstallQEMUGuestAgent bool
	Packages              []string
}

// Seed is the rendered document set, before it becomes an image.
type Seed struct {
	MetaData      []byte
	UserData      []byte
	NetworkConfig []byte
}

// cryptHashRE matches the crypt(3) forms cloud-init's chpasswd accepts.
//
// Checking the shape is what turns "never put a plaintext password in the seed"
// from a comment into something enforced. A caller that passed the plaintext by
// mistake gets an error instead of a world-readable password on every pool.
var cryptHashRE = regexp.MustCompile(`^\$(1|2[aby]?|5|6|y|7|argon2(id|i|d))\$`)

// hostnameRE is the RFC 1123 label form. Anything else is refused rather than
// sanitized: quietly rewriting a hostname produces a guest the operator cannot
// find by the name they chose.
var hostnameRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// Build renders the three documents.
func Build(cfg Config) (Seed, error) {
	if cfg.InstanceID == "" {
		return Seed{}, errors.New("cloudinit: instance id is required")
	}
	if err := validate(cfg); err != nil {
		return Seed{}, err
	}

	metaData, err := buildMetaData(cfg)
	if err != nil {
		return Seed{}, err
	}
	userData, err := buildUserData(cfg)
	if err != nil {
		return Seed{}, err
	}
	networkConfig, err := buildNetworkConfig(cfg)
	if err != nil {
		return Seed{}, err
	}

	return Seed{MetaData: metaData, UserData: userData, NetworkConfig: networkConfig}, nil
}

func validate(cfg Config) error {
	if cfg.Hostname != "" && !hostnameRE.MatchString(cfg.Hostname) {
		return fmt.Errorf("cloudinit: %q is not a valid hostname", cfg.Hostname)
	}
	if cfg.PasswordHash != "" && !cryptHashRE.MatchString(cfg.PasswordHash) {
		// The message deliberately does not echo the value: if this fired
		// because a plaintext password was passed, repeating it would put the
		// password into the agent's logs, which is the failure this check
		// exists to prevent.
		return errors.New("cloudinit: the password field must be a crypt(3) hash " +
			"(for example $6$...), never a plaintext password")
	}
	for _, key := range cfg.SSHKeys {
		if err := validateSSHKey(key); err != nil {
			return err
		}
	}
	if cfg.NetworkMode == "static" && cfg.StaticIP == "" {
		return errors.New("cloudinit: static networking needs an address")
	}
	return nil
}

// sshKeyRE matches the public key types cloud-init accepts. Private keys and
// arbitrary text are refused — a private key in a seed would be readable by
// anything that can read the pool.
var sshKeyRE = regexp.MustCompile(`^(ssh-rsa|ssh-ed25519|ssh-dss|ecdsa-sha2-nistp(256|384|521)|sk-ssh-ed25519@openssh\.com|sk-ecdsa-sha2-nistp256@openssh\.com) [A-Za-z0-9+/=]+( .*)?$`)

func validateSSHKey(key string) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return errors.New("cloudinit: empty SSH key")
	}
	if strings.Contains(trimmed, "PRIVATE KEY") {
		return errors.New("cloudinit: refusing to write a private key into the seed image; " +
			"the seed is readable by anything that can read the pool")
	}
	if !sshKeyRE.MatchString(trimmed) {
		return fmt.Errorf("cloudinit: %.32q... is not a valid SSH public key", trimmed)
	}
	return nil
}

// buildMetaData renders the NoCloud identity document.
func buildMetaData(cfg Config) ([]byte, error) {
	doc := map[string]any{
		"instance-id": cfg.InstanceID,
	}
	if cfg.Hostname != "" {
		doc["local-hostname"] = cfg.Hostname
	}
	return marshal(doc, false)
}

// buildUserData renders the cloud-config document.
func buildUserData(cfg Config) ([]byte, error) {
	doc := map[string]any{}

	if cfg.Hostname != "" {
		doc["hostname"] = cfg.Hostname
		// preserve_hostname false lets cloud-init set it on every boot, which
		// is what a clone needs in order to stop answering to the template's
		// name.
		doc["preserve_hostname"] = false
	}
	if cfg.FQDN != "" {
		doc["fqdn"] = cfg.FQDN
	}
	if cfg.Timezone != "" {
		doc["timezone"] = cfg.Timezone
	}

	user := cfg.DefaultUser
	if user == "" {
		user = "mcharbor"
	}
	account := map[string]any{
		"name": user,
		// The default user gets passwordless sudo, which is the convention
		// every distribution's own cloud image already follows.
		"sudo":  "ALL=(ALL) NOPASSWD:ALL",
		"shell": "/bin/bash",
		// Never a distro-specific group such as sudo or wheel: useradd refuses a
		// group that does not exist, and cloud-init then skips the whole account,
		// so the wizard's user and password silently never work. Sudo rights come
		// from the rule above, not from a group.
		"groups": []string{"adm"},
		// lock_passwd true means no password login at all unless one was set
		// below. A cloud guest reachable by SSH key only is the safer default.
		"lock_passwd": cfg.PasswordHash == "",
	}
	if len(cfg.SSHKeys) > 0 {
		account["ssh_authorized_keys"] = cfg.SSHKeys
	}
	doc["users"] = []any{"default", account}
	// cloud-init creates these before it creates users, so an image without an
	// adm group gets one instead of failing the account the same way.
	doc["groups"] = []any{"adm"}

	if cfg.PasswordHash != "" {
		// The hashed form. chpasswd accepts it on every supported distro, and
		// it is what keeps the plaintext out of a world-readable seed.
		doc["chpasswd"] = map[string]any{
			"expire": false,
			"users": []any{
				map[string]any{
					"name":     user,
					"password": cfg.PasswordHash,
					"type":     "hash",
				},
			},
		}
		// Password login is enabled only when a password was actually set.
		doc["ssh_pwauth"] = true
	}

	packages := append([]string{}, cfg.Packages...)
	if cfg.InstallQEMUGuestAgent {
		packages = append(packages, "qemu-guest-agent")
	}
	if len(packages) > 0 {
		doc["packages"] = packages
		doc["package_update"] = true
	}
	if cfg.InstallQEMUGuestAgent {
		// Installing the package is not enough on every distribution; the
		// service has to be enabled for the host to see the guest at all.
		doc["runcmd"] = append([]any{
			[]string{"systemctl", "enable", "--now", "qemu-guest-agent"},
		}, toAnySlice(cfg.RunCmd)...)
	} else if len(cfg.RunCmd) > 0 {
		doc["runcmd"] = toAnySlice(cfg.RunCmd)
	}

	if cfg.UserDataExtra != "" {
		// Parsed before merging so a malformed override is an error the
		// operator sees at create time, not a guest that boots unconfigured.
		var extra map[string]any
		if err := yaml.Unmarshal([]byte(cfg.UserDataExtra), &extra); err != nil {
			return nil, fmt.Errorf("cloudinit: the extra user-data is not valid YAML: %w", err)
		}
		for key, value := range extra {
			doc[key] = value
		}
	}

	// The #cloud-config header is mandatory: cloud-init identifies the
	// document by it and silently ignores a file without it.
	return marshal(doc, true)
}

// buildNetworkConfig renders a version 2 netplan-style network document, which
// is the form current cloud-init versions expect.
func buildNetworkConfig(cfg Config) ([]byte, error) {
	iface := cfg.InterfaceName
	if iface == "" {
		// Matching by driver rather than by name: predictable interface names
		// differ per machine type and per guest OS, so naming one here would
		// produce a guest whose NIC is never configured.
		iface = "id0"
	}

	entry := map[string]any{
		"match": map[string]any{"name": "en*"},
		// set-name is deliberately omitted: renaming the guest's interface
		// serves no purpose here and breaks any in-guest config that referred
		// to the original name.
	}

	if cfg.NetworkMode == "static" {
		entry["dhcp4"] = false
		entry["addresses"] = []string{cfg.StaticIP}
		if cfg.StaticGateway != "" {
			// routes rather than the deprecated gateway4, which current
			// netplan warns about and will eventually reject.
			entry["routes"] = []any{
				map[string]any{"to": "default", "via": cfg.StaticGateway},
			}
		}
		if len(cfg.StaticDNS) > 0 {
			entry["nameservers"] = map[string]any{"addresses": cfg.StaticDNS}
		}
	} else {
		entry["dhcp4"] = true
		entry["dhcp6"] = false
	}

	doc := map[string]any{
		"version":   2,
		"ethernets": map[string]any{iface: entry},
	}
	return marshal(doc, false)
}

// marshal renders a document with a two-space indent, optionally prefixed with
// the cloud-config header.
func marshal(doc map[string]any, cloudConfigHeader bool) ([]byte, error) {
	var sb strings.Builder
	if cloudConfigHeader {
		sb.WriteString("#cloud-config\n")
	}

	encoder := yaml.NewEncoder(&sb)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return nil, fmt.Errorf("cloudinit: render document: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("cloudinit: close encoder: %w", err)
	}
	return []byte(sb.String()), nil
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}

// ContainsPlaintextSecret is a last-line check the caller runs over the
// rendered seed before it is written.
//
// It cannot catch everything — a secret is only recognisable by context — but
// it catches the shapes that actually happen: a PEM block pasted into the extra
// user-data, or a password field that is not a hash. Cheap, and the failure it
// prevents is a credential readable by every local user on the host.
func ContainsPlaintextSecret(seed Seed) error {
	for name, doc := range map[string][]byte{
		"user-data":      seed.UserData,
		"meta-data":      seed.MetaData,
		"network-config": seed.NetworkConfig,
	} {
		text := string(doc)
		if strings.Contains(text, "PRIVATE KEY") {
			return fmt.Errorf("cloudinit: %s contains a private key; the seed image is "+
				"readable by anything that can read the pool", name)
		}
		if idx := strings.Index(text, "password:"); idx >= 0 {
			rest := strings.TrimSpace(text[idx+len("password:"):])
			if rest != "" && !strings.HasPrefix(rest, "$") && !strings.HasPrefix(rest, "\n") {
				return fmt.Errorf("cloudinit: %s carries a password that is not a crypt(3) hash", name)
			}
		}
	}
	return nil
}
