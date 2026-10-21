// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// guardFixtures holds the host-side files a hostile kubeconfig points at. Nothing here
// is executable: the exec command is a path that must not exist, so an attempt to run it
// leaves its name in the error instead of starting a process.
type guardFixtures struct {
	execCmd     string
	tokenFile   string
	tokenSecret string
	certFile    string
	keyFile     string
	caFile      string
	certPEM     []byte
	keyPEM      []byte
}

func newGuardFixtures(t *testing.T) guardFixtures {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "guard-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	fx := guardFixtures{
		execCmd:     filepath.Join(dir, "must-not-exist-exec-marker"),
		tokenFile:   filepath.Join(dir, "token"),
		tokenSecret: "host-file-secret-must-not-be-read",
		certFile:    filepath.Join(dir, "client.crt"),
		keyFile:     filepath.Join(dir, "client.key"),
		caFile:      filepath.Join(dir, "ca.crt"),
		certPEM:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:      pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	}
	for path, body := range map[string][]byte{
		fx.tokenFile: []byte(fx.tokenSecret),
		fx.certFile:  fx.certPEM,
		fx.keyFile:   fx.keyPEM,
		fx.caFile:    fx.certPEM,
	} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(fx.execCmd); err == nil {
		t.Fatal("the exec marker path must not exist")
	}
	return fx
}

// hostileField is one forbidden kubeconfig field, applied to either a user or a cluster.
type hostileField struct {
	name    string
	onUser  func(fx guardFixtures, u *api.AuthInfo)
	onClust func(fx guardFixtures, c *api.Cluster)
}

func hostileFields() []hostileField {
	return []hostileField{
		{name: "exec", onUser: func(fx guardFixtures, u *api.AuthInfo) {
			u.Exec = &api.ExecConfig{
				APIVersion:      "client.authentication.k8s.io/v1",
				Command:         fx.execCmd,
				InteractiveMode: api.NeverExecInteractiveMode,
			}
		}},
		{name: "auth-provider", onUser: func(_ guardFixtures, u *api.AuthInfo) {
			u.AuthProvider = &api.AuthProviderConfig{Name: "oidc", Config: map[string]string{"idp-issuer-url": "https://127.0.0.1:1"}}
		}},
		{name: "tokenFile", onUser: func(fx guardFixtures, u *api.AuthInfo) {
			u.Token = ""
			u.TokenFile = fx.tokenFile
		}},
		{name: "client-certificate", onUser: func(fx guardFixtures, u *api.AuthInfo) {
			u.ClientCertificate = fx.certFile
			u.ClientKeyData = fx.keyPEM
		}},
		{name: "client-key", onUser: func(fx guardFixtures, u *api.AuthInfo) {
			u.ClientCertificateData = fx.certPEM
			u.ClientKey = fx.keyFile
		}},
		{name: "certificate-authority", onClust: func(fx guardFixtures, c *api.Cluster) {
			c.CertificateAuthority = fx.caFile
			c.CertificateAuthorityData = nil
		}},
		{name: "proxy-url", onClust: func(_ guardFixtures, c *api.Cluster) { c.ProxyURL = "http://127.0.0.1:1" }},
	}
}

// guardPlacements name the entry that carries the hostile field. Every one except the
// first is an entry the current context never reads, which a current-context-only check
// would let through.
var guardPlacements = []string{"current", "other-context", "orphan"}

// guardKubeconfig renders a two-context kubeconfig with inline credentials only, then
// applies f to the placement's user or cluster. A nil f yields the clean kubeconfig.
func guardKubeconfig(t *testing.T, fx guardFixtures, placement string, f *hostileField) string {
	t.Helper()
	cfg := api.NewConfig()
	for _, n := range []string{"1", "2", "3"} {
		cluster := api.NewCluster()
		cluster.Server = "https://127.0.0.1:6443"
		cluster.CertificateAuthorityData = fx.certPEM
		cfg.Clusters["c"+n] = cluster
		user := api.NewAuthInfo()
		user.Token = "inline-token-" + n
		cfg.AuthInfos["u"+n] = user
	}
	for _, n := range []string{"1", "2"} {
		kctx := api.NewContext()
		kctx.Cluster, kctx.AuthInfo = "c"+n, "u"+n
		cfg.Contexts["ctx"+n] = kctx
	}
	cfg.CurrentContext = "ctx1"
	if f != nil {
		target := map[string]string{"current": "1", "other-context": "2", "orphan": "3"}[placement]
		if f.onUser != nil {
			f.onUser(fx, cfg.AuthInfos["u"+target])
		}
		if f.onClust != nil {
			f.onClust(fx, cfg.Clusters["c"+target])
		}
	}
	out, err := clientcmd.Write(*cfg)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func forEachHostile(t *testing.T, body func(t *testing.T, fx guardFixtures, kubeconfig string)) {
	t.Helper()
	fx := newGuardFixtures(t)
	for _, f := range hostileFields() {
		for _, placement := range guardPlacements {
			t.Run(f.name+"/"+placement, func(t *testing.T) {
				body(t, fx, guardKubeconfig(t, fx, placement, &f))
			})
		}
	}
}

func guardStore(t *testing.T) *clusterStore {
	t.Helper()
	store := legacyStore(t)
	if err := store.ensureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

// TestKubeconfigGuard_CreateRefusesUnsafeKubeconfig is acceptance (a) for POST: an
// exec, auth-provider or host-file reference is refused in any user or cluster entry,
// and nothing is stored.
func TestKubeconfigGuard_CreateRefusesUnsafeKubeconfig(t *testing.T) {
	forEachHostile(t, func(t *testing.T, _ guardFixtures, kubeconfig string) {
		store := guardStore(t)
		ctx := context.Background()
		if _, err := store.create(ctx, ClusterInput{Name: strp("hostile"), Kubeconfig: &kubeconfig}); err == nil {
			t.Error("create accepted a kubeconfig carrying a forbidden field")
		}
		if rows, err := store.list(ctx); err != nil || len(rows) != 0 {
			t.Errorf("a refused create left rows behind: %d (%v)", len(rows), err)
		}
	})
}

// TestKubeconfigGuard_UpdateRefusesUnsafeKubeconfig is acceptance (a) for PUT: a request
// that changes only the kubeconfig is refused and the stored kubeconfig is untouched.
func TestKubeconfigGuard_UpdateRefusesUnsafeKubeconfig(t *testing.T) {
	forEachHostile(t, func(t *testing.T, fx guardFixtures, kubeconfig string) {
		store := guardStore(t)
		ctx := context.Background()
		clean := guardKubeconfig(t, fx, "current", nil)
		created, err := store.create(ctx, ClusterInput{Name: strp("clean"), Kubeconfig: &clean})
		if err != nil {
			t.Fatalf("a clean inline kubeconfig must be accepted on create: %v", err)
		}
		if _, err := store.update(ctx, created.ID, ClusterInput{Kubeconfig: &kubeconfig}); err == nil {
			t.Error("update accepted a kubeconfig carrying a forbidden field")
		}
		conn, err := store.connection(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if conn.Kubeconfig != clean {
			t.Error("a refused update changed the stored kubeconfig")
		}
	})
}

// TestKubeconfigGuard_LoadRefusesStoredUnsafeRows is acceptance (b): a row already in
// the table (migration 071, a restored backup) is refused when the connection is built,
// without passing through save.
func TestKubeconfigGuard_LoadRefusesStoredUnsafeRows(t *testing.T) {
	forEachHostile(t, func(t *testing.T, _ guardFixtures, kubeconfig string) {
		store := guardStore(t)
		ctx := context.Background()
		cipher, err := store.encrypt(kubeconfig)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := store.db.Exec(ctx, `INSERT INTO plugin_kubernetes_clusters
			(id, name, namespace, server_url, kubeconfig, bearer_token, ca_cert, is_default, is_active, created_at, updated_at, environment_id)
			VALUES ('legacy', 'legacy', '', '', ?, '', '', 1, 1, ?, ?, '')`, cipher, now, now); err != nil {
			t.Fatal(err)
		}
		conn, err := store.connection(ctx, "legacy")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.restConfig(); err == nil {
			t.Error("restConfig built a config from a stored kubeconfig carrying a forbidden field")
		}
		if _, err := conn.createClient(); err == nil {
			t.Error("createClient built a client from a stored kubeconfig carrying a forbidden field")
		}
	})
}

// TestKubeconfigGuard_NoProcessAndNoFileRead proves the side effects, not just the error:
// the exec command is never attempted and the tokenFile contents never become a credential.
func TestKubeconfigGuard_NoProcessAndNoFileRead(t *testing.T) {
	fx := newGuardFixtures(t)
	fields := hostileFields()
	byName := map[string]*hostileField{}
	for i := range fields {
		byName[fields[i].name] = &fields[i]
	}

	t.Run("exec is never attempted", func(t *testing.T) {
		for _, placement := range guardPlacements {
			conn := &Connection{Kubeconfig: guardKubeconfig(t, fx, placement, byName["exec"])}
			cfg, err := conn.restConfig()
			if err != nil || cfg.ExecProvider == nil {
				continue
			}
			// The guard let it through. Trigger the first request, which is what runs the
			// command in production. The path does not exist, so no process starts; the
			// attempt shows up as the path inside the error.
			client, cErr := conn.createClient()
			if cErr != nil {
				t.Errorf("%s: exec stanza reached the rest config, then client build failed: %v", placement, cErr)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, listErr := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			cancel()
			if listErr != nil && strings.Contains(listErr.Error(), "must-not-exist-exec-marker") {
				t.Errorf("%s: the exec plugin was attempted for command %s", placement, fx.execCmd)
			} else {
				t.Errorf("%s: the exec stanza reached the rest config (ExecProvider set)", placement)
			}
		}
	})

	t.Run("tokenFile is never read", func(t *testing.T) {
		for _, placement := range guardPlacements {
			conn := &Connection{Kubeconfig: guardKubeconfig(t, fx, placement, byName["tokenFile"])}
			cfg, err := conn.restConfig()
			if err != nil {
				continue
			}
			if strings.Contains(cfg.BearerToken, fx.tokenSecret) {
				t.Errorf("%s: the host file %s was read into the credential", placement, fx.tokenFile)
			}
		}
	})
}

// TestKubeconfigGuard_CleanInlineKubeconfigStillWorks is acceptance (c): inline-data
// credentials are saved, stored and loaded as before.
func TestKubeconfigGuard_CleanInlineKubeconfigStillWorks(t *testing.T) {
	fx := newGuardFixtures(t)

	certData := api.NewConfig()
	cluster := api.NewCluster()
	cluster.Server = "https://127.0.0.1:6443"
	cluster.CertificateAuthorityData = fx.certPEM
	user := api.NewAuthInfo()
	user.ClientCertificateData = fx.certPEM
	user.ClientKeyData = fx.keyPEM
	certData.Clusters["c"] = cluster
	certData.AuthInfos["u"] = user
	kctx := api.NewContext()
	kctx.Cluster, kctx.AuthInfo = "c", "u"
	certData.Contexts["x"] = kctx
	certData.CurrentContext = "x"
	certDataKubeconfig, err := clientcmd.Write(*certData)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"inline token":     guardKubeconfig(t, fx, "current", nil),
		"inline cert data": string(certDataKubeconfig),
	}
	for name, kubeconfig := range cases {
		t.Run(name, func(t *testing.T) {
			store := guardStore(t)
			ctx := context.Background()
			created, err := store.create(ctx, ClusterInput{Name: strp("clean"), Kubeconfig: &kubeconfig})
			if err != nil {
				t.Fatalf("create refused a clean kubeconfig: %v", err)
			}
			if _, err := store.update(ctx, created.ID, ClusterInput{Name: strp("renamed")}); err != nil {
				t.Fatalf("an update that leaves the kubeconfig alone must still work: %v", err)
			}
			if _, err := store.update(ctx, created.ID, ClusterInput{Kubeconfig: &kubeconfig}); err != nil {
				t.Fatalf("update refused a clean kubeconfig: %v", err)
			}
			conn, err := store.connection(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := conn.restConfig()
			if err != nil {
				t.Fatalf("restConfig refused a clean kubeconfig: %v", err)
			}
			if cfg.Host != "https://127.0.0.1:6443" {
				t.Errorf("host = %q", cfg.Host)
			}
			if name == "inline token" && cfg.BearerToken != "inline-token-1" {
				t.Errorf("the current context's inline token was not used: %q", cfg.BearerToken)
			}
			if _, err := conn.createClient(); err != nil {
				t.Errorf("createClient refused a clean kubeconfig: %v", err)
			}
		})
	}
}
