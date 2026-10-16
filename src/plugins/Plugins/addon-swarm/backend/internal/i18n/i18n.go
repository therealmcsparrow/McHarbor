// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package i18n is the Swarm plugin's own message catalog.
//
// These sixty-nine codes were `i18n.ErrSwarm*` / `i18n.MsgSwarm*` in the
// host's `core/i18n` until Swarm left the host. They are the plugin's now,
// which is the point: an installation that does not run Swarm no longer
// carries sixty-nine strings it can never emit.
//
// The code *strings* are unchanged. The frontend keys them by string, so a
// client written against the host version keeps resolving the same message.
package i18n

import (
	"net/http"
	"strings"
)

// MsgCode is a stable, translatable message identifier. Mirrors the host's
// type of the same name — a plugin cannot use the host's, because taking that
// dependency is what this migration removes.
type MsgCode string

// Lang is a supported UI language.
type Lang string

const (
	EN Lang = "en"
	NL Lang = "nl"
	DE Lang = "de"
	ES Lang = "es"
	FR Lang = "fr"
	PT Lang = "pt"
)

// FromRequest reads the caller's preferred language out of Accept-Language.
//
// Deliberately simpler than the host's negotiator: this catalog ships English
// only, so anything beyond "which language did they ask for" would be
// machinery with nothing behind it. When the other five bundles land, the
// shape here is already the one they plug into.
func FromRequest(r *http.Request) Lang {
	if r == nil {
		return EN
	}
	header := r.Header.Get("Accept-Language")
	for _, part := range strings.Split(header, ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		if tag == "" {
			continue
		}
		switch Lang(strings.SplitN(tag, "-", 2)[0]) {
		case NL:
			return NL
		case DE:
			return DE
		case ES:
			return ES
		case FR:
			return FR
		case PT:
			return PT
		case EN:
			return EN
		}
	}
	return EN
}

// T resolves a code to a sentence, falling back to English and then to the
// code itself. A code that reaches a client raw is a bug, not a crash.
func T(lang Lang, code MsgCode) string {
	if table, ok := translations[lang]; ok {
		if msg, ok := table[code]; ok {
			return msg
		}
	}
	if msg, ok := messagesEN[code]; ok {
		return msg
	}
	return string(code)
}

// Only English is populated. The other five resolve through T's fallback,
// exactly as they did in the host catalog — which never carried a Swarm
// translation either.
var translations = map[Lang]map[MsgCode]string{
	EN: messagesEN,
}

// Two codes the plugin shares with the host rather than owns. They keep the
// host's exact strings: an "Unauthorized" from a plugin route and one from a
// host route are the same event to the reader, and the frontend already has a
// message for both.
const (
	ErrUnauthorized MsgCode = "err.unauthorized"
	ErrInvalidBody  MsgCode = "err.invalid_body"
)

// --- Docker Swarm ---
const (
	ErrSwarmNotInitialised          MsgCode = "err.swarm.not_initialised"
	ErrSwarmNotManager              MsgCode = "err.swarm.not_manager"
	ErrSwarmProbeFailed             MsgCode = "err.swarm.probe_failed"
	ErrSwarmInspectFailed           MsgCode = "err.swarm.inspect_failed"
	ErrSwarmInitFailed              MsgCode = "err.swarm.init_failed"
	ErrSwarmJoinFailed              MsgCode = "err.swarm.join_failed"
	ErrSwarmLeaveFailed             MsgCode = "err.swarm.leave_failed"
	ErrSwarmUpdateFailed            MsgCode = "err.swarm.update_failed"
	ErrSwarmLastManager             MsgCode = "err.swarm.last_manager"
	ErrSwarmRemoveSelf              MsgCode = "err.swarm.remove_self"
	ErrSwarmTokensFailed            MsgCode = "err.swarm.tokens_failed"
	ErrSwarmUnlockKeyFailed         MsgCode = "err.swarm.unlock_key_failed"
	ErrSwarmUnlockFailed            MsgCode = "err.swarm.unlock_failed"
	ErrSwarmVersionConflict         MsgCode = "err.swarm.version_conflict"
	ErrSwarmOverviewFailed          MsgCode = "err.swarm.overview_failed"
	ErrSwarmTokenKindInvalid        MsgCode = "err.swarm.token_kind_invalid"
	ErrSwarmUnlockKeyRequired       MsgCode = "err.swarm.unlock_key_required"
	ErrSwarmRemoteAddrRequired      MsgCode = "err.swarm.remote_addr_required"
	ErrSwarmJoinTokenRequired       MsgCode = "err.swarm.join_token_required"
	ErrSwarmServiceListFailed       MsgCode = "err.swarm_service.list_failed"
	ErrSwarmServiceNotFound         MsgCode = "err.swarm_service.not_found"
	ErrSwarmServiceInspectFailed    MsgCode = "err.swarm_service.inspect_failed"
	ErrSwarmServiceCreateFailed     MsgCode = "err.swarm_service.create_failed"
	ErrSwarmServiceUpdateFailed     MsgCode = "err.swarm_service.update_failed"
	ErrSwarmServiceMaskedMoved      MsgCode = "err.swarm_service.masked_value_moved"
	ErrSwarmServiceRemoveFailed     MsgCode = "err.swarm_service.remove_failed"
	ErrSwarmServiceScaleFailed      MsgCode = "err.swarm_service.scale_failed"
	ErrSwarmServiceRestartFailed    MsgCode = "err.swarm_service.restart_failed"
	ErrSwarmServiceRollbackFailed   MsgCode = "err.swarm_service.rollback_failed"
	ErrSwarmServiceLogsFailed       MsgCode = "err.swarm_service.logs_failed"
	ErrSwarmServiceNotReplicated    MsgCode = "err.swarm_service.not_replicated"
	ErrSwarmServiceNoPrevious       MsgCode = "err.swarm_service.no_previous_spec"
	ErrSwarmServiceNameRequired     MsgCode = "err.swarm_service.name_required"
	ErrSwarmServiceImageRequired    MsgCode = "err.swarm_service.image_required"
	ErrSwarmReplicasInvalid         MsgCode = "err.swarm_service.replicas_invalid"
	ErrSwarmTaskListFailed          MsgCode = "err.swarm_task.list_failed"
	ErrSwarmTaskNotFound            MsgCode = "err.swarm_task.not_found"
	ErrSwarmTaskInspectFailed       MsgCode = "err.swarm_task.inspect_failed"
	ErrSwarmTaskLogsFailed          MsgCode = "err.swarm_task.logs_failed"
	ErrSwarmNodeListFailed          MsgCode = "err.swarm_node.list_failed"
	ErrSwarmNodeNotFound            MsgCode = "err.swarm_node.not_found"
	ErrSwarmNodeInspectFailed       MsgCode = "err.swarm_node.inspect_failed"
	ErrSwarmNodeUpdateFailed        MsgCode = "err.swarm_node.update_failed"
	ErrSwarmNodeRemoveFailed        MsgCode = "err.swarm_node.remove_failed"
	ErrSwarmNodeAvailabilityInvalid MsgCode = "err.swarm_node.availability_invalid"
	ErrSwarmNodeRoleInvalid         MsgCode = "err.swarm_node.role_invalid"
	ErrSwarmSecretListFailed        MsgCode = "err.swarm_secret.list_failed"
	ErrSwarmSecretNotFound          MsgCode = "err.swarm_secret.not_found"
	ErrSwarmSecretCreateFailed      MsgCode = "err.swarm_secret.create_failed"
	ErrSwarmSecretUpdateFailed      MsgCode = "err.swarm_secret.update_failed"
	ErrSwarmSecretRemoveFailed      MsgCode = "err.swarm_secret.remove_failed"
	ErrSwarmSecretNameRequired      MsgCode = "err.swarm_secret.name_required"
	ErrSwarmSecretPayloadInvalid    MsgCode = "err.swarm_secret.payload_invalid"
	ErrSwarmConfigListFailed        MsgCode = "err.swarm_config.list_failed"
	ErrSwarmConfigNotFound          MsgCode = "err.swarm_config.not_found"
	ErrSwarmConfigCreateFailed      MsgCode = "err.swarm_config.create_failed"
	ErrSwarmConfigUpdateFailed      MsgCode = "err.swarm_config.update_failed"
	ErrSwarmConfigRemoveFailed      MsgCode = "err.swarm_config.remove_failed"
	ErrSwarmConfigNameRequired      MsgCode = "err.swarm_config.name_required"
	ErrSwarmConfigPayloadInvalid    MsgCode = "err.swarm_config.payload_invalid"
	ErrSwarmStackListFailed         MsgCode = "err.swarm_stack.list_failed"
	ErrSwarmStackNotFound           MsgCode = "err.swarm_stack.not_found"
	ErrSwarmStackDeployFailed       MsgCode = "err.swarm_stack.deploy_failed"
	ErrSwarmStackRemoveFailed       MsgCode = "err.swarm_stack.remove_failed"
	ErrSwarmStackNameRequired       MsgCode = "err.swarm_stack.name_required"
	ErrSwarmStackComposeRequired    MsgCode = "err.swarm_stack.compose_required"
	ErrSwarmStackRejected           MsgCode = "err.swarm_stack.rejected"
	ErrSwarmNetworkListFailed       MsgCode = "err.swarm_network.list_failed"

	MsgSwarmInitialised  MsgCode = "msg.swarm.initialised"
	MsgSwarmJoined       MsgCode = "msg.swarm.joined"
	MsgSwarmLeft         MsgCode = "msg.swarm.left"
	MsgSwarmUnlocked     MsgCode = "msg.swarm.unlocked"
	MsgSwarmTokenRotated MsgCode = "msg.swarm.token_rotated"
)

var messagesEN = map[MsgCode]string{
	ErrUnauthorized: "Unauthorized",
	ErrInvalidBody:  "Invalid request body",

	ErrSwarmNotInitialised:     "This environment is not part of a Swarm",
	ErrSwarmNotManager:         "This node is a Swarm worker and cannot answer control-plane requests",
	ErrSwarmProbeFailed:        "Could not determine the Swarm state of this environment",
	ErrSwarmInspectFailed:      "Failed to inspect the Swarm cluster",
	ErrSwarmInitFailed:         "Failed to initialise the Swarm",
	ErrSwarmJoinFailed:         "Failed to join the Swarm",
	ErrSwarmLeaveFailed:        "Failed to leave the Swarm",
	ErrSwarmUpdateFailed:       "Failed to update the Swarm configuration",
	ErrSwarmLastManager:        "This is the only reachable manager in the cluster. Promote another node to manager first, or use Leave cluster with force to dissolve this Swarm",
	ErrSwarmRemoveSelf:         "A node cannot remove itself from the cluster. Use Leave cluster on this host instead",
	ErrSwarmTokensFailed:       "Failed to read the Swarm join tokens",
	ErrSwarmUnlockKeyFailed:    "Failed to read the Swarm unlock key",
	ErrSwarmUnlockFailed:       "Failed to unlock the Swarm. Check the unlock key",
	ErrSwarmVersionConflict:    "This object changed since it was read. Reload and try again",
	ErrSwarmOverviewFailed:     "Failed to load the Swarm overview",
	ErrSwarmTokenKindInvalid:   "Token kind must be worker or manager",
	ErrSwarmUnlockKeyRequired:  "An unlock key is required",
	ErrSwarmRemoteAddrRequired: "At least one remote manager address is required",
	ErrSwarmJoinTokenRequired:  "A join token is required",

	ErrSwarmServiceListFailed:     "Failed to list Swarm services",
	ErrSwarmServiceNotFound:       "Swarm service not found",
	ErrSwarmServiceInspectFailed:  "Failed to inspect the Swarm service",
	ErrSwarmServiceCreateFailed:   "Failed to create the Swarm service",
	ErrSwarmServiceUpdateFailed:   "Failed to update the Swarm service",
	ErrSwarmServiceMaskedMoved:    "A masked value (••••••••) was kept while the image, command, arguments or labels also changed. Type the real value, or save the two changes separately",
	ErrSwarmServiceRemoveFailed:   "Failed to remove the Swarm service",
	ErrSwarmServiceScaleFailed:    "Failed to scale the Swarm service",
	ErrSwarmServiceRestartFailed:  "Failed to restart the Swarm service",
	ErrSwarmServiceRollbackFailed: "Failed to roll the Swarm service back",
	ErrSwarmServiceLogsFailed:     "Failed to read the Swarm service logs",
	ErrSwarmServiceNotReplicated:  "This is a global service and has no replica count to change",
	ErrSwarmServiceNoPrevious:     "This service has no previous specification to roll back to",
	ErrSwarmServiceNameRequired:   "A service name is required",
	ErrSwarmServiceImageRequired:  "An image is required",
	ErrSwarmReplicasInvalid:       "Replica count must be zero or greater",

	ErrSwarmTaskListFailed:    "Failed to list Swarm tasks",
	ErrSwarmTaskNotFound:      "Swarm task not found",
	ErrSwarmTaskInspectFailed: "Failed to inspect the Swarm task",
	ErrSwarmTaskLogsFailed:    "Failed to read the Swarm task logs",

	ErrSwarmNodeListFailed:          "Failed to list Swarm nodes",
	ErrSwarmNodeNotFound:            "Swarm node not found",
	ErrSwarmNodeInspectFailed:       "Failed to inspect the Swarm node",
	ErrSwarmNodeUpdateFailed:        "Failed to update the Swarm node",
	ErrSwarmNodeRemoveFailed:        "Failed to remove the Swarm node",
	ErrSwarmNodeAvailabilityInvalid: "Availability must be active, pause or drain",
	ErrSwarmNodeRoleInvalid:         "Role must be manager or worker",

	ErrSwarmSecretListFailed:     "Failed to list Swarm secrets",
	ErrSwarmSecretNotFound:       "Swarm secret not found",
	ErrSwarmSecretCreateFailed:   "Failed to create the Swarm secret",
	ErrSwarmSecretUpdateFailed:   "Failed to update the Swarm secret",
	ErrSwarmSecretRemoveFailed:   "Failed to remove the Swarm secret",
	ErrSwarmSecretNameRequired:   "A secret name is required",
	ErrSwarmSecretPayloadInvalid: "The secret payload is not valid",

	ErrSwarmConfigListFailed:     "Failed to list Swarm configs",
	ErrSwarmConfigNotFound:       "Swarm config not found",
	ErrSwarmConfigCreateFailed:   "Failed to create the Swarm config",
	ErrSwarmConfigUpdateFailed:   "Failed to update the Swarm config",
	ErrSwarmConfigRemoveFailed:   "Failed to remove the Swarm config",
	ErrSwarmConfigNameRequired:   "A config name is required",
	ErrSwarmConfigPayloadInvalid: "The config payload is not valid",

	ErrSwarmStackListFailed:      "Failed to list Swarm stacks",
	ErrSwarmStackNotFound:        "Swarm stack not found",
	ErrSwarmStackDeployFailed:    "Failed to deploy the Swarm stack",
	ErrSwarmStackRemoveFailed:    "Failed to remove the Swarm stack",
	ErrSwarmStackNameRequired:    "A stack name is required",
	ErrSwarmStackComposeRequired: "A Compose file is required",
	ErrSwarmStackRejected:        "The Compose file cannot be deployed to a Swarm",

	ErrSwarmNetworkListFailed: "Failed to list Swarm networks",

	MsgSwarmInitialised:  "Swarm initialised",
	MsgSwarmJoined:       "Joined the Swarm",
	MsgSwarmLeft:         "Left the Swarm",
	MsgSwarmUnlocked:     "Swarm unlocked",
	MsgSwarmTokenRotated: "Join token rotated",
}
