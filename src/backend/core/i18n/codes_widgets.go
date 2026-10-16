// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// The downloaded-widget codes. A refusal names why the bundle may not run, because
// an operator fixes an untrusted signature and a missing catalog entry differently.
const (
	ErrWidgetSignatureInvalid     MsgCode = "err.widget.signature_invalid"
	ErrWidgetChecksumMismatch     MsgCode = "err.widget.checksum_mismatch"
	ErrWidgetCatalogEntryNotFound MsgCode = "err.widget.catalog_entry_not_found"
	ErrWidgetDowngrade            MsgCode = "err.widget.downgrade"
)

// widgetMessages holds the six translations of the downloaded-widget codes.
var widgetMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrWidgetSignatureInvalid:     "The widget bundle is not signed by a trusted key, so it will not be installed or loaded",
		ErrWidgetChecksumMismatch:     "The widget bundle does not match the checksum its catalog entry declares",
		ErrWidgetCatalogEntryNotFound: "No widget with that key is in the widget catalog",
		ErrWidgetDowngrade:            "The widget catalog offers an older version than the one installed, so it was not installed",
	},
	NL: {
		ErrWidgetSignatureInvalid:     "De widgetbundel is niet ondertekend met een vertrouwde sleutel en wordt dus niet geïnstalleerd of geladen",
		ErrWidgetChecksumMismatch:     "De widgetbundel komt niet overeen met de checksum uit de catalogus",
		ErrWidgetCatalogEntryNotFound: "Er staat geen widget met die sleutel in de widgetcatalogus",
		ErrWidgetDowngrade:            "De widgetcatalogus biedt een oudere versie dan de geïnstalleerde aan, dus deze is niet geïnstalleerd",
	},
	DE: {
		ErrWidgetSignatureInvalid:     "Das Widget-Paket ist nicht mit einem vertrauenswürdigen Schlüssel signiert und wird daher weder installiert noch geladen",
		ErrWidgetChecksumMismatch:     "Das Widget-Paket stimmt nicht mit der Prüfsumme aus dem Katalog überein",
		ErrWidgetCatalogEntryNotFound: "Im Widget-Katalog gibt es kein Widget mit diesem Schlüssel",
		ErrWidgetDowngrade:            "Der Widget-Katalog bietet eine ältere Version als die installierte an, daher wurde sie nicht installiert",
	},
	ES: {
		ErrWidgetSignatureInvalid:     "El paquete del widget no está firmado con una clave de confianza, así que no se instalará ni se cargará",
		ErrWidgetChecksumMismatch:     "El paquete del widget no coincide con la suma de comprobación de su entrada de catálogo",
		ErrWidgetCatalogEntryNotFound: "No hay ningún widget con esa clave en el catálogo de widgets",
		ErrWidgetDowngrade:            "El catálogo de widgets ofrece una versión anterior a la instalada, así que no se ha instalado",
	},
	FR: {
		ErrWidgetSignatureInvalid:     "Le paquet du widget n'est pas signé par une clé de confiance ; il ne sera ni installé ni chargé",
		ErrWidgetChecksumMismatch:     "Le paquet du widget ne correspond pas à la somme de contrôle de son entrée de catalogue",
		ErrWidgetCatalogEntryNotFound: "Aucun widget avec cette clé dans le catalogue de widgets",
		ErrWidgetDowngrade:            "Le catalogue de widgets propose une version plus ancienne que celle installée ; elle n'a pas été installée",
	},
	PT: {
		ErrWidgetSignatureInvalid:     "O pacote do widget não está assinado por uma chave confiável, por isso não será instalado nem carregado",
		ErrWidgetChecksumMismatch:     "O pacote do widget não corresponde à soma de verificação da sua entrada no catálogo",
		ErrWidgetCatalogEntryNotFound: "Não existe nenhum widget com essa chave no catálogo de widgets",
		ErrWidgetDowngrade:            "O catálogo de widgets oferece uma versão mais antiga do que a instalada, por isso não foi instalada",
	},
}

// init folds widgetMessages into the per-language maps T() reads, as
// codes_debugshell.go does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range widgetMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
