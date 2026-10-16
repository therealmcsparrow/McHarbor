// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// The runbook engine's codes. ErrRunbookStepForbidden is the important one: it
// is returned with the missing permission key as data, because an operator at
// 03:00 needs to know what to ask for, not that something was refused.
const (
	ErrRunbookNotFound        MsgCode = "err.runbook.not_found"
	ErrRunbookRunNotFound     MsgCode = "err.runbook.run_not_found"
	ErrRunbookStepNotFound    MsgCode = "err.runbook.step_not_found"
	ErrRunbookRunFinished     MsgCode = "err.runbook.run_finished"
	ErrRunbookStepForbidden   MsgCode = "err.runbook.step_forbidden"
	ErrRunbookActionUnknown   MsgCode = "err.runbook.action_unknown"
	ErrRunbookDiagnosticUnkn  MsgCode = "err.runbook.diagnostic_unknown"
	ErrRunbookStepFailed      MsgCode = "err.runbook.step_failed"
	ErrRunbookNameRequired    MsgCode = "err.runbook.name_required"
	ErrRunbookStateInvalid    MsgCode = "err.runbook.state_invalid"
	ErrRunbookOutcomeInvalid  MsgCode = "err.runbook.outcome_invalid"
	ErrRunbookSkipNeedsReason MsgCode = "err.runbook.skip_needs_reason"
	MsgRunbookSaved           MsgCode = "msg.runbook.saved"
	MsgRunbookDeleted         MsgCode = "msg.runbook.deleted"
)

// runbookMessages holds the six translations of the runbook codes.
var runbookMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrRunbookNotFound:        "Runbook not found",
		ErrRunbookRunNotFound:     "Runbook run not found",
		ErrRunbookStepNotFound:    "That step is not part of this runbook",
		ErrRunbookRunFinished:     "This run has already finished; start a new one",
		ErrRunbookStepForbidden:   "You do not hold the permission this step needs",
		ErrRunbookActionUnknown:   "That action is not one a runbook step may take",
		ErrRunbookDiagnosticUnkn:  "That diagnostic is not one a runbook step may run",
		ErrRunbookStepFailed:      "The step could not be completed; its message is recorded on the run",
		ErrRunbookNameRequired:    "A runbook needs a name",
		ErrRunbookStateInvalid:    "A step result must be done, skipped or failed",
		ErrRunbookOutcomeInvalid:  "A run must finish as completed, abandoned or escalated",
		ErrRunbookSkipNeedsReason: "Say why the step was skipped",
		MsgRunbookSaved:           "Runbook saved",
		MsgRunbookDeleted:         "Runbook deleted",
	},
	NL: {
		ErrRunbookNotFound:        "Runbook niet gevonden",
		ErrRunbookRunNotFound:     "Runbook-uitvoering niet gevonden",
		ErrRunbookStepNotFound:    "Die stap hoort niet bij dit runbook",
		ErrRunbookRunFinished:     "Deze uitvoering is al afgerond; start een nieuwe",
		ErrRunbookStepForbidden:   "Je hebt de rechten niet die deze stap vereist",
		ErrRunbookActionUnknown:   "Die actie mag een runbookstap niet uitvoeren",
		ErrRunbookDiagnosticUnkn:  "Die diagnose mag een runbookstap niet uitvoeren",
		ErrRunbookStepFailed:      "De stap kon niet worden voltooid; het bericht staat op de uitvoering",
		ErrRunbookNameRequired:    "Een runbook heeft een naam nodig",
		ErrRunbookStateInvalid:    "Een stapresultaat moet done, skipped of failed zijn",
		ErrRunbookOutcomeInvalid:  "Een uitvoering moet eindigen als completed, abandoned of escalated",
		ErrRunbookSkipNeedsReason: "Geef aan waarom de stap is overgeslagen",
		MsgRunbookSaved:           "Runbook opgeslagen",
		MsgRunbookDeleted:         "Runbook verwijderd",
	},
	DE: {
		ErrRunbookNotFound:        "Runbook nicht gefunden",
		ErrRunbookRunNotFound:     "Runbook-Durchlauf nicht gefunden",
		ErrRunbookStepNotFound:    "Dieser Schritt gehört nicht zu diesem Runbook",
		ErrRunbookRunFinished:     "Dieser Durchlauf ist schon beendet; starten Sie einen neuen",
		ErrRunbookStepForbidden:   "Ihnen fehlt die Berechtigung, die dieser Schritt braucht",
		ErrRunbookActionUnknown:   "Diese Aktion darf ein Runbook-Schritt nicht ausführen",
		ErrRunbookDiagnosticUnkn:  "Diese Diagnose darf ein Runbook-Schritt nicht ausführen",
		ErrRunbookStepFailed:      "Der Schritt konnte nicht abgeschlossen werden; die Meldung ist am Durchlauf vermerkt",
		ErrRunbookNameRequired:    "Ein Runbook braucht einen Namen",
		ErrRunbookStateInvalid:    "Ein Schrittergebnis muss done, skipped oder failed sein",
		ErrRunbookOutcomeInvalid:  "Ein Durchlauf muss als completed, abandoned oder escalated enden",
		ErrRunbookSkipNeedsReason: "Nennen Sie den Grund für das Überspringen",
		MsgRunbookSaved:           "Runbook gespeichert",
		MsgRunbookDeleted:         "Runbook gelöscht",
	},
	ES: {
		ErrRunbookNotFound:        "Runbook no encontrado",
		ErrRunbookRunNotFound:     "Ejecución de runbook no encontrada",
		ErrRunbookStepNotFound:    "Ese paso no pertenece a este runbook",
		ErrRunbookRunFinished:     "Esta ejecución ya terminó; inicie una nueva",
		ErrRunbookStepForbidden:   "No tiene el permiso que necesita este paso",
		ErrRunbookActionUnknown:   "Esa acción no es una que un paso de runbook pueda realizar",
		ErrRunbookDiagnosticUnkn:  "Ese diagnóstico no es uno que un paso de runbook pueda ejecutar",
		ErrRunbookStepFailed:      "No se pudo completar el paso; su mensaje queda registrado en la ejecución",
		ErrRunbookNameRequired:    "Un runbook necesita un nombre",
		ErrRunbookStateInvalid:    "Un resultado de paso debe ser done, skipped o failed",
		ErrRunbookOutcomeInvalid:  "Una ejecución debe terminar como completed, abandoned o escalated",
		ErrRunbookSkipNeedsReason: "Indique por qué se omitió el paso",
		MsgRunbookSaved:           "Runbook guardado",
		MsgRunbookDeleted:         "Runbook eliminado",
	},
	FR: {
		ErrRunbookNotFound:        "Runbook introuvable",
		ErrRunbookRunNotFound:     "Exécution de runbook introuvable",
		ErrRunbookStepNotFound:    "Cette étape n'appartient pas à ce runbook",
		ErrRunbookRunFinished:     "Cette exécution est déjà terminée ; démarrez-en une nouvelle",
		ErrRunbookStepForbidden:   "Vous n'avez pas la permission requise par cette étape",
		ErrRunbookActionUnknown:   "Cette action n'est pas autorisée pour une étape de runbook",
		ErrRunbookDiagnosticUnkn:  "Ce diagnostic n'est pas autorisé pour une étape de runbook",
		ErrRunbookStepFailed:      "L'étape n'a pas pu être terminée ; son message est consigné sur l'exécution",
		ErrRunbookNameRequired:    "Un runbook a besoin d'un nom",
		ErrRunbookStateInvalid:    "Un résultat d'étape doit être done, skipped ou failed",
		ErrRunbookOutcomeInvalid:  "Une exécution doit se terminer en completed, abandoned ou escalated",
		ErrRunbookSkipNeedsReason: "Indiquez pourquoi l'étape a été ignorée",
		MsgRunbookSaved:           "Runbook enregistré",
		MsgRunbookDeleted:         "Runbook supprimé",
	},
	PT: {
		ErrRunbookNotFound:        "Runbook não encontrado",
		ErrRunbookRunNotFound:     "Execução de runbook não encontrada",
		ErrRunbookStepNotFound:    "Essa etapa não pertence a este runbook",
		ErrRunbookRunFinished:     "Esta execução já terminou; inicie uma nova",
		ErrRunbookStepForbidden:   "Você não tem a permissão que esta etapa exige",
		ErrRunbookActionUnknown:   "Essa ação não é permitida para uma etapa de runbook",
		ErrRunbookDiagnosticUnkn:  "Esse diagnóstico não é permitido para uma etapa de runbook",
		ErrRunbookStepFailed:      "A etapa não pôde ser concluída; a mensagem fica registada na execução",
		ErrRunbookNameRequired:    "Um runbook precisa de um nome",
		ErrRunbookStateInvalid:    "Um resultado de etapa deve ser done, skipped ou failed",
		ErrRunbookOutcomeInvalid:  "Uma execução deve terminar como completed, abandoned ou escalated",
		ErrRunbookSkipNeedsReason: "Diga por que a etapa foi ignorada",
		MsgRunbookSaved:           "Runbook guardado",
		MsgRunbookDeleted:         "Runbook eliminado",
	},
}

// init folds runbookMessages into the per-language maps T() reads, as
// codes_storage.go does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range runbookMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
