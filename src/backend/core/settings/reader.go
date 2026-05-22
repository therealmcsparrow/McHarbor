// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"database/sql"
	"strconv"
)

// AgentSettings holds configurable agent behavior settings.
type AgentSettings struct {
	EventMode         string `json:"eventMode"`         // "poll" or "stream"
	EventPollInterval int    `json:"eventPollInterval"` // seconds (default 30)
	PingInterval      int    `json:"pingInterval"`      // seconds (default 30)
	MetricsEnabled    bool   `json:"metricsEnabled"`    // default false
	RequestTimeout    int    `json:"requestTimeout"`    // seconds (default 30)
}

// DefaultAgentSettings returns the default agent settings.
func DefaultAgentSettings() AgentSettings {
	return AgentSettings{
		EventMode:         "poll",
		EventPollInterval: 30,
		PingInterval:      30,
		MetricsEnabled:    false,
		RequestTimeout:    30,
	}
}

// ReadAgentSettings reads agent settings from the settings table, falling back to defaults.
func ReadAgentSettings(db *sql.DB) AgentSettings {
	s := DefaultAgentSettings()

	rows, err := db.Query("SELECT key, value FROM settings WHERE category = 'agent'")
	if err != nil {
		return s
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "agent_event_mode":
			if value == "poll" || value == "stream" {
				s.EventMode = value
			}
		case "agent_event_poll_interval":
			if v, err := strconv.Atoi(value); err == nil && v >= 10 && v <= 300 {
				s.EventPollInterval = v
			}
		case "agent_ping_interval":
			if v, err := strconv.Atoi(value); err == nil && v >= 10 && v <= 120 {
				s.PingInterval = v
			}
		case "agent_metrics_enabled":
			s.MetricsEnabled = value == "true"
		case "agent_request_timeout":
			if v, err := strconv.Atoi(value); err == nil && v >= 5 && v <= 120 {
				s.RequestTimeout = v
			}
		}
	}

	return s
}

// ScannerSettings holds configurable vulnerability scanner settings.
type ScannerSettings struct {
	TrivyEnabled   bool   `json:"trivyEnabled"`
	GrypeEnabled   bool   `json:"grypeEnabled"`
	ClairEnabled   bool   `json:"clairEnabled"`
	ClairURL       string `json:"clairUrl"`
	DefaultScanner string `json:"defaultScanner"`
	ScanTimeout    int    `json:"scanTimeout"` // seconds
	ScanOnInstall  bool   `json:"scanOnInstall"`
	ScanOnUpdate   bool   `json:"scanOnUpdate"`
}

// DefaultScannerSettings returns the default scanner settings.
func DefaultScannerSettings() ScannerSettings {
	return ScannerSettings{
		TrivyEnabled:   true,
		GrypeEnabled:   false,
		ClairEnabled:   false,
		ClairURL:       "",
		DefaultScanner: "trivy",
		ScanTimeout:    300,
		ScanOnInstall:  false,
		ScanOnUpdate:   false,
	}
}

// RetentionSettings holds configurable data retention settings.
type RetentionSettings struct {
	AuditRetentionDays    int `json:"auditRetentionDays"`    // 0 = keep forever
	ActivityRetentionDays int `json:"activityRetentionDays"` // 0 = keep forever
}

// DefaultRetentionSettings returns the default retention settings.
func DefaultRetentionSettings() RetentionSettings {
	return RetentionSettings{
		AuditRetentionDays:    90,
		ActivityRetentionDays: 30,
	}
}

// ReadRetentionSettings reads retention settings from the settings table, falling back to defaults.
func ReadRetentionSettings(db *sql.DB) RetentionSettings {
	s := DefaultRetentionSettings()

	rows, err := db.Query("SELECT key, value FROM settings WHERE category = 'retention'")
	if err != nil {
		return s
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "retention_audit_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.AuditRetentionDays = v
			}
		case "retention_activity_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.ActivityRetentionDays = v
			}
		}
	}

	return s
}

// ReadScannerSettings reads scanner settings from the settings table, falling back to defaults.
func ReadScannerSettings(db *sql.DB) ScannerSettings {
	s := DefaultScannerSettings()

	rows, err := db.Query("SELECT key, value FROM settings WHERE category = 'scanners'")
	if err != nil {
		return s
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "scanner_trivy_enabled":
			s.TrivyEnabled = value == "true"
		case "scanner_grype_enabled":
			s.GrypeEnabled = value == "true"
		case "scanner_clair_enabled":
			s.ClairEnabled = value == "true"
		case "scanner_clair_url":
			s.ClairURL = value
		case "scanner_default":
			if value == "trivy" || value == "grype" || value == "clair" {
				s.DefaultScanner = value
			}
		case "scanner_timeout":
			if v, err := strconv.Atoi(value); err == nil && v >= 30 && v <= 1800 {
				s.ScanTimeout = v
			}
		case "scanner_scan_on_install":
			s.ScanOnInstall = value == "true"
		case "scanner_scan_on_update":
			s.ScanOnUpdate = value == "true"
		}
	}

	return s
}
