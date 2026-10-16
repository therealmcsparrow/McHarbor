// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// The agent release store's codes. Refusals name the rule that refused, because
// each one points the operator at a different fix.
const (
	ErrAgentReleaseUnavailable     MsgCode = "err.agent_release.unavailable"
	ErrAgentReleaseListFailed      MsgCode = "err.agent_release.list_failed"
	ErrAgentReleaseNotFound        MsgCode = "err.agent_release.not_found"
	ErrAgentReleaseNotReady        MsgCode = "err.agent_release.not_ready"
	ErrAgentReleaseIsTarget        MsgCode = "err.agent_release.is_target"
	ErrAgentReleaseInUse           MsgCode = "err.agent_release.in_use"
	ErrAgentReleaseBusy            MsgCode = "err.agent_release.busy"
	ErrAgentReleaseInvalidVersion  MsgCode = "err.agent_release.invalid_version"
	ErrAgentReleaseInvalidSettings MsgCode = "err.agent_release.invalid_settings"
	ErrAgentReleaseCheckFailed     MsgCode = "err.agent_release.check_failed"
	ErrAgentReleasePullFailed      MsgCode = "err.agent_release.pull_failed"
	ErrAgentReleaseDeleteFailed    MsgCode = "err.agent_release.delete_failed"
	ErrAgentReleaseSaveFailed      MsgCode = "err.agent_release.save_failed"
	ErrAgentReleaseNoArtifact      MsgCode = "err.agent_release.no_artifact"
	MsgAgentReleasePullStarted     MsgCode = "msg.agent_release.pull_started"
	MsgAgentReleaseDeleted         MsgCode = "msg.agent_release.deleted"
	MsgAgentReleaseTargetSet       MsgCode = "msg.agent_release.target_set"
)

// agentReleaseMessages holds the six translations of the agent release codes.
var agentReleaseMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrAgentReleaseUnavailable:     "The agent release store is not available on this server",
		ErrAgentReleaseListFailed:      "Could not read the stored agent releases",
		ErrAgentReleaseNotFound:        "That agent release is not stored",
		ErrAgentReleaseNotReady:        "That agent release is not ready; only a verified, fully downloaded release can be the target",
		ErrAgentReleaseIsTarget:        "That release is the current target; choose another target before deleting it",
		ErrAgentReleaseInUse:           "A connected agent is running that release, so it is kept",
		ErrAgentReleaseBusy:            "That release is already being downloaded",
		ErrAgentReleaseInvalidVersion:  "That is not a valid agent version",
		ErrAgentReleaseInvalidSettings: "The agent release settings are invalid",
		ErrAgentReleaseCheckFailed:     "No release source answered the check",
		ErrAgentReleasePullFailed:      "The agent release could not be downloaded",
		ErrAgentReleaseDeleteFailed:    "The agent release could not be deleted",
		ErrAgentReleaseSaveFailed:      "The agent release settings could not be saved",
		ErrAgentReleaseNoArtifact:      "McHarbor holds no agent for this host's platform; download a release under Settings > Updates > Agents or build one locally",
		MsgAgentReleasePullStarted:     "Agent release download started",
		MsgAgentReleaseDeleted:         "Agent release deleted",
		MsgAgentReleaseTargetSet:       "Agent target updated",
	},
	NL: {
		ErrAgentReleaseUnavailable:     "De opslag voor agent-releases is op deze server niet beschikbaar",
		ErrAgentReleaseListFailed:      "De opgeslagen agent-releases konden niet worden gelezen",
		ErrAgentReleaseNotFound:        "Die agent-release is niet opgeslagen",
		ErrAgentReleaseNotReady:        "Die agent-release is niet gereed; alleen een geverifieerde, volledig gedownloade release kan het doel zijn",
		ErrAgentReleaseIsTarget:        "Die release is het huidige doel; kies eerst een ander doel",
		ErrAgentReleaseInUse:           "Een verbonden agent draait die release, dus hij blijft bewaard",
		ErrAgentReleaseBusy:            "Die release wordt al gedownload",
		ErrAgentReleaseInvalidVersion:  "Dat is geen geldige agentversie",
		ErrAgentReleaseInvalidSettings: "De instellingen voor agent-releases zijn ongeldig",
		ErrAgentReleaseCheckFailed:     "Geen enkele releasebron gaf antwoord",
		ErrAgentReleasePullFailed:      "De agent-release kon niet worden gedownload",
		ErrAgentReleaseDeleteFailed:    "De agent-release kon niet worden verwijderd",
		ErrAgentReleaseSaveFailed:      "De instellingen voor agent-releases konden niet worden opgeslagen",
		ErrAgentReleaseNoArtifact:      "McHarbor heeft geen agent voor het platform van deze host; download een release onder Instellingen > Updates > Agents of bouw er lokaal een",
		MsgAgentReleasePullStarted:     "Download van agent-release gestart",
		MsgAgentReleaseDeleted:         "Agent-release verwijderd",
		MsgAgentReleaseTargetSet:       "Agentdoel bijgewerkt",
	},
	DE: {
		ErrAgentReleaseUnavailable:     "Der Speicher für Agent-Releases ist auf diesem Server nicht verfügbar",
		ErrAgentReleaseListFailed:      "Die gespeicherten Agent-Releases konnten nicht gelesen werden",
		ErrAgentReleaseNotFound:        "Dieses Agent-Release ist nicht gespeichert",
		ErrAgentReleaseNotReady:        "Dieses Agent-Release ist nicht bereit; nur ein geprüftes, vollständig geladenes Release kann Ziel sein",
		ErrAgentReleaseIsTarget:        "Dieses Release ist das aktuelle Ziel; wählen Sie zuerst ein anderes Ziel",
		ErrAgentReleaseInUse:           "Ein verbundener Agent führt dieses Release aus, daher bleibt es erhalten",
		ErrAgentReleaseBusy:            "Dieses Release wird bereits heruntergeladen",
		ErrAgentReleaseInvalidVersion:  "Das ist keine gültige Agent-Version",
		ErrAgentReleaseInvalidSettings: "Die Einstellungen für Agent-Releases sind ungültig",
		ErrAgentReleaseCheckFailed:     "Keine Release-Quelle hat geantwortet",
		ErrAgentReleasePullFailed:      "Das Agent-Release konnte nicht heruntergeladen werden",
		ErrAgentReleaseDeleteFailed:    "Das Agent-Release konnte nicht gelöscht werden",
		ErrAgentReleaseSaveFailed:      "Die Einstellungen für Agent-Releases konnten nicht gespeichert werden",
		ErrAgentReleaseNoArtifact:      "McHarbor hat keinen Agent für die Plattform dieses Hosts; laden Sie ein Release unter Einstellungen > Updates > Agents herunter oder bauen Sie einen lokal",
		MsgAgentReleasePullStarted:     "Download des Agent-Release gestartet",
		MsgAgentReleaseDeleted:         "Agent-Release gelöscht",
		MsgAgentReleaseTargetSet:       "Agent-Ziel aktualisiert",
	},
	ES: {
		ErrAgentReleaseUnavailable:     "El almacén de versiones del agente no está disponible en este servidor",
		ErrAgentReleaseListFailed:      "No se pudieron leer las versiones del agente almacenadas",
		ErrAgentReleaseNotFound:        "Esa versión del agente no está almacenada",
		ErrAgentReleaseNotReady:        "Esa versión del agente no está lista; solo una versión verificada y descargada por completo puede ser el objetivo",
		ErrAgentReleaseIsTarget:        "Esa versión es el objetivo actual; elija otro objetivo antes de eliminarla",
		ErrAgentReleaseInUse:           "Un agente conectado ejecuta esa versión, así que se conserva",
		ErrAgentReleaseBusy:            "Esa versión ya se está descargando",
		ErrAgentReleaseInvalidVersion:  "No es una versión de agente válida",
		ErrAgentReleaseInvalidSettings: "La configuración de versiones del agente no es válida",
		ErrAgentReleaseCheckFailed:     "Ninguna fuente de versiones respondió",
		ErrAgentReleasePullFailed:      "No se pudo descargar la versión del agente",
		ErrAgentReleaseDeleteFailed:    "No se pudo eliminar la versión del agente",
		ErrAgentReleaseSaveFailed:      "No se pudo guardar la configuración de versiones del agente",
		ErrAgentReleaseNoArtifact:      "McHarbor no tiene un agente para la plataforma de este host; descargue una versión en Configuración > Actualizaciones > Agentes o compile uno localmente",
		MsgAgentReleasePullStarted:     "Descarga de la versión del agente iniciada",
		MsgAgentReleaseDeleted:         "Versión del agente eliminada",
		MsgAgentReleaseTargetSet:       "Objetivo del agente actualizado",
	},
	FR: {
		ErrAgentReleaseUnavailable:     "Le dépôt des versions de l'agent n'est pas disponible sur ce serveur",
		ErrAgentReleaseListFailed:      "Impossible de lire les versions de l'agent stockées",
		ErrAgentReleaseNotFound:        "Cette version de l'agent n'est pas stockée",
		ErrAgentReleaseNotReady:        "Cette version de l'agent n'est pas prête ; seule une version vérifiée et entièrement téléchargée peut être la cible",
		ErrAgentReleaseIsTarget:        "Cette version est la cible actuelle ; choisissez une autre cible avant de la supprimer",
		ErrAgentReleaseInUse:           "Un agent connecté exécute cette version, elle est donc conservée",
		ErrAgentReleaseBusy:            "Cette version est déjà en cours de téléchargement",
		ErrAgentReleaseInvalidVersion:  "Ce n'est pas une version d'agent valide",
		ErrAgentReleaseInvalidSettings: "Les paramètres des versions de l'agent ne sont pas valides",
		ErrAgentReleaseCheckFailed:     "Aucune source de versions n'a répondu",
		ErrAgentReleasePullFailed:      "La version de l'agent n'a pas pu être téléchargée",
		ErrAgentReleaseDeleteFailed:    "La version de l'agent n'a pas pu être supprimée",
		ErrAgentReleaseSaveFailed:      "Les paramètres des versions de l'agent n'ont pas pu être enregistrés",
		ErrAgentReleaseNoArtifact:      "McHarbor ne dispose d'aucun agent pour la plateforme de cet hôte ; téléchargez une version dans Paramètres > Mises à jour > Agents ou compilez-en un localement",
		MsgAgentReleasePullStarted:     "Téléchargement de la version de l'agent lancé",
		MsgAgentReleaseDeleted:         "Version de l'agent supprimée",
		MsgAgentReleaseTargetSet:       "Cible de l'agent mise à jour",
	},
	PT: {
		ErrAgentReleaseUnavailable:     "O repositório de versões do agente não está disponível neste servidor",
		ErrAgentReleaseListFailed:      "Não foi possível ler as versões do agente armazenadas",
		ErrAgentReleaseNotFound:        "Essa versão do agente não está armazenada",
		ErrAgentReleaseNotReady:        "Essa versão do agente não está pronta; só uma versão verificada e totalmente transferida pode ser o alvo",
		ErrAgentReleaseIsTarget:        "Essa versão é o alvo atual; escolha outro alvo antes de a eliminar",
		ErrAgentReleaseInUse:           "Um agente ligado está a executar essa versão, por isso é mantida",
		ErrAgentReleaseBusy:            "Essa versão já está a ser transferida",
		ErrAgentReleaseInvalidVersion:  "Não é uma versão de agente válida",
		ErrAgentReleaseInvalidSettings: "As definições das versões do agente são inválidas",
		ErrAgentReleaseCheckFailed:     "Nenhuma fonte de versões respondeu",
		ErrAgentReleasePullFailed:      "Não foi possível transferir a versão do agente",
		ErrAgentReleaseDeleteFailed:    "Não foi possível eliminar a versão do agente",
		ErrAgentReleaseSaveFailed:      "Não foi possível guardar as definições das versões do agente",
		ErrAgentReleaseNoArtifact:      "O McHarbor não tem um agente para a plataforma deste anfitrião; transfira uma versão em Definições > Atualizações > Agentes ou compile um localmente",
		MsgAgentReleasePullStarted:     "Transferência da versão do agente iniciada",
		MsgAgentReleaseDeleted:         "Versão do agente eliminada",
		MsgAgentReleaseTargetSet:       "Alvo do agente atualizado",
	},
}

// init folds agentReleaseMessages into the per-language maps T() reads, as
// codes_debugshell.go does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range agentReleaseMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
