// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// updateChannelMessages holds the six translations of the prerelease update-channel
// codes declared in codes.go; each refusal names the rule so the operator knows the fix.
var updateChannelMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrUpdateDowngradeRefused:        "This would move to an older version, and McHarbor does not downgrade on its own",
		ErrUpdateMigrationBlocksStable:   "The installed prerelease has applied a database migration that the stable release does not have, so it cannot return to stable",
		ErrUpdateReturnOnPrereleaseHost:  "This McHarbor runs a prerelease. Return it to stable first, then return its plugins",
		ErrAgentNoStableRelease:          "No stable release with a build for this agent is available to return to",
		ErrPluginNoStableRelease:         "No stable release of this plugin is available to return to",
		ErrAgentNotConnected:             "The agent for this environment is not connected",
		ErrImageDigestPinnedNoPrerelease: "This image reference is pinned by digest, which names no version. Turn on prerelease updates or use a plain version tag",
		ErrUpdatePrereleaseOff:           "This is a prerelease and prerelease updates are switched off. Turn them on in Settings, then Updates, first",
	},
	NL: {
		ErrUpdateDowngradeRefused:        "Dit zou naar een oudere versie gaan, en McHarbor downgradet niet uit zichzelf",
		ErrUpdateMigrationBlocksStable:   "De geïnstalleerde prerelease heeft een databasemigratie toegepast die de stabiele release niet kent, dus terugkeren naar stabiel kan niet",
		ErrUpdateReturnOnPrereleaseHost:  "Deze McHarbor draait een prerelease. Zet hem eerst terug naar stabiel en daarna zijn plugins",
		ErrAgentNoStableRelease:          "Er is geen stabiele release met een build voor deze agent beschikbaar om naar terug te keren",
		ErrPluginNoStableRelease:         "Er is geen stabiele release van deze plugin beschikbaar om naar terug te keren",
		ErrAgentNotConnected:             "De agent van deze omgeving is niet verbonden",
		ErrImageDigestPinnedNoPrerelease: "Deze image-verwijzing is vastgezet op digest, die geen versie noemt. Zet prerelease-updates aan of gebruik een gewone versietag",
		ErrUpdatePrereleaseOff:           "Dit is een prerelease en prerelease-updates staan uit. Zet ze eerst aan via Instellingen, daarna Updates",
	},
	DE: {
		ErrUpdateDowngradeRefused:        "Dies würde auf eine ältere Version wechseln, und McHarbor führt von selbst kein Downgrade durch",
		ErrUpdateMigrationBlocksStable:   "Die installierte Vorabversion hat eine Datenbankmigration angewendet, die das stabile Release nicht kennt, daher ist die Rückkehr zu stabil nicht möglich",
		ErrUpdateReturnOnPrereleaseHost:  "Dieses McHarbor läuft als Vorabversion. Setzen Sie es zuerst auf stabil zurück und danach seine Plugins",
		ErrAgentNoStableRelease:          "Es ist kein stabiles Release mit einem Build für diesen Agent verfügbar, zu dem zurückgekehrt werden könnte",
		ErrPluginNoStableRelease:         "Es ist kein stabiles Release dieses Plugins verfügbar, zu dem zurückgekehrt werden könnte",
		ErrAgentNotConnected:             "Der Agent dieser Umgebung ist nicht verbunden",
		ErrImageDigestPinnedNoPrerelease: "Diese Image-Referenz ist per Digest festgelegt, der keine Version nennt. Aktiviere Vorabversions-Updates oder verwende ein normales Versions-Tag",
		ErrUpdatePrereleaseOff:           "Dies ist eine Vorabversion und Vorabversions-Updates sind ausgeschaltet. Schalten Sie sie zuerst unter Einstellungen, dann Updates ein",
	},
	ES: {
		ErrUpdateDowngradeRefused:        "Esto pasaría a una versión anterior, y McHarbor no hace downgrades por sí mismo",
		ErrUpdateMigrationBlocksStable:   "La versión preliminar instalada ha aplicado una migración de base de datos que la versión estable no tiene, por lo que no puede volver a la estable",
		ErrUpdateReturnOnPrereleaseHost:  "Este McHarbor ejecuta una versión preliminar. Devuélvalo primero a la estable y después sus plugins",
		ErrAgentNoStableRelease:          "No hay ninguna versión estable con compilación para este agente a la que volver",
		ErrPluginNoStableRelease:         "No hay ninguna versión estable de este plugin a la que volver",
		ErrAgentNotConnected:             "El agente de este entorno no está conectado",
		ErrImageDigestPinnedNoPrerelease: "Esta referencia de imagen está fijada por digest, que no indica ninguna versión. Activa las actualizaciones preliminares o usa una etiqueta de versión normal",
		ErrUpdatePrereleaseOff:           "Esta es una versión preliminar y las actualizaciones preliminares están desactivadas. Actívelas primero en Ajustes, y luego en Actualizaciones",
	},
	FR: {
		ErrUpdateDowngradeRefused:        "Cela passerait à une version plus ancienne, et McHarbor ne rétrograde pas de lui-même",
		ErrUpdateMigrationBlocksStable:   "La préversion installée a appliqué une migration de base de données que la version stable ne connaît pas, le retour à la stable est donc impossible",
		ErrUpdateReturnOnPrereleaseHost:  "Ce McHarbor exécute une préversion. Ramenez-le d'abord à la stable, puis ses plugins",
		ErrAgentNoStableRelease:          "Aucune version stable avec un build pour cet agent n'est disponible pour y revenir",
		ErrPluginNoStableRelease:         "Aucune version stable de ce plugin n'est disponible pour y revenir",
		ErrAgentNotConnected:             "L'agent de cet environnement n'est pas connecté",
		ErrImageDigestPinnedNoPrerelease: "Cette référence d'image est épinglée par digest, qui n'indique aucune version. Activez les mises à jour préliminaires ou utilisez une étiquette de version simple",
		ErrUpdatePrereleaseOff:           "Ceci est une préversion et les mises à jour en préversion sont désactivées. Activez-les d'abord dans Paramètres, puis Mises à jour",
	},
	PT: {
		ErrUpdateDowngradeRefused:        "Isto passaria para uma versão mais antiga, e o McHarbor não faz downgrade por conta própria",
		ErrUpdateMigrationBlocksStable:   "A pré-versão instalada aplicou uma migração de banco de dados que a versão estável não tem, então não pode voltar à estável",
		ErrUpdateReturnOnPrereleaseHost:  "Este McHarbor executa uma pré-versão. Volte-o primeiro para a estável e depois seus plugins",
		ErrAgentNoStableRelease:          "Não há nenhuma versão estável com build para este agente à qual voltar",
		ErrPluginNoStableRelease:         "Não há nenhuma versão estável deste plugin à qual voltar",
		ErrAgentNotConnected:             "O agente deste ambiente não está conectado",
		ErrImageDigestPinnedNoPrerelease: "Esta referência de imagem está fixada por digest, que não indica nenhuma versão. Ative as atualizações de pré-versão ou use uma tag de versão simples",
		ErrUpdatePrereleaseOff:           "Esta é uma pré-versão e as atualizações de pré-versão estão desativadas. Ative-as primeiro em Configurações, depois em Atualizações",
	},
}

// init folds updateChannelMessages into the per-language maps T() reads, as codes_hostself.go does.
func init() {
	for lang, msgs := range updateChannelMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
