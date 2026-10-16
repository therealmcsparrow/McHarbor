// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// guardSecrets are the values the fake cluster below holds in env literals, a Secret and a
// ConfigMap. None of the plugin's read surfaces may return one.
var guardSecrets = []string{"env-literal-db-password", "secret-data-api-token", "secret-string-data-value", "configmap-db-password"}

func guardContainer() corev1.Container {
	return corev1.Container{
		Name: "app", Image: "app:1",
		Env: []corev1.EnvVar{
			{Name: "DB_PASSWORD", Value: guardSecrets[0]},
			{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "creds"}, Key: "token",
			}}},
		},
	}
}

// guardCluster serves the Kubernetes API calls Summary, Pod and Deployment make, over objects
// that carry credentials; anything else is an empty 404 the list helpers already tolerate.
func guardCluster(t *testing.T) *httptest.Server {
	t.Helper()
	meta := metav1.ObjectMeta{Name: "web", Namespace: "default"}
	pod := corev1.Pod{ObjectMeta: meta, Spec: corev1.PodSpec{Containers: []corev1.Container{guardContainer()}}}
	replicas := int32(1)
	deployment := appsv1.Deployment{ObjectMeta: meta, Spec: appsv1.DeploymentSpec{
		Replicas: &replicas,
		Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{guardContainer()}}},
	}}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "default"}, Type: corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"token": []byte(guardSecrets[1])},
		StringData: map[string]string{"extra": guardSecrets[2]},
	}
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "settings", Namespace: "default"},
		Data:       map[string]string{"DB_PASSWORD": guardSecrets[3], "LOG_LEVEL": "info"},
	}
	routes := map[string]any{
		"/version":                            map[string]string{"gitVersion": "v1.37.0"},
		"/api/v1/pods":                        corev1.PodList{Items: []corev1.Pod{pod}},
		"/api/v1/namespaces/default/pods/web": pod,
		"/apis/apps/v1/deployments":           appsv1.DeploymentList{Items: []appsv1.Deployment{deployment}},
		"/apis/apps/v1/namespaces/default/deployments/web": deployment,
		"/api/v1/secrets":    corev1.SecretList{Items: []corev1.Secret{secret}},
		"/api/v1/configmaps": corev1.ConfigMapList{Items: []corev1.ConfigMap{configMap}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, found := routes[r.URL.Path]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			body = metav1.Status{Status: metav1.StatusFailure, Code: http.StatusNotFound, Reason: metav1.StatusReasonNotFound}
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encoding %s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestNoReadSurfaceReturnsEnvOrSecretValues guards the plugin's redaction posture: it has no
// reveal route because no view carries container env, Secret data or ConfigMap data. A field
// added to Summary, PodDetail or DeploymentDetail that copies one of them fails here.
func TestNoReadSurfaceReturnsEnvOrSecretValues(t *testing.T) {
	srv := guardCluster(t)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(&ClientPool{clients: map[string]*kubernetes.Clientset{"c1": cs}}, nil)
	ctx := context.Background()

	summary, err := svc.Summary(ctx, "c1", "")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Pods) != 1 || len(summary.Secrets) != 1 || len(summary.ConfigMaps) != 1 {
		t.Fatalf("the fake cluster was not read: %d pods, %d secrets, %d config maps",
			len(summary.Pods), len(summary.Secrets), len(summary.ConfigMaps))
	}
	pod, err := svc.Pod(ctx, "c1", "default", "web")
	if err != nil {
		t.Fatalf("pod: %v", err)
	}
	deployment, err := svc.Deployment(ctx, "c1", "default", "web")
	if err != nil {
		t.Fatalf("deployment: %v", err)
	}

	for name, view := range map[string]any{"summary": summary, "pod detail": pod, "deployment detail": deployment} {
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range guardSecrets {
			if strings.Contains(string(raw), secret) {
				t.Errorf("the %s returned %q; mask it and add a manage-gated reveal before exposing it", name, secret)
			}
		}
	}
}
