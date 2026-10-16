// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// The host services codes. Each refusal names its own rule, because a
// protected unit, an old agent and a containerised agent need different fixes.
const (
	ErrHostServicesListFailed      MsgCode = "err.host_services.list_failed"
	ErrHostServicesActionFailed    MsgCode = "err.host_services.action_failed"
	ErrHostServicesOverrideFailed  MsgCode = "err.host_services.override_failed"
	ErrHostServicesReloadFailed    MsgCode = "err.host_services.reload_failed"
	ErrHostServicesInvalidUnit     MsgCode = "err.host_services.invalid_unit"
	ErrHostServicesInvalidAction   MsgCode = "err.host_services.invalid_action"
	ErrHostServicesInvalidContent  MsgCode = "err.host_services.invalid_content"
	ErrHostServicesProtected       MsgCode = "err.host_services.protected"
	ErrHostServicesUnsupported     MsgCode = "err.host_services.unsupported"
	ErrHostServicesAgentOutdated   MsgCode = "err.host_services.agent_outdated"
	ErrHostServicesAgentOffline    MsgCode = "err.host_services.agent_offline"
	MsgHostServicesReloaded        MsgCode = "msg.host_services.reloaded"
	MsgHostServicesOverrideRemoved MsgCode = "msg.host_services.override_removed"
)

// hostServiceMessages holds the six translations of the host services codes.
var hostServiceMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrHostServicesListFailed:      "Could not list the host's services",
		ErrHostServicesActionFailed:    "systemd refused the service action",
		ErrHostServicesOverrideFailed:  "The service override could not be changed",
		ErrHostServicesReloadFailed:    "systemctl daemon-reload failed",
		ErrHostServicesInvalidUnit:     "That is not a valid .service unit name",
		ErrHostServicesInvalidAction:   "That service action is not supported",
		ErrHostServicesInvalidContent:  "The override must be UTF-8 text of at most 64 KiB",
		ErrHostServicesProtected:       "That unit is protected: McHarbor will not stop or disable it",
		ErrHostServicesUnsupported:     "Services can only be managed through a native agent on a host running systemd",
		ErrHostServicesAgentOutdated:   "This environment's agent is too old to manage services; update the agent",
		ErrHostServicesAgentOffline:    "The agent for this environment is not connected",
		MsgHostServicesReloaded:        "systemd configuration reloaded",
		MsgHostServicesOverrideRemoved: "Override removed and systemd reloaded",
	},
	NL: {
		ErrHostServicesListFailed:      "De services van de host konden niet worden opgehaald",
		ErrHostServicesActionFailed:    "systemd weigerde de serviceactie",
		ErrHostServicesOverrideFailed:  "De service-override kon niet worden gewijzigd",
		ErrHostServicesReloadFailed:    "systemctl daemon-reload is mislukt",
		ErrHostServicesInvalidUnit:     "Dat is geen geldige naam voor een .service-unit",
		ErrHostServicesInvalidAction:   "Die serviceactie wordt niet ondersteund",
		ErrHostServicesInvalidContent:  "De override moet UTF-8-tekst van maximaal 64 KiB zijn",
		ErrHostServicesProtected:       "Die unit is beschermd: McHarbor stopt of schakelt hem niet uit",
		ErrHostServicesUnsupported:     "Services kunnen alleen worden beheerd via een native agent op een host met systemd",
		ErrHostServicesAgentOutdated:   "De agent van deze omgeving is te oud om services te beheren; werk de agent bij",
		ErrHostServicesAgentOffline:    "De agent van deze omgeving is niet verbonden",
		MsgHostServicesReloaded:        "systemd-configuratie opnieuw geladen",
		MsgHostServicesOverrideRemoved: "Override verwijderd en systemd opnieuw geladen",
	},
	DE: {
		ErrHostServicesListFailed:      "Die Dienste des Hosts konnten nicht aufgelistet werden",
		ErrHostServicesActionFailed:    "systemd hat die Dienstaktion abgelehnt",
		ErrHostServicesOverrideFailed:  "Das Dienst-Override konnte nicht geändert werden",
		ErrHostServicesReloadFailed:    "systemctl daemon-reload ist fehlgeschlagen",
		ErrHostServicesInvalidUnit:     "Das ist kein gültiger Name für eine .service-Unit",
		ErrHostServicesInvalidAction:   "Diese Dienstaktion wird nicht unterstützt",
		ErrHostServicesInvalidContent:  "Das Override muss UTF-8-Text von höchstens 64 KiB sein",
		ErrHostServicesProtected:       "Diese Unit ist geschützt: McHarbor stoppt oder deaktiviert sie nicht",
		ErrHostServicesUnsupported:     "Dienste lassen sich nur über einen nativen Agent auf einem Host mit systemd verwalten",
		ErrHostServicesAgentOutdated:   "Der Agent dieser Umgebung ist zu alt, um Dienste zu verwalten; aktualisieren Sie den Agent",
		ErrHostServicesAgentOffline:    "Der Agent dieser Umgebung ist nicht verbunden",
		MsgHostServicesReloaded:        "systemd-Konfiguration neu geladen",
		MsgHostServicesOverrideRemoved: "Override entfernt und systemd neu geladen",
	},
	ES: {
		ErrHostServicesListFailed:      "No se pudieron listar los servicios del host",
		ErrHostServicesActionFailed:    "systemd rechazó la acción sobre el servicio",
		ErrHostServicesOverrideFailed:  "No se pudo cambiar la anulación del servicio",
		ErrHostServicesReloadFailed:    "systemctl daemon-reload falló",
		ErrHostServicesInvalidUnit:     "Ese no es un nombre de unidad .service válido",
		ErrHostServicesInvalidAction:   "Esa acción de servicio no está admitida",
		ErrHostServicesInvalidContent:  "La anulación debe ser texto UTF-8 de 64 KiB como máximo",
		ErrHostServicesProtected:       "Esa unidad está protegida: McHarbor no la detendrá ni la deshabilitará",
		ErrHostServicesUnsupported:     "Los servicios solo se pueden gestionar mediante un agente nativo en un host con systemd",
		ErrHostServicesAgentOutdated:   "El agente de este entorno es demasiado antiguo para gestionar servicios; actualice el agente",
		ErrHostServicesAgentOffline:    "El agente de este entorno no está conectado",
		MsgHostServicesReloaded:        "Configuración de systemd recargada",
		MsgHostServicesOverrideRemoved: "Anulación eliminada y systemd recargado",
	},
	FR: {
		ErrHostServicesListFailed:      "Impossible de lister les services de l'hôte",
		ErrHostServicesActionFailed:    "systemd a refusé l'action sur le service",
		ErrHostServicesOverrideFailed:  "La surcharge du service n'a pas pu être modifiée",
		ErrHostServicesReloadFailed:    "systemctl daemon-reload a échoué",
		ErrHostServicesInvalidUnit:     "Ce n'est pas un nom d'unité .service valide",
		ErrHostServicesInvalidAction:   "Cette action de service n'est pas prise en charge",
		ErrHostServicesInvalidContent:  "La surcharge doit être du texte UTF-8 de 64 Kio au plus",
		ErrHostServicesProtected:       "Cette unité est protégée : McHarbor ne l'arrêtera ni ne la désactivera",
		ErrHostServicesUnsupported:     "Les services ne peuvent être gérés que par un agent natif sur un hôte utilisant systemd",
		ErrHostServicesAgentOutdated:   "L'agent de cet environnement est trop ancien pour gérer les services ; mettez l'agent à jour",
		ErrHostServicesAgentOffline:    "L'agent de cet environnement n'est pas connecté",
		MsgHostServicesReloaded:        "Configuration systemd rechargée",
		MsgHostServicesOverrideRemoved: "Surcharge supprimée et systemd rechargé",
	},
	PT: {
		ErrHostServicesListFailed:      "Não foi possível listar os serviços do anfitrião",
		ErrHostServicesActionFailed:    "O systemd recusou a ação sobre o serviço",
		ErrHostServicesOverrideFailed:  "Não foi possível alterar a substituição do serviço",
		ErrHostServicesReloadFailed:    "systemctl daemon-reload falhou",
		ErrHostServicesInvalidUnit:     "Esse não é um nome de unidade .service válido",
		ErrHostServicesInvalidAction:   "Essa ação de serviço não é suportada",
		ErrHostServicesInvalidContent:  "A substituição deve ser texto UTF-8 com no máximo 64 KiB",
		ErrHostServicesProtected:       "Essa unidade está protegida: o McHarbor não a irá parar nem desativar",
		ErrHostServicesUnsupported:     "Os serviços só podem ser geridos através de um agente nativo num anfitrião com systemd",
		ErrHostServicesAgentOutdated:   "O agente deste ambiente é demasiado antigo para gerir serviços; atualize o agente",
		ErrHostServicesAgentOffline:    "O agente deste ambiente não está ligado",
		MsgHostServicesReloaded:        "Configuração do systemd recarregada",
		MsgHostServicesOverrideRemoved: "Substituição removida e systemd recarregado",
	},
}

// init folds hostServiceMessages into the per-language maps T() reads, as
// codes_agentreleases.go does.
func init() {
	for lang, msgs := range hostServiceMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
