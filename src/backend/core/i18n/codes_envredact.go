// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// ErrStackComposeMaskedValue is returned when a saved compose file still carries the masked
// marker somewhere McHarbor cannot put the stored value back, so saving would write bullets.
const ErrStackComposeMaskedValue MsgCode = "err.stack.compose_masked_value"

// ErrMaskedValueMoved is returned when a compose save or a recreate keeps a value nobody may
// reveal while changing what runs with it, which would move a value the editor cannot read.
const ErrMaskedValueMoved MsgCode = "err.envredact.masked_value_moved"

// ErrProtectedPathForbidden is returned when the file browser reaches McHarbor's database,
// TLS keys or secret files without system.manage.
const ErrProtectedPathForbidden MsgCode = "err.container.protected_path_forbidden"

// ErrKeyFilePathForbidden is returned for a McHarbor key file, which no permission opens
// through the file browser.
const ErrKeyFilePathForbidden MsgCode = "err.container.key_file_forbidden"

// envRedactMessages holds the six translations of the redaction and protected-path codes.
var envRedactMessages = map[Lang]map[MsgCode]string{
	EN: {ErrStackComposeMaskedValue: "The compose file still contains a masked value (••••••••) that McHarbor cannot put back — for a new or renamed variable or service, or outside an environment entry. Type the real value there, or remove the entry", ErrMaskedValueMoved: "A masked value (••••••••) was kept for a service whose image, command, entrypoint or labels also changed. Type the real value, or save the two changes separately", ErrProtectedPathForbidden: "This path holds McHarbor's database, keys or secrets and requires the system.manage permission", ErrKeyFilePathForbidden: "McHarbor key files cannot be opened through the file browser"},
	NL: {ErrStackComposeMaskedValue: "Het compose-bestand bevat nog een gemaskeerde waarde (••••••••) die McHarbor niet kan terugzetten — bij een nieuwe of hernoemde variabele of service, of buiten een environment-regel. Typ daar de echte waarde, of verwijder de regel", ErrMaskedValueMoved: "Een gemaskeerde waarde (••••••••) is behouden voor een service waarvan ook de image, het command, het entrypoint of de labels zijn gewijzigd. Typ de echte waarde, of sla de twee wijzigingen apart op", ErrProtectedPathForbidden: "Dit pad bevat de database, sleutels of secrets van McHarbor en vereist de rechten system.manage", ErrKeyFilePathForbidden: "Sleutelbestanden van McHarbor kunnen niet via de bestandsbrowser worden geopend"},
	DE: {ErrStackComposeMaskedValue: "Die Compose-Datei enthält noch einen maskierten Wert (••••••••), den McHarbor nicht zurücksetzen kann — bei einer neuen oder umbenannten Variable oder einem Dienst, oder außerhalb eines environment-Eintrags. Geben Sie dort den echten Wert ein oder entfernen Sie den Eintrag", ErrMaskedValueMoved: "Ein maskierter Wert (••••••••) wurde für einen Dienst beibehalten, dessen Image, Befehl, Entrypoint oder Labels sich ebenfalls geändert haben. Geben Sie den echten Wert ein oder speichern Sie die beiden Änderungen getrennt", ErrProtectedPathForbidden: "Dieser Pfad enthält die Datenbank, Schlüssel oder Secrets von McHarbor und erfordert die Berechtigung system.manage", ErrKeyFilePathForbidden: "Schlüsseldateien von McHarbor können nicht über den Dateibrowser geöffnet werden"},
	ES: {ErrStackComposeMaskedValue: "El archivo compose aún contiene un valor enmascarado (••••••••) que McHarbor no puede restaurar: en una variable o servicio nuevo o renombrado, o fuera de una entrada environment. Escriba allí el valor real o elimine la entrada", ErrMaskedValueMoved: "Se conservó un valor enmascarado (••••••••) para un servicio cuya imagen, comando, entrypoint o etiquetas también cambiaron. Escriba el valor real o guarde los dos cambios por separado", ErrProtectedPathForbidden: "Esta ruta contiene la base de datos, las claves o los secretos de McHarbor y requiere el permiso system.manage", ErrKeyFilePathForbidden: "Los archivos de claves de McHarbor no se pueden abrir desde el explorador de archivos"},
	FR: {ErrStackComposeMaskedValue: "Le fichier compose contient encore une valeur masquée (••••••••) que McHarbor ne peut pas restaurer : variable ou service nouveau ou renommé, ou hors d'une entrée environment. Saisissez la vraie valeur ou supprimez l'entrée", ErrMaskedValueMoved: "Une valeur masquée (••••••••) a été conservée pour un service dont l'image, la commande, l'entrypoint ou les labels ont aussi changé. Saisissez la vraie valeur ou enregistrez les deux modifications séparément", ErrProtectedPathForbidden: "Ce chemin contient la base de données, les clés ou les secrets de McHarbor et nécessite la permission system.manage", ErrKeyFilePathForbidden: "Les fichiers de clés de McHarbor ne peuvent pas être ouverts depuis le navigateur de fichiers"},
	PT: {ErrStackComposeMaskedValue: "O arquivo compose ainda contém um valor mascarado (••••••••) que o McHarbor não consegue restaurar — numa variável ou serviço novo ou renomeado, ou fora de uma entrada environment. Digite o valor real ou remova a entrada", ErrMaskedValueMoved: "Um valor mascarado (••••••••) foi mantido para um serviço cuja imagem, comando, entrypoint ou labels também mudaram. Digite o valor real ou salve as duas alterações separadamente", ErrProtectedPathForbidden: "Este caminho contém o banco de dados, as chaves ou os segredos do McHarbor e requer a permissão system.manage", ErrKeyFilePathForbidden: "Os arquivos de chave do McHarbor não podem ser abertos pelo navegador de arquivos"},
}

// init folds envRedactMessages into the per-language maps T() reads, as codes_storage.go
// does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range envRedactMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
