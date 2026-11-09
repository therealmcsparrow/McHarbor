// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// Codes for the container backup temp location and its cleanup.
const (
	// ErrBackupTempPathSensitive refuses a temp location inside a system folder, the data directory or the local backup directory, or one that contains them.
	ErrBackupTempPathSensitive MsgCode = "err.backup_temp.path_sensitive"
	// ErrBackupTempWorldWritable refuses a temp location any local user can write to, because backup archives pass through it.
	ErrBackupTempWorldWritable MsgCode = "err.backup_temp.world_writable"
	// ErrBackupTempNotWritable reports that McHarbor could not create and delete a test file in the chosen folder.
	ErrBackupTempNotWritable MsgCode = "err.backup_temp.not_writable"
	// ErrBackupTempEnvOverride refuses a saved location while MCHARBOR_BACKUP_TEMP_DIR decides the folder.
	ErrBackupTempEnvOverride MsgCode = "err.backup_temp.env_override"
	// ErrBackupTempSaveFailed reports that the new temp location could not be stored.
	ErrBackupTempSaveFailed MsgCode = "err.backup_temp.save_failed"
	// ErrBackupTempStatusFailed reports that the temp location status could not be read.
	ErrBackupTempStatusFailed MsgCode = "err.backup_temp.status_failed"
	// ErrBackupTempCleanupFailed reports that the manual temp cleanup could not finish.
	ErrBackupTempCleanupFailed MsgCode = "err.backup_temp.cleanup_failed"
)

// backupTempMessages holds the six translations of the backup temp codes; the environment variable name is quoted verbatim.
var backupTempMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrBackupTempPathSensitive: "This folder cannot be used for backup temp files: it is a system folder, the McHarbor data directory or backup directory, or a folder that contains them",
		ErrBackupTempWorldWritable: "This folder can be written to by every user on the host. Choose a folder only the McHarbor service account can write to",
		ErrBackupTempNotWritable:   "McHarbor could not create a test file in this folder. Check that it exists, that the service account owns it and that the service has write access to it",
		ErrBackupTempEnvOverride:   "The temp folder is set by the MCHARBOR_BACKUP_TEMP_DIR environment variable. Remove the variable to change it here",
		ErrBackupTempSaveFailed:    "The backup temp folder could not be saved",
		ErrBackupTempStatusFailed:  "The backup temp folder status could not be read",
		ErrBackupTempCleanupFailed: "The backup temp folder could not be cleaned up",
	},
	NL: { //nolint:gosec // translated error text mentioning a user account, not a credential
		ErrBackupTempPathSensitive: "Deze map kan niet voor tijdelijke backupbestanden worden gebruikt: het is een systeemmap, de datamap of backupmap van McHarbor, of een map die deze bevat",
		ErrBackupTempWorldWritable: "In deze map kan elke gebruiker op de host schrijven. Kies een map waarin alleen het serviceaccount van McHarbor kan schrijven",
		ErrBackupTempNotWritable:   "McHarbor kon geen testbestand in deze map aanmaken. Controleer of de map bestaat, of het serviceaccount de eigenaar is en of de service er schrijfrechten op heeft",
		ErrBackupTempEnvOverride:   "De tijdelijke map wordt bepaald door de omgevingsvariabele MCHARBOR_BACKUP_TEMP_DIR. Verwijder de variabele om dit hier te wijzigen",
		ErrBackupTempSaveFailed:    "De tijdelijke backupmap kon niet worden opgeslagen",
		ErrBackupTempStatusFailed:  "De status van de tijdelijke backupmap kon niet worden gelezen",
		ErrBackupTempCleanupFailed: "De tijdelijke backupmap kon niet worden opgeruimd",
	},
	DE: { //nolint:gosec // translated error text mentioning a user account, not a credential
		ErrBackupTempPathSensitive: "Dieser Ordner kann nicht für temporäre Backup-Dateien verwendet werden: Er ist ein Systemordner, das Datenverzeichnis oder Backup-Verzeichnis von McHarbor oder ein Ordner, der diese enthält",
		ErrBackupTempWorldWritable: "In diesen Ordner kann jeder Benutzer auf dem Host schreiben. Wählen Sie einen Ordner, in den nur das McHarbor-Dienstkonto schreiben darf",
		ErrBackupTempNotWritable:   "McHarbor konnte in diesem Ordner keine Testdatei anlegen. Prüfen Sie, ob er existiert, ob das Dienstkonto ihn besitzt und ob der Dienst Schreibrechte darauf hat",
		ErrBackupTempEnvOverride:   "Der temporäre Ordner wird durch die Umgebungsvariable MCHARBOR_BACKUP_TEMP_DIR festgelegt. Entfernen Sie die Variable, um ihn hier zu ändern",
		ErrBackupTempSaveFailed:    "Der temporäre Backup-Ordner konnte nicht gespeichert werden",
		ErrBackupTempStatusFailed:  "Der Status des temporären Backup-Ordners konnte nicht gelesen werden",
		ErrBackupTempCleanupFailed: "Der temporäre Backup-Ordner konnte nicht bereinigt werden",
	},
	ES: { //nolint:gosec // translated error text mentioning a user account, not a credential
		ErrBackupTempPathSensitive: "Esta carpeta no se puede usar para archivos temporales de copia de seguridad: es una carpeta del sistema, el directorio de datos o de copias de McHarbor, o una carpeta que los contiene",
		ErrBackupTempWorldWritable: "Cualquier usuario del host puede escribir en esta carpeta. Elija una carpeta en la que solo pueda escribir la cuenta de servicio de McHarbor",
		ErrBackupTempNotWritable:   "McHarbor no pudo crear un archivo de prueba en esta carpeta. Compruebe que existe, que la cuenta de servicio es su propietaria y que el servicio tiene permiso de escritura",
		ErrBackupTempEnvOverride:   "La carpeta temporal la define la variable de entorno MCHARBOR_BACKUP_TEMP_DIR. Elimine la variable para cambiarla aquí",
		ErrBackupTempSaveFailed:    "No se pudo guardar la carpeta temporal de copias de seguridad",
		ErrBackupTempStatusFailed:  "No se pudo leer el estado de la carpeta temporal de copias de seguridad",
		ErrBackupTempCleanupFailed: "No se pudo limpiar la carpeta temporal de copias de seguridad",
	},
	FR: { //nolint:gosec // translated error text mentioning a user account, not a credential
		ErrBackupTempPathSensitive: "Ce dossier ne peut pas servir aux fichiers temporaires de sauvegarde : c'est un dossier système, le répertoire de données ou de sauvegarde de McHarbor, ou un dossier qui les contient",
		ErrBackupTempWorldWritable: "Tous les utilisateurs de l'hôte peuvent écrire dans ce dossier. Choisissez un dossier où seul le compte de service de McHarbor peut écrire",
		ErrBackupTempNotWritable:   "McHarbor n'a pas pu créer de fichier de test dans ce dossier. Vérifiez qu'il existe, que le compte de service en est propriétaire et que le service peut y écrire",
		ErrBackupTempEnvOverride:   "Le dossier temporaire est défini par la variable d'environnement MCHARBOR_BACKUP_TEMP_DIR. Supprimez la variable pour le modifier ici",
		ErrBackupTempSaveFailed:    "Le dossier temporaire de sauvegarde n'a pas pu être enregistré",
		ErrBackupTempStatusFailed:  "L'état du dossier temporaire de sauvegarde n'a pas pu être lu",
		ErrBackupTempCleanupFailed: "Le dossier temporaire de sauvegarde n'a pas pu être nettoyé",
	},
	PT: { //nolint:gosec // translated error text mentioning a user account, not a credential
		ErrBackupTempPathSensitive: "Esta pasta não pode ser usada para arquivos temporários de backup: é uma pasta do sistema, o diretório de dados ou de backup do McHarbor, ou uma pasta que os contém",
		ErrBackupTempWorldWritable: "Qualquer usuário do host pode gravar nesta pasta. Escolha uma pasta em que apenas a conta de serviço do McHarbor possa gravar",
		ErrBackupTempNotWritable:   "O McHarbor não conseguiu criar um arquivo de teste nesta pasta. Verifique se ela existe, se a conta de serviço é a proprietária e se o serviço tem permissão de gravação",
		ErrBackupTempEnvOverride:   "A pasta temporária é definida pela variável de ambiente MCHARBOR_BACKUP_TEMP_DIR. Remova a variável para alterá-la aqui",
		ErrBackupTempSaveFailed:    "Não foi possível salvar a pasta temporária de backup",
		ErrBackupTempStatusFailed:  "Não foi possível ler o status da pasta temporária de backup",
		ErrBackupTempCleanupFailed: "Não foi possível limpar a pasta temporária de backup",
	},
}

// init folds backupTempMessages into the per-language maps T() reads, as codes_hostself.go does.
func init() {
	for lang, msgs := range backupTempMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
