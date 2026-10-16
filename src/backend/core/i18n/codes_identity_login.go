// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// ErrIdentityDirectoryUnavailable answers a directory sign-in the directory itself
// could not serve. It is kept apart from invalid credentials because a user told
// their password is wrong during an outage retries into their own lockout.
const ErrIdentityDirectoryUnavailable MsgCode = "err.identity.directory_unavailable"

// identityLoginMessages holds the six translations of the directory sign-in codes.
var identityLoginMessages = map[Lang]map[MsgCode]string{
	EN: {ErrIdentityDirectoryUnavailable: "The directory could not be reached. Try again shortly or contact your administrator."},
	NL: {ErrIdentityDirectoryUnavailable: "De directory is niet bereikbaar. Probeer het zo opnieuw of neem contact op met je beheerder."},
	DE: {ErrIdentityDirectoryUnavailable: "Das Verzeichnis ist nicht erreichbar. Versuchen Sie es gleich erneut oder wenden Sie sich an Ihren Administrator."},
	ES: {ErrIdentityDirectoryUnavailable: "No se pudo contactar con el directorio. Inténtelo de nuevo en breve o contacte con su administrador."},
	FR: {ErrIdentityDirectoryUnavailable: "L'annuaire est injoignable. Réessayez dans un instant ou contactez votre administrateur."},
	PT: {ErrIdentityDirectoryUnavailable: "Não foi possível contactar o diretório. Tente novamente em breve ou contacte o seu administrador."},
}

// init folds identityLoginMessages into the per-language maps T() reads, as
// codes_debugshell.go does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range identityLoginMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
