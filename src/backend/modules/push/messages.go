// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package push

import (
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/pushmsg"
)

// Message keys a Notification can carry instead of fixed text. The catalog
// lives here rather than in core/i18n because every plugin links core/i18n,
// and editing it would force every installed plugin to be rebuilt.
const (
	MsgAlertTriggered     = pushmsg.MsgAlertTriggered
	MsgAlertResolved      = pushmsg.MsgAlertResolved
	MsgElevationRequested = pushmsg.MsgElevationRequested
	MsgTest               = pushmsg.MsgTest
)

// message is one notification's title and body in one language; {name}-style
// placeholders are filled from Notification.Args.
type message struct{ title, body string }

var catalog = map[string]map[string]message{
	"en": {
		MsgAlertTriggered:     {"Alert triggered: {name}", ""},
		MsgAlertResolved:      {"Alert resolved: {name}", ""},
		MsgElevationRequested: {"Elevation requested", "{user} asks for {permission} for {minutes} minutes"},
		MsgTest:               {"McHarbor test notification", "Push notifications reach this device."},
	},
	"nl": {
		MsgAlertTriggered:     {"Waarschuwing actief: {name}", ""},
		MsgAlertResolved:      {"Waarschuwing opgelost: {name}", ""},
		MsgElevationRequested: {"Verhoging aangevraagd", "{user} vraagt {permission} voor {minutes} minuten"},
		MsgTest:               {"McHarbor-testmelding", "Pushmeldingen bereiken dit apparaat."},
	},
	"de": {
		MsgAlertTriggered:     {"Alarm ausgelöst: {name}", ""},
		MsgAlertResolved:      {"Alarm behoben: {name}", ""},
		MsgElevationRequested: {"Rechteerweiterung angefragt", "{user} bittet um {permission} für {minutes} Minuten"},
		MsgTest:               {"McHarbor-Testbenachrichtigung", "Push-Benachrichtigungen erreichen dieses Gerät."},
	},
	"es": {
		MsgAlertTriggered:     {"Alerta activada: {name}", ""},
		MsgAlertResolved:      {"Alerta resuelta: {name}", ""},
		MsgElevationRequested: {"Elevación solicitada", "{user} solicita {permission} durante {minutes} minutos"},
		MsgTest:               {"Notificación de prueba de McHarbor", "Las notificaciones push llegan a este dispositivo."},
	},
	"fr": {
		MsgAlertTriggered:     {"Alerte déclenchée : {name}", ""},
		MsgAlertResolved:      {"Alerte résolue : {name}", ""},
		MsgElevationRequested: {"Élévation demandée", "{user} demande {permission} pendant {minutes} minutes"},
		MsgTest:               {"Notification de test McHarbor", "Les notifications push arrivent sur cet appareil."},
	},
	"pt": {
		MsgAlertTriggered:     {"Alerta disparado: {name}", ""},
		MsgAlertResolved:      {"Alerta resolvido: {name}", ""},
		MsgElevationRequested: {"Elevação solicitada", "{user} pede {permission} por {minutes} minutos"},
		MsgTest:               {"Notificação de teste do McHarbor", "As notificações push chegam a este dispositivo."},
	},
}

// supportedLanguage reduces a stored preference ("nl", "nl-BE", "NL") to a
// catalog language, or "" when there is none.
func supportedLanguage(raw string) string {
	lang := strings.ToLower(strings.TrimSpace(raw))
	if len(lang) > 2 {
		lang = lang[:2]
	}
	if _, ok := catalog[lang]; ok {
		return lang
	}
	return ""
}

// localize renders n for lang. A notification without a key, or a key the
// catalog lacks, keeps its own title and body; an empty catalog body keeps the
// notification's body, which is text the operator wrote.
func localize(n Notification, lang string) Notification {
	if n.Key == "" {
		return n
	}
	msgs, ok := catalog[lang]
	if !ok {
		msgs = catalog["en"]
	}
	m, ok := msgs[n.Key]
	if !ok {
		return n
	}
	out := n
	out.Title = fill(m.title, n.Args)
	if m.body != "" {
		out.Body = fill(m.body, n.Args)
	}
	return out
}

func fill(tmpl string, args map[string]string) string {
	for k, v := range args {
		tmpl = strings.ReplaceAll(tmpl, "{"+k+"}", v)
	}
	return tmpl
}
