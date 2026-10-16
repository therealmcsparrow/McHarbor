// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// The unattended-runbook and service-principal codes, kept beside
// codes_runbooks.go rather than inside it so that file's table stays the record
// of the operator-driven engine.
const (
	ErrRunbookCronInvalid         MsgCode = "err.runbook.cron_invalid"
	ErrRunbookPrincipalInvalid    MsgCode = "err.runbook.principal_invalid"
	ErrRunbookAutomationFirstStep MsgCode = "err.runbook.automation_first_step"
	ErrRunbookAutomationAlert     MsgCode = "err.runbook.automation_alert_attach"
	ErrRunbookAutomationNoTrigger MsgCode = "err.runbook.automation_no_trigger"
	ErrRunbookAutomateRequired    MsgCode = "err.runbook.automate_required"
	ErrUserServicePrincipalRole   MsgCode = "err.user.service_principal_role"
	ErrUserServicePrincipalSignIn MsgCode = "err.user.service_principal_sign_in"
	MsgRunbookAutomationSaved     MsgCode = "msg.runbook.automation_saved"
)

// runbookAutomationMessages holds the six translations of the codes above.
var runbookAutomationMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrRunbookCronInvalid:         "That schedule is not a valid five-field cron expression",
		ErrRunbookPrincipalInvalid:    "Choose an active service principal; a runbook cannot run unattended as a person",
		ErrRunbookAutomationFirstStep: "The first step is an instruction for a person, so an unattended run would do nothing",
		ErrRunbookAutomationAlert:     "Running on an alert needs a runbook attached to an alert type or rule",
		ErrRunbookAutomationNoTrigger: "Choose at least one trigger: the alert, a schedule, or both",
		ErrRunbookAutomateRequired:    "This runbook runs unattended; changing it needs the runbooks.automate permission",
		ErrUserServicePrincipalRole:   "A service principal needs a role",
		ErrUserServicePrincipalSignIn: "A service principal cannot sign in, so it has no password",
		MsgRunbookAutomationSaved:     "Unattended runs saved",
	},
	NL: {
		ErrRunbookCronInvalid:         "Dat schema is geen geldige cron-expressie met vijf velden",
		ErrRunbookPrincipalInvalid:    "Kies een actieve service principal; een runbook draait niet onbeheerd als persoon",
		ErrRunbookAutomationFirstStep: "De eerste stap is een instructie voor een persoon, dus een onbeheerde uitvoering zou niets doen",
		ErrRunbookAutomationAlert:     "Draaien bij een melding vereist een runbook dat aan een meldingstype of regel hangt",
		ErrRunbookAutomationNoTrigger: "Kies minstens één trigger: de melding, een schema, of beide",
		ErrRunbookAutomateRequired:    "Dit runbook draait onbeheerd; wijzigen vereist de rechten runbooks.automate",
		ErrUserServicePrincipalRole:   "Een service principal heeft een rol nodig",
		ErrUserServicePrincipalSignIn: "Een service principal kan niet inloggen en heeft dus geen wachtwoord",
		MsgRunbookAutomationSaved:     "Onbeheerde uitvoering opgeslagen",
	},
	DE: {
		ErrRunbookCronInvalid:         "Dieser Zeitplan ist kein gültiger Cron-Ausdruck mit fünf Feldern",
		ErrRunbookPrincipalInvalid:    "Wählen Sie einen aktiven Service-Principal; ein Runbook läuft nicht unbeaufsichtigt als Person",
		ErrRunbookAutomationFirstStep: "Der erste Schritt ist eine Anweisung für eine Person, ein unbeaufsichtigter Durchlauf täte nichts",
		ErrRunbookAutomationAlert:     "Ausführen bei einem Alarm braucht ein Runbook, das an einem Alarmtyp oder einer Regel hängt",
		ErrRunbookAutomationNoTrigger: "Wählen Sie mindestens einen Auslöser: den Alarm, einen Zeitplan oder beides",
		ErrRunbookAutomateRequired:    "Dieses Runbook läuft unbeaufsichtigt; Änderungen erfordern die Berechtigung runbooks.automate",
		ErrUserServicePrincipalRole:   "Ein Service-Principal braucht eine Rolle",
		ErrUserServicePrincipalSignIn: "Ein Service-Principal kann sich nicht anmelden und hat daher kein Passwort",
		MsgRunbookAutomationSaved:     "Unbeaufsichtigte Ausführung gespeichert",
	},
	ES: {
		ErrRunbookCronInvalid:         "Esa programación no es una expresión cron válida de cinco campos",
		ErrRunbookPrincipalInvalid:    "Elija un principal de servicio activo; un runbook no se ejecuta desatendido como una persona",
		ErrRunbookAutomationFirstStep: "El primer paso es una instrucción para una persona, así que una ejecución desatendida no haría nada",
		ErrRunbookAutomationAlert:     "Ejecutar ante una alerta requiere un runbook vinculado a un tipo o regla de alerta",
		ErrRunbookAutomationNoTrigger: "Elija al menos un disparador: la alerta, una programación o ambos",
		ErrRunbookAutomateRequired:    "Este runbook se ejecuta desatendido; cambiarlo requiere el permiso runbooks.automate",
		ErrUserServicePrincipalRole:   "Un principal de servicio necesita un rol",
		ErrUserServicePrincipalSignIn: "Un principal de servicio no puede iniciar sesión, así que no tiene contraseña",
		MsgRunbookAutomationSaved:     "Ejecución desatendida guardada",
	},
	FR: {
		ErrRunbookCronInvalid:         "Cette planification n'est pas une expression cron valide à cinq champs",
		ErrRunbookPrincipalInvalid:    "Choisissez un principal de service actif ; un runbook ne s'exécute pas sans surveillance en tant que personne",
		ErrRunbookAutomationFirstStep: "La première étape est une instruction pour une personne ; une exécution sans surveillance ne ferait rien",
		ErrRunbookAutomationAlert:     "L'exécution sur alerte exige un runbook rattaché à un type ou une règle d'alerte",
		ErrRunbookAutomationNoTrigger: "Choisissez au moins un déclencheur : l'alerte, une planification, ou les deux",
		ErrRunbookAutomateRequired:    "Ce runbook s'exécute sans surveillance ; le modifier exige la permission runbooks.automate",
		ErrUserServicePrincipalRole:   "Un principal de service a besoin d'un rôle",
		ErrUserServicePrincipalSignIn: "Un principal de service ne peut pas se connecter, il n'a donc pas de mot de passe",
		MsgRunbookAutomationSaved:     "Exécution sans surveillance enregistrée",
	},
	PT: {
		ErrRunbookCronInvalid:         "Esse agendamento não é uma expressão cron válida de cinco campos",
		ErrRunbookPrincipalInvalid:    "Escolha um principal de serviço ativo; um runbook não corre sem supervisão como uma pessoa",
		ErrRunbookAutomationFirstStep: "O primeiro passo é uma instrução para uma pessoa, por isso uma execução sem supervisão não faria nada",
		ErrRunbookAutomationAlert:     "Correr num alerta exige um runbook associado a um tipo ou regra de alerta",
		ErrRunbookAutomationNoTrigger: "Escolha pelo menos um gatilho: o alerta, um agendamento, ou ambos",
		ErrRunbookAutomateRequired:    "Este runbook corre sem supervisão; alterá-lo exige a permissão runbooks.automate",
		ErrUserServicePrincipalRole:   "Um principal de serviço precisa de uma função",
		ErrUserServicePrincipalSignIn: "Um principal de serviço não pode iniciar sessão, por isso não tem palavra-passe",
		MsgRunbookAutomationSaved:     "Execução sem supervisão guardada",
	},
}

// init folds runbookAutomationMessages into the per-language maps T() reads.
func init() {
	for lang, msgs := range runbookAutomationMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
