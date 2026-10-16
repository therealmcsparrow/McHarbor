// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"slices"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

func attachKindsOf(views []AttachKindView) []string {
	out := make([]string, 0, len(views))
	for _, v := range views {
		out = append(out, v.Kind)
	}
	return out
}

// TestShippedPluginsDeclareTheFormerAttachKinds: the kinds the host used to hard-code are now
// declared, so stored runbooks keep their values without a data migration.
func TestShippedPluginsDeclareTheFormerAttachKinds(t *testing.T) {
	svc := &Service{Plugins: loadedGateway(t, pluginKVM, pluginLXC, pluginSwarm, pluginKubernetes)}
	got := attachKindsOf(svc.Catalog().AttachKinds)
	want := []string{"ct", "k8s_workload", "swarm_service", "vm"}
	if !slices.Equal(got, want) {
		t.Fatalf("attach kinds = %v, want %v", got, want)
	}
	for _, v := range svc.Catalog().AttachKinds {
		if v.LabelKey == "" || v.LabelNS == "" || v.Selector == "" {
			t.Errorf("attach kind %s is missing its label or selector: %+v", v.Kind, v)
		}
	}
}

// TestDeclaredAttachKindIsAcceptedAndAnUndeclaredOneRefused covers the allow-list on save:
// a host kind always, a provider's kind while it is loaded, and nothing else.
func TestDeclaredAttachKindIsAcceptedAndAnUndeclaredOneRefused(t *testing.T) {
	loaded := &Service{Plugins: loadedGateway(t, pluginKVM)}
	bare := &Service{}
	cases := []struct {
		name string
		svc  *Service
		kind string
		want i18n.MsgCode
	}{
		{"host kind without plugins", bare, AttachContainer, ""},
		{"declared kind while loaded", loaded, "vm", ""},
		{"another plugin's kind, not loaded", loaded, "ct", i18n.ErrPluginNotInstalled},
		{"orphan of an uninstalled provider", bare, "vm", i18n.ErrPluginNotInstalled},
		{"malformed kind", loaded, "VM; drop", i18n.ErrInvalidBody},
		{"empty kind", loaded, "", i18n.ErrInvalidBody},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, bad := c.svc.validateInput(RunbookInput{Name: "x", AttachKind: c.kind})
			if bad != (c.want != "") || code != c.want {
				t.Fatalf("validateInput(%q) = %q, %v; want %q", c.kind, code, bad, c.want)
			}
		})
	}
}

// TestAnOrphanIsClassifiedNotHiddenNorInvented: a stored kind whose provider is gone is
// neither a host kind nor declared, and it reads as orphaned until the plugin returns.
func TestAnOrphanIsClassifiedNotHiddenNorInvented(t *testing.T) {
	gw := loadedGateway(t, pluginKVM)
	svc := &Service{Plugins: gw}
	if host, declared, orphan := svc.attachKindStatus("vm"); host || !declared || orphan {
		t.Fatalf("loaded: host=%v declared=%v orphan=%v", host, declared, orphan)
	}
	gw.loaded[pluginKVM] = false
	if host, declared, orphan := svc.attachKindStatus("vm"); host || declared || !orphan {
		t.Fatalf("uninstalled: host=%v declared=%v orphan=%v", host, declared, orphan)
	}
	if host, _, _ := svc.attachKindStatus(AttachStack); !host {
		t.Fatal("a host kind must stay a host kind")
	}
	if len(svc.Catalog().AttachKinds) != 0 {
		t.Fatalf("the catalog must not offer an attach kind whose plugin is gone: %+v", svc.Catalog().AttachKinds)
	}
}

// TestSubjectFitsFollowsTheDeclaredSubjectTargets: a k8s workload run may act on pods and
// deployments because the plugin said so, not because the host knows what a workload is.
func TestSubjectFitsFollowsTheDeclaredSubjectTargets(t *testing.T) {
	svc := &Service{Plugins: loadedGateway(t, pluginKubernetes, pluginKVM)}
	yes := [][2]string{{"k8s_workload", "k8s_pod"}, {"k8s_workload", "k8s_deployment"}, {"vm", "vm"}, {AttachContainer, TargetContainer}}
	no := [][2]string{{"k8s_workload", "k8s_cluster"}, {"vm", "ct"}, {AttachStack, TargetContainer}}
	for _, c := range yes {
		if !svc.subjectFits(c[0], c[1]) {
			t.Errorf("subjectFits(%s, %s) = false, want true", c[0], c[1])
		}
	}
	for _, c := range no {
		if svc.subjectFits(c[0], c[1]) {
			t.Errorf("subjectFits(%s, %s) = true, want false", c[0], c[1])
		}
	}
}

// TestStartForSubjectOfAProviderAttachedRunbook: the run's subject is the attached object's
// id for any provider kind, with no kind list in the host.
func TestStartForSubjectOfAProviderAttachedRunbook(t *testing.T) {
	in := startFor(&Runbook{ID: "r1", AttachKind: "k8s_workload", AttachSubject: "prod/web"}, nil)
	if in.SubjectKind != "k8s_workload" || in.SubjectID != "prod/web" || in.SubjectName != "prod/web" {
		t.Fatalf("subject = %+v", in)
	}
	if got := startFor(&Runbook{ID: "r2", AttachKind: AttachAlertCondition}, nil); got.SubjectKind != "" {
		t.Fatalf("an alert-attached runbook has no subject of its own: %+v", got)
	}
}
