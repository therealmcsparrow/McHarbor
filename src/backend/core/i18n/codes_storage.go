// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// ErrStorageLocationProtected is returned when an operator tries to disable,
// delete or retype the built-in local storage location every backup falls back to.
const ErrStorageLocationProtected MsgCode = "err.storage_location.protected"

// storageMessages holds the six translations of the storage location codes.
var storageMessages = map[Lang]map[MsgCode]string{
	EN: {ErrStorageLocationProtected: "The built-in local storage location cannot be disabled, deleted or changed to another type"},
	NL: {ErrStorageLocationProtected: "De ingebouwde lokale opslaglocatie kan niet worden uitgeschakeld, verwijderd of naar een ander type worden gewijzigd"},
	DE: {ErrStorageLocationProtected: "Der integrierte lokale Speicherort kann nicht deaktiviert, gelöscht oder in einen anderen Typ geändert werden"},
	ES: {ErrStorageLocationProtected: "La ubicación de almacenamiento local integrada no se puede desactivar, eliminar ni cambiar a otro tipo"},
	FR: {ErrStorageLocationProtected: "L'emplacement de stockage local intégré ne peut pas être désactivé, supprimé ni changé de type"},
	PT: {ErrStorageLocationProtected: "O local de armazenamento local integrado não pode ser desativado, excluído nem alterado para outro tipo"},
}

// init folds storageMessages into the per-language maps T() reads, as
// codes_database.go does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range storageMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
