// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/inapp"
)

func logInAppNotification(db *sql.DB, username *string, entry Entry) {
	record, ok := notificationRecord(username, entry)
	if !ok {
		return
	}

	if err := inapp.CreateBroadcast(db, record); err != nil {
		slog.Error("failed to write in-app notification", "action", entry.Action, "error", err)
	}
}

func notificationRecord(username *string, entry Entry) (inapp.Record, bool) {
	actionKey := normalizeNotificationAction(entry.Action)
	if actionKey == "" {
		return inapp.Record{}, false
	}

	entityLabel := humanizeNotificationToken(entry.EntityType)
	if entityLabel == "" {
		entityLabel = "System"
	}

	return inapp.Record{
		Level:      notificationLevel(actionKey),
		Title:      notificationTitle(entityLabel, actionKey),
		Message:    notificationMessage(entityLabel, entry.EntityName, actionKey, username),
		Action:     entry.Action,
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
	}, true
}

func normalizeNotificationAction(action string) string {
	normalized := strings.TrimSpace(strings.ToLower(action))
	if normalized == "" {
		return ""
	}

	switch normalized {
	case "login", "logout", "setup", "oidc_login":
		return ""
	}

	if parts := strings.Split(normalized, "."); len(parts) > 1 {
		normalized = parts[len(parts)-1]
	}

	return normalized
}

func notificationLevel(action string) string {
	switch action {
	case "delete", "deleted", "remove", "removed", "down", "stop", "kill", "revoke", "unassign_role", "remove_member", "uninstall":
		return "warning"
	default:
		return "success"
	}
}

func notificationTitle(entityLabel, action string) string {
	switch action {
	case "create", "created":
		return fmt.Sprintf("%s created", entityLabel)
	case "update", "updated":
		return fmt.Sprintf("%s updated", entityLabel)
	case "delete", "deleted":
		return fmt.Sprintf("%s deleted", entityLabel)
	case "remove", "removed":
		return fmt.Sprintf("%s removed", entityLabel)
	case "default_set":
		return fmt.Sprintf("Default %s updated", strings.ToLower(entityLabel))
	case "start", "up":
		return fmt.Sprintf("%s started", entityLabel)
	case "stop", "down":
		return fmt.Sprintf("%s stopped", entityLabel)
	case "restart":
		return fmt.Sprintf("%s restarted", entityLabel)
	case "reinstall":
		return fmt.Sprintf("%s reinstalled", entityLabel)
	case "pull":
		return fmt.Sprintf("%s pulled", entityLabel)
	case "tag":
		return fmt.Sprintf("%s tagged", entityLabel)
	case "import":
		return fmt.Sprintf("%s imported", entityLabel)
	case "export":
		return fmt.Sprintf("%s exported", entityLabel)
	case "assign_role":
		return "Role assigned"
	case "unassign_role":
		return "Role unassigned"
	case "add_member":
		return "Member added"
	case "remove_member":
		return "Member removed"
	case "deploy_agent":
		return "Agent deployed"
	case "create_install_token":
		return "Install token created"
	case "create_webhook":
		return "Webhook created"
	case "update_webhook":
		return "Webhook updated"
	case "delete_webhook":
		return "Webhook deleted"
	case "update_env_vars":
		return fmt.Sprintf("%s environment updated", entityLabel)
	default:
		return fmt.Sprintf("%s activity", entityLabel)
	}
}

func notificationMessage(entityLabel, entityName, action string, username *string) string {
	subject := entityName
	if strings.TrimSpace(subject) == "" {
		subject = fmt.Sprintf("The %s", strings.ToLower(entityLabel))
	}

	message := fmt.Sprintf("%s %s", subject, notificationPastTense(action))
	if username != nil && strings.TrimSpace(*username) != "" {
		message = fmt.Sprintf("%s by %s", message, *username)
	}

	return message
}

func notificationPastTense(action string) string {
	switch action {
	case "create", "created":
		return "was created"
	case "update", "updated":
		return "was updated"
	case "delete", "deleted":
		return "was deleted"
	case "remove", "removed":
		return "was removed"
	case "default_set":
		return "was set as default"
	case "start", "up":
		return "was started"
	case "stop", "down":
		return "was stopped"
	case "restart":
		return "was restarted"
	case "reinstall":
		return "was reinstalled"
	case "pull":
		return "was pulled"
	case "tag":
		return "was tagged"
	case "import":
		return "was imported"
	case "export":
		return "was exported"
	case "assign_role":
		return "was assigned"
	case "unassign_role":
		return "was unassigned"
	case "add_member":
		return "was added"
	case "remove_member":
		return "was removed"
	case "deploy_agent":
		return "was deployed"
	case "create_install_token":
		return "was created"
	case "create_webhook":
		return "webhook was created for"
	case "update_webhook":
		return "webhook was updated for"
	case "delete_webhook":
		return "webhook was deleted for"
	case "update_env_vars":
		return "environment variables were updated"
	default:
		return "was changed"
	}
}

func humanizeNotificationToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parts := strings.Fields(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(value))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}

	return strings.Join(parts, " ")
}
