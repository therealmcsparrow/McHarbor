// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	zeroTime       = "0001-01-01T00:00:00Z"
	statusFiring   = "firing"
	statusResolved = "resolved"
)

var labelNameRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

type amAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"`
	EndsAt       string            `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

type amPayload struct {
	Version            string            `json:"version"`
	GroupKey           string            `json:"groupKey"`
	TruncatedAlerts    int               `json:"truncatedAlerts"`
	Status             string            `json:"status"`
	Receiver           string            `json:"receiver"`
	GroupLabels        map[string]string `json:"groupLabels"`
	CommonLabels       map[string]string `json:"commonLabels"`
	CommonAnnotations  map[string]string `json:"commonAnnotations"`
	ExternalURL        string            `json:"externalURL"`
	NotificationReason string            `json:"notification_reason"`
	Alerts             []amAlert         `json:"alerts"`
}

// fingerprint is 16 hex characters, the length and alphabet of a real Alertmanager fingerprint,
// so receivers that validate or index it accept ours unchanged.
func fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}

func labelName(k string) string {
	k = labelNameRE.ReplaceAllString(k, "_")
	if k == "" || (k[0] >= '0' && k[0] <= '9') {
		k = "_" + k
	}
	return k
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// buildPayload derives groupKey, groupLabels and fingerprint from fp alone, so one McHarbor
// condition is always exactly one Alertmanager group at every receiver.
func buildPayload(cfg amConfig, ev incident.Event, fp, startsAt, status string, endsAt time.Time) amPayload {
	labels := map[string]string{}
	keys := make([]string, 0, len(ev.Labels))
	for k := range ev.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n := labelName(k)
		if _, taken := labels[n]; !taken && ev.Labels[k] != "" {
			labels[n] = ev.Labels[k]
		}
	}
	alertname := firstNonEmpty(ev.AlertName, "McHarborAlert")
	if cfg.Profile == "zenduty" {
		alertname += " [" + fp[:8] + "]"
	}
	labels["alertname"] = alertname
	labels["severity"] = firstNonEmpty(cfg.SeverityMap[ev.Severity], string(ev.Severity), "warning")
	labels["mcharbor_dedup"] = fp
	if ev.EnvironmentName != "" {
		labels["environment"] = ev.EnvironmentName
	}
	labels["instance"] = firstNonEmpty(ev.SubjectName, ev.EnvironmentName, "mcharbor")
	labels["job"] = "mcharbor"
	ann := map[string]string{"summary": ev.Title, "description": firstNonEmpty(ev.Message, ev.Title)}
	end := zeroTime
	if status == statusResolved {
		end = resolvedEnd(startsAt, endsAt)
	}
	return amPayload{
		Version:            "4",
		GroupKey:           `{}:{mcharbor_dedup="` + fp + `"}`,
		Status:             status,
		Receiver:           cfg.Receiver,
		GroupLabels:        map[string]string{"mcharbor_dedup": fp},
		CommonLabels:       labels,
		CommonAnnotations:  ann,
		ExternalURL:        originOf(ev.Link),
		NotificationReason: "mcharbor",
		Alerts: []amAlert{{Status: status, Labels: labels, Annotations: ann, StartsAt: startsAt, EndsAt: end,
			GeneratorURL: ev.Link, Fingerprint: fp}},
	}
}

// resolvedEnd never reports an endsAt before startsAt, which a skewed clock or a future
// OccurredAt would otherwise produce and some receivers reject as an invalid interval.
func resolvedEnd(startsAt string, endsAt time.Time) string {
	end := endsAt.UTC().Truncate(time.Second)
	if st, err := time.Parse(time.RFC3339, startsAt); err == nil && end.Before(st) {
		end = st
	}
	return end.Format(time.RFC3339)
}

func originOf(link string) string {
	if i := strings.Index(link, "://"); i > 0 {
		if j := strings.Index(link[i+3:], "/"); j >= 0 {
			return link[:i+3+j]
		}
		return link
	}
	return ""
}
