// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// Message codes for the database backend selector and the cross-backend
// data migration (modules/database).
//
// They live in their own file, and register themselves into the six
// translation maps from init(), rather than being appended to the six
// literal maps in messages.go. That file is 4,600 lines of one flat map per
// language; a feature that adds sixteen codes to all six of them by hand is
// six merge conflicts waiting to happen, and the codes end up scattered
// hundreds of lines apart from each other.
const (
	ErrDatabaseConfigInvalid       MsgCode = "err.database.config_invalid"
	ErrDatabaseConnectFailed       MsgCode = "err.database.connect_failed"
	ErrDatabaseAuthFailed          MsgCode = "err.database.auth_failed"
	ErrDatabaseUnreachable         MsgCode = "err.database.unreachable"
	ErrDatabaseVersionUnsupported  MsgCode = "err.database.version_unsupported"
	ErrDatabasePrivileges          MsgCode = "err.database.privileges"
	ErrDatabaseEncoding            MsgCode = "err.database.encoding"
	ErrDatabaseEnvPinned           MsgCode = "err.database.env_pinned"
	ErrDatabaseMigrationRunning    MsgCode = "err.database.migration_running"
	ErrDatabaseMigrationInProgress MsgCode = "err.database.migration_in_progress"
	ErrDatabaseMigrationFailed     MsgCode = "err.database.migration_failed"
	ErrDatabaseMigrationNotFound   MsgCode = "err.database.migration_not_found"
	ErrDatabaseSetupCompleted      MsgCode = "err.database.setup_completed"
	ErrDatabaseSameTarget          MsgCode = "err.database.same_target"
	ErrDatabaseSchemaFailed        MsgCode = "err.database.schema_failed"
	ErrDatabaseCleanupUnavailable  MsgCode = "err.database.cleanup_unavailable"
	ErrDatabaseCleanupFailed       MsgCode = "err.database.cleanup_failed"
	ErrDatabaseCheckFailed         MsgCode = "err.database.check_failed"
	ErrDatabaseCheckRunning        MsgCode = "err.database.check_running"
	ErrDatabaseCheckNotFound       MsgCode = "err.database.check_not_found"
	ErrDatabaseRepairFailed        MsgCode = "err.database.repair_failed"
	ErrDatabaseRepairUnknown       MsgCode = "err.database.repair_unknown_finding"

	MsgDatabaseTestPassed       MsgCode = "msg.database.test_passed"
	MsgDatabaseMigrationStarted MsgCode = "msg.database.migration_started"
	MsgDatabaseConfigSaved      MsgCode = "msg.database.config_saved"
	MsgDatabaseMigrationDone    MsgCode = "msg.database.migration_done"
	MsgDatabaseCheckCancelled   MsgCode = "msg.database.check_cancelled"
)

// databaseMessages holds the six translations of every code above.
var databaseMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrDatabaseConfigInvalid:       "The database connection details are incomplete or malformed",
		ErrDatabaseConnectFailed:       "Could not connect to the database",
		ErrDatabaseAuthFailed:          "The database rejected the username or password",
		ErrDatabaseUnreachable:         "The database host could not be reached",
		ErrDatabaseVersionUnsupported:  "That database server version is too old for McHarbor",
		ErrDatabasePrivileges:          "The database user may not create tables in this database",
		ErrDatabaseEncoding:            "The database is not using a UTF-8 character set",
		ErrDatabaseEnvPinned:           "The database is pinned by MCHARBOR_DB_DRIVER and cannot be changed from the interface",
		ErrDatabaseMigrationRunning:    "A database migration is already running",
		ErrDatabaseMigrationInProgress: "McHarbor is migrating its database; changes are paused until it finishes",
		ErrDatabaseMigrationFailed:     "The database migration failed",
		ErrDatabaseMigrationNotFound:   "No database migration has been run",
		ErrDatabaseSetupCompleted:      "The first-run database setup is already complete",
		ErrDatabaseSameTarget:          "That is the database McHarbor is already running on",
		ErrDatabaseSchemaFailed:        "The McHarbor schema could not be created on the target database",
		ErrDatabaseCleanupUnavailable:  "There is no old database this install may delete",
		ErrDatabaseCleanupFailed:       "The old database files could not be deleted",
		ErrDatabaseCheckFailed:         "The database check could not be completed",
		ErrDatabaseCheckRunning:        "A database check is already running",
		ErrDatabaseCheckNotFound:       "No database check has been run",
		ErrDatabaseRepairFailed:        "The repair could not be completed",
		ErrDatabaseRepairUnknown:       "That finding is not part of the current report",

		MsgDatabaseTestPassed:       "The database connection test passed",
		MsgDatabaseMigrationStarted: "Database migration started",
		MsgDatabaseConfigSaved:      "Database configuration saved",
		MsgDatabaseMigrationDone:    "Database migration complete",
		MsgDatabaseCheckCancelled:   "Database check cancelled",
	},
	NL: {
		ErrDatabaseConfigInvalid:       "De databasegegevens zijn onvolledig of ongeldig",
		ErrDatabaseConnectFailed:       "Kon geen verbinding maken met de database",
		ErrDatabaseAuthFailed:          "De database weigerde de gebruikersnaam of het wachtwoord",
		ErrDatabaseUnreachable:         "De databasehost is niet bereikbaar",
		ErrDatabaseVersionUnsupported:  "Deze versie van de databaseserver is te oud voor McHarbor",
		ErrDatabasePrivileges:          "De databasegebruiker mag geen tabellen aanmaken in deze database",
		ErrDatabaseEncoding:            "De database gebruikt geen UTF-8-tekenset",
		ErrDatabaseEnvPinned:           "De database is vastgezet met MCHARBOR_DB_DRIVER en kan niet via de interface worden gewijzigd",
		ErrDatabaseMigrationRunning:    "Er loopt al een databasemigratie",
		ErrDatabaseMigrationInProgress: "McHarbor migreert de database; wijzigingen zijn gepauzeerd tot dat klaar is",
		ErrDatabaseMigrationFailed:     "De databasemigratie is mislukt",
		ErrDatabaseMigrationNotFound:   "Er is nog geen databasemigratie uitgevoerd",
		ErrDatabaseSetupCompleted:      "De eerste database-instelling is al voltooid",
		ErrDatabaseSameTarget:          "Dit is de database waarop McHarbor al draait",
		ErrDatabaseSchemaFailed:        "Het McHarbor-schema kon niet worden aangemaakt op de doeldatabase",
		ErrDatabaseCleanupUnavailable:  "Er is geen oude database die deze installatie mag verwijderen",
		ErrDatabaseCleanupFailed:       "De oude databasebestanden konden niet worden verwijderd",
		ErrDatabaseCheckFailed:         "De databasecontrole kon niet worden voltooid",
		ErrDatabaseCheckRunning:        "Er wordt al een databasecontrole uitgevoerd",
		ErrDatabaseCheckNotFound:       "Er is nog geen databasecontrole uitgevoerd",
		ErrDatabaseRepairFailed:        "De reparatie kon niet worden voltooid",
		ErrDatabaseRepairUnknown:       "Die bevinding maakt geen deel uit van het huidige rapport",

		MsgDatabaseTestPassed:       "De databaseverbindingstest is geslaagd",
		MsgDatabaseMigrationStarted: "Databasemigratie gestart",
		MsgDatabaseConfigSaved:      "Databaseconfiguratie opgeslagen",
		MsgDatabaseMigrationDone:    "Databasemigratie voltooid",
		MsgDatabaseCheckCancelled:   "Databasecontrole geannuleerd",
	},
	DE: { //nolint:gosec // translated error text mentioning a password, not a credential
		ErrDatabaseConfigInvalid:       "Die Datenbank-Verbindungsdaten sind unvollständig oder ungültig",
		ErrDatabaseConnectFailed:       "Verbindung zur Datenbank fehlgeschlagen",
		ErrDatabaseAuthFailed:          "Die Datenbank hat Benutzername oder Passwort abgelehnt",
		ErrDatabaseUnreachable:         "Der Datenbank-Host ist nicht erreichbar",
		ErrDatabaseVersionUnsupported:  "Diese Version des Datenbankservers ist zu alt für McHarbor",
		ErrDatabasePrivileges:          "Der Datenbankbenutzer darf in dieser Datenbank keine Tabellen anlegen",
		ErrDatabaseEncoding:            "Die Datenbank verwendet keinen UTF-8-Zeichensatz",
		ErrDatabaseEnvPinned:           "Die Datenbank ist über MCHARBOR_DB_DRIVER festgelegt und kann nicht über die Oberfläche geändert werden",
		ErrDatabaseMigrationRunning:    "Es läuft bereits eine Datenbankmigration",
		ErrDatabaseMigrationInProgress: "McHarbor migriert gerade seine Datenbank; Änderungen sind bis zum Abschluss pausiert",
		ErrDatabaseMigrationFailed:     "Die Datenbankmigration ist fehlgeschlagen",
		ErrDatabaseMigrationNotFound:   "Es wurde noch keine Datenbankmigration ausgeführt",
		ErrDatabaseSetupCompleted:      "Die erstmalige Datenbankeinrichtung ist bereits abgeschlossen",
		ErrDatabaseSameTarget:          "Auf dieser Datenbank läuft McHarbor bereits",
		ErrDatabaseSchemaFailed:        "Das McHarbor-Schema konnte in der Zieldatenbank nicht angelegt werden",
		ErrDatabaseCleanupUnavailable:  "Es gibt keine alte Datenbank, die diese Installation löschen darf",
		ErrDatabaseCleanupFailed:       "Die alten Datenbankdateien konnten nicht gelöscht werden",
		ErrDatabaseCheckFailed:         "Die Datenbankprüfung konnte nicht abgeschlossen werden",
		ErrDatabaseCheckRunning:        "Es läuft bereits eine Datenbankprüfung",
		ErrDatabaseCheckNotFound:       "Es wurde noch keine Datenbankprüfung durchgeführt",
		ErrDatabaseRepairFailed:        "Die Reparatur konnte nicht abgeschlossen werden",
		ErrDatabaseRepairUnknown:       "Dieser Befund gehört nicht zum aktuellen Bericht",

		MsgDatabaseTestPassed:       "Der Datenbank-Verbindungstest war erfolgreich",
		MsgDatabaseMigrationStarted: "Datenbankmigration gestartet",
		MsgDatabaseConfigSaved:      "Datenbankkonfiguration gespeichert",
		MsgDatabaseMigrationDone:    "Datenbankmigration abgeschlossen",
		MsgDatabaseCheckCancelled:   "Datenbankprüfung abgebrochen",
	},
	ES: { //nolint:gosec // translated error text mentioning a password, not a credential
		ErrDatabaseConfigInvalid:       "Los datos de conexión a la base de datos están incompletos o son inválidos",
		ErrDatabaseConnectFailed:       "No se pudo conectar con la base de datos",
		ErrDatabaseAuthFailed:          "La base de datos rechazó el usuario o la contraseña",
		ErrDatabaseUnreachable:         "No se pudo alcanzar el servidor de base de datos",
		ErrDatabaseVersionUnsupported:  "Esa versión del servidor de base de datos es demasiado antigua para McHarbor",
		ErrDatabasePrivileges:          "El usuario de la base de datos no puede crear tablas en ella",
		ErrDatabaseEncoding:            "La base de datos no usa un juego de caracteres UTF-8",
		ErrDatabaseEnvPinned:           "La base de datos está fijada por MCHARBOR_DB_DRIVER y no se puede cambiar desde la interfaz",
		ErrDatabaseMigrationRunning:    "Ya hay una migración de base de datos en curso",
		ErrDatabaseMigrationInProgress: "McHarbor está migrando su base de datos; los cambios están pausados hasta que termine",
		ErrDatabaseMigrationFailed:     "La migración de la base de datos falló",
		ErrDatabaseMigrationNotFound:   "Todavía no se ha ejecutado ninguna migración de base de datos",
		ErrDatabaseSetupCompleted:      "La configuración inicial de la base de datos ya está completa",
		ErrDatabaseSameTarget:          "Esa es la base de datos en la que McHarbor ya se ejecuta",
		ErrDatabaseSchemaFailed:        "No se pudo crear el esquema de McHarbor en la base de datos de destino",
		ErrDatabaseCleanupUnavailable:  "No hay ninguna base de datos antigua que esta instalación pueda eliminar",
		ErrDatabaseCleanupFailed:       "No se pudieron eliminar los archivos de la base de datos antigua",
		ErrDatabaseCheckFailed:         "No se pudo completar la comprobación de la base de datos",
		ErrDatabaseCheckRunning:        "Ya hay una comprobación de la base de datos en curso",
		ErrDatabaseCheckNotFound:       "Todavía no se ha ejecutado ninguna comprobación de la base de datos",
		ErrDatabaseRepairFailed:        "No se pudo completar la reparación",
		ErrDatabaseRepairUnknown:       "Ese hallazgo no forma parte del informe actual",

		MsgDatabaseTestPassed:       "La prueba de conexión a la base de datos fue correcta",
		MsgDatabaseMigrationStarted: "Migración de base de datos iniciada",
		MsgDatabaseConfigSaved:      "Configuración de base de datos guardada",
		MsgDatabaseMigrationDone:    "Migración de base de datos completada",
		MsgDatabaseCheckCancelled:   "Comprobación de la base de datos cancelada",
	},
	FR: { //nolint:gosec // translated error text mentioning a password, not a credential
		ErrDatabaseConfigInvalid:       "Les informations de connexion à la base de données sont incomplètes ou invalides",
		ErrDatabaseConnectFailed:       "Impossible de se connecter à la base de données",
		ErrDatabaseAuthFailed:          "La base de données a refusé le nom d'utilisateur ou le mot de passe",
		ErrDatabaseUnreachable:         "L'hôte de la base de données est injoignable",
		ErrDatabaseVersionUnsupported:  "Cette version du serveur de base de données est trop ancienne pour McHarbor",
		ErrDatabasePrivileges:          "L'utilisateur de la base de données ne peut pas y créer de tables",
		ErrDatabaseEncoding:            "La base de données n'utilise pas un jeu de caractères UTF-8",
		ErrDatabaseEnvPinned:           "La base de données est figée par MCHARBOR_DB_DRIVER et ne peut pas être modifiée depuis l'interface",
		ErrDatabaseMigrationRunning:    "Une migration de base de données est déjà en cours",
		ErrDatabaseMigrationInProgress: "McHarbor migre sa base de données ; les modifications sont suspendues jusqu'à la fin",
		ErrDatabaseMigrationFailed:     "La migration de la base de données a échoué",
		ErrDatabaseMigrationNotFound:   "Aucune migration de base de données n'a encore été exécutée",
		ErrDatabaseSetupCompleted:      "La configuration initiale de la base de données est déjà terminée",
		ErrDatabaseSameTarget:          "C'est déjà la base de données utilisée par McHarbor",
		ErrDatabaseSchemaFailed:        "Le schéma McHarbor n'a pas pu être créé dans la base de données cible",
		ErrDatabaseCleanupUnavailable:  "Il n'y a aucune ancienne base de données que cette installation puisse supprimer",
		ErrDatabaseCleanupFailed:       "Les anciens fichiers de base de données n'ont pas pu être supprimés",
		ErrDatabaseCheckFailed:         "La vérification de la base de données n'a pas pu être terminée",
		ErrDatabaseCheckRunning:        "Une vérification de la base de données est déjà en cours",
		ErrDatabaseCheckNotFound:       "Aucune vérification de la base de données n'a encore été effectuée",
		ErrDatabaseRepairFailed:        "La réparation n'a pas pu être terminée",
		ErrDatabaseRepairUnknown:       "Ce résultat ne fait pas partie du rapport actuel",

		MsgDatabaseTestPassed:       "Le test de connexion à la base de données a réussi",
		MsgDatabaseMigrationStarted: "Migration de base de données démarrée",
		MsgDatabaseConfigSaved:      "Configuration de la base de données enregistrée",
		MsgDatabaseMigrationDone:    "Migration de base de données terminée",
		MsgDatabaseCheckCancelled:   "Vérification de la base de données annulée",
	},
	PT: { //nolint:gosec // translated error text mentioning a password, not a credential
		ErrDatabaseConfigInvalid:       "Os dados de ligação à base de dados estão incompletos ou inválidos",
		ErrDatabaseConnectFailed:       "Não foi possível ligar à base de dados",
		ErrDatabaseAuthFailed:          "A base de dados rejeitou o utilizador ou a palavra-passe",
		ErrDatabaseUnreachable:         "Não foi possível alcançar o servidor da base de dados",
		ErrDatabaseVersionUnsupported:  "Essa versão do servidor de base de dados é demasiado antiga para o McHarbor",
		ErrDatabasePrivileges:          "O utilizador da base de dados não pode criar tabelas nesta base de dados",
		ErrDatabaseEncoding:            "A base de dados não usa um conjunto de caracteres UTF-8",
		ErrDatabaseEnvPinned:           "A base de dados está fixada por MCHARBOR_DB_DRIVER e não pode ser alterada pela interface",
		ErrDatabaseMigrationRunning:    "Já está a decorrer uma migração de base de dados",
		ErrDatabaseMigrationInProgress: "O McHarbor está a migrar a base de dados; as alterações estão pausadas até terminar",
		ErrDatabaseMigrationFailed:     "A migração da base de dados falhou",
		ErrDatabaseMigrationNotFound:   "Ainda não foi executada nenhuma migração de base de dados",
		ErrDatabaseSetupCompleted:      "A configuração inicial da base de dados já está concluída",
		ErrDatabaseSameTarget:          "Essa é a base de dados em que o McHarbor já está a correr",
		ErrDatabaseSchemaFailed:        "Não foi possível criar o esquema do McHarbor na base de dados de destino",
		ErrDatabaseCleanupUnavailable:  "Não existe nenhuma base de dados antiga que esta instalação possa eliminar",
		ErrDatabaseCleanupFailed:       "Não foi possível eliminar os ficheiros da base de dados antiga",
		ErrDatabaseCheckFailed:         "Não foi possível concluir a verificação da base de dados",
		ErrDatabaseCheckRunning:        "Já está a decorrer uma verificação da base de dados",
		ErrDatabaseCheckNotFound:       "Ainda não foi executada nenhuma verificação da base de dados",
		ErrDatabaseRepairFailed:        "Não foi possível concluir a reparação",
		ErrDatabaseRepairUnknown:       "Esse resultado não faz parte do relatório atual",

		MsgDatabaseTestPassed:       "O teste de ligação à base de dados foi bem-sucedido",
		MsgDatabaseMigrationStarted: "Migração de base de dados iniciada",
		MsgDatabaseConfigSaved:      "Configuração da base de dados guardada",
		MsgDatabaseMigrationDone:    "Migração de base de dados concluída",
		MsgDatabaseCheckCancelled:   "Verificação da base de dados cancelada",
	},
}

// init folds databaseMessages into the per-language maps T() reads. Package
// level variables are fully initialised before any init() runs, so the six
// maps in messages.go exist by the time this executes.
func init() {
	for lang, msgs := range databaseMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
