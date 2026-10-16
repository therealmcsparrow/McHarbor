// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// The debug shell's codes. Each refusal names a property of the container rather
// than a McHarbor fault, because the operator's next step depends on which one it is.
const (
	ErrContainerHasShell         MsgCode = "err.container.has_shell"
	ErrContainerReadonlyRootfs   MsgCode = "err.container.readonly_rootfs"
	ErrContainerNotRunning       MsgCode = "err.container.not_running"
	ErrDebugShellUnsupportedArch MsgCode = "err.debug_shell.unsupported_arch"
	ErrDebugShellUnavailable     MsgCode = "err.debug_shell.binary_unavailable"
	ErrDebugShellInstallFailed   MsgCode = "err.debug_shell.install_failed"
	ErrDebugShellSessionInvalid  MsgCode = "err.debug_shell.session_invalid"
	ErrDebugShellStatusFailed    MsgCode = "err.debug_shell.status_failed"
	ErrDebugShellRemoveFailed    MsgCode = "err.debug_shell.remove_failed"
	MsgDebugShellRemoved         MsgCode = "msg.debug_shell.removed"
)

// debugShellMessages holds the six translations of the debug shell codes.
var debugShellMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrContainerHasShell:         "This container already has a shell; open the ordinary terminal instead",
		ErrContainerReadonlyRootfs:   "This container's root filesystem is read-only, so nothing can be placed in it",
		ErrContainerNotRunning:       "The container is not running",
		ErrDebugShellUnsupportedArch: "The container's host architecture has no bundled debug binary; amd64 and arm64 are supported",
		ErrDebugShellUnavailable:     "This McHarbor build does not carry the debug binary",
		ErrDebugShellInstallFailed:   "The debug shell could not be installed into the container",
		ErrDebugShellSessionInvalid:  "That debug session does not exist, has expired, was removed, or belongs to someone else",
		ErrDebugShellStatusFailed:    "Could not read the container's debug sessions",
		ErrDebugShellRemoveFailed:    "The debug binary could not be removed from the container",
		MsgDebugShellRemoved:         "Debug shell removed",
	},
	NL: {
		ErrContainerHasShell:         "Deze container heeft al een shell; open de gewone terminal",
		ErrContainerReadonlyRootfs:   "Het root-bestandssysteem van deze container is alleen-lezen, dus er kan niets in worden geplaatst",
		ErrContainerNotRunning:       "De container draait niet",
		ErrDebugShellUnsupportedArch: "Voor de architectuur van de host van deze container is geen debugprogramma meegeleverd; amd64 en arm64 worden ondersteund",
		ErrDebugShellUnavailable:     "Deze McHarbor-build bevat het debugprogramma niet",
		ErrDebugShellInstallFailed:   "De debugshell kon niet in de container worden geïnstalleerd",
		ErrDebugShellSessionInvalid:  "Die debugsessie bestaat niet, is verlopen, is verwijderd of hoort bij iemand anders",
		ErrDebugShellStatusFailed:    "De debugsessies van de container konden niet worden gelezen",
		ErrDebugShellRemoveFailed:    "Het debugprogramma kon niet uit de container worden verwijderd",
		MsgDebugShellRemoved:         "Debugshell verwijderd",
	},
	DE: {
		ErrContainerHasShell:         "Dieser Container hat bereits eine Shell; öffnen Sie das normale Terminal",
		ErrContainerReadonlyRootfs:   "Das Root-Dateisystem dieses Containers ist schreibgeschützt, daher kann nichts darin abgelegt werden",
		ErrContainerNotRunning:       "Der Container läuft nicht",
		ErrDebugShellUnsupportedArch: "Für die Architektur des Container-Hosts ist kein Debug-Programm enthalten; unterstützt werden amd64 und arm64",
		ErrDebugShellUnavailable:     "Dieser McHarbor-Build enthält das Debug-Programm nicht",
		ErrDebugShellInstallFailed:   "Die Debug-Shell konnte nicht in den Container installiert werden",
		ErrDebugShellSessionInvalid:  "Diese Debug-Sitzung existiert nicht, ist abgelaufen, wurde entfernt oder gehört jemand anderem",
		ErrDebugShellStatusFailed:    "Die Debug-Sitzungen des Containers konnten nicht gelesen werden",
		ErrDebugShellRemoveFailed:    "Das Debug-Programm konnte nicht aus dem Container entfernt werden",
		MsgDebugShellRemoved:         "Debug-Shell entfernt",
	},
	ES: {
		ErrContainerHasShell:         "Este contenedor ya tiene un shell; abra la terminal normal",
		ErrContainerReadonlyRootfs:   "El sistema de archivos raíz de este contenedor es de solo lectura, así que no se puede colocar nada en él",
		ErrContainerNotRunning:       "El contenedor no está en ejecución",
		ErrDebugShellUnsupportedArch: "No hay un binario de depuración incluido para la arquitectura del host del contenedor; se admiten amd64 y arm64",
		ErrDebugShellUnavailable:     "Esta compilación de McHarbor no incluye el binario de depuración",
		ErrDebugShellInstallFailed:   "No se pudo instalar el shell de depuración en el contenedor",
		ErrDebugShellSessionInvalid:  "Esa sesión de depuración no existe, ha caducado, se eliminó o pertenece a otra persona",
		ErrDebugShellStatusFailed:    "No se pudieron leer las sesiones de depuración del contenedor",
		ErrDebugShellRemoveFailed:    "No se pudo eliminar el binario de depuración del contenedor",
		MsgDebugShellRemoved:         "Shell de depuración eliminado",
	},
	FR: {
		ErrContainerHasShell:         "Ce conteneur a déjà un shell ; ouvrez le terminal habituel",
		ErrContainerReadonlyRootfs:   "Le système de fichiers racine de ce conteneur est en lecture seule, rien ne peut y être placé",
		ErrContainerNotRunning:       "Le conteneur n'est pas en cours d'exécution",
		ErrDebugShellUnsupportedArch: "Aucun binaire de débogage n'est fourni pour l'architecture de l'hôte du conteneur ; amd64 et arm64 sont pris en charge",
		ErrDebugShellUnavailable:     "Cette version de McHarbor ne contient pas le binaire de débogage",
		ErrDebugShellInstallFailed:   "Le shell de débogage n'a pas pu être installé dans le conteneur",
		ErrDebugShellSessionInvalid:  "Cette session de débogage n'existe pas, a expiré, a été supprimée ou appartient à quelqu'un d'autre",
		ErrDebugShellStatusFailed:    "Impossible de lire les sessions de débogage du conteneur",
		ErrDebugShellRemoveFailed:    "Le binaire de débogage n'a pas pu être retiré du conteneur",
		MsgDebugShellRemoved:         "Shell de débogage retiré",
	},
	PT: {
		ErrContainerHasShell:         "Este contentor já tem uma shell; abra o terminal normal",
		ErrContainerReadonlyRootfs:   "O sistema de ficheiros raiz deste contentor é só de leitura, por isso nada pode ser colocado nele",
		ErrContainerNotRunning:       "O contentor não está em execução",
		ErrDebugShellUnsupportedArch: "Não há binário de depuração incluído para a arquitetura do anfitrião do contentor; amd64 e arm64 são suportados",
		ErrDebugShellUnavailable:     "Esta compilação do McHarbor não inclui o binário de depuração",
		ErrDebugShellInstallFailed:   "Não foi possível instalar a shell de depuração no contentor",
		ErrDebugShellSessionInvalid:  "Essa sessão de depuração não existe, expirou, foi removida ou pertence a outra pessoa",
		ErrDebugShellStatusFailed:    "Não foi possível ler as sessões de depuração do contentor",
		ErrDebugShellRemoveFailed:    "Não foi possível remover o binário de depuração do contentor",
		MsgDebugShellRemoved:         "Shell de depuração removida",
	},
}

// init folds debugShellMessages into the per-language maps T() reads, as
// codes_runbooks.go does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range debugShellMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
