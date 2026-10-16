// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Bundled i18n resources for the identity-generic-oidc plugin. The
// host's PluginFormOutlet passes its own t() shim through ctx.t
// (preferred), but identity-generic-oidc's ConfigForm / SetupGuide
// also call useTranslation('identity-generic-oidc') at module-eval
// time — without an initialised i18next instance react-i18next emits
// NO_I18NEXT_INSTANCE and falls back to the raw key.
//
// Resources are registered under the plugin-slug namespace
// ('identity-generic-oidc'), matching the namespace ConfigForm /
// SetupGuide request via useTranslation('identity-generic-oidc').
// Registering under the default 'translation' namespace would leave
// the lookup failing — see task #537 journal.
//
// Phase 3 ships all 6 locales (en/nl/de/es/fr/pt). NL/DE/ES/FR/PT
// mirror EN content as placeholders until translated.

import en from '../../i18n/en/common.json';
import nl from '../../i18n/nl/common.json';
import de from '../../i18n/de/common.json';
import es from '../../i18n/es/common.json';
import fr from '../../i18n/fr/common.json';
import pt from '../../i18n/pt/common.json';

type Locale = Record<string, Record<string, unknown>>;
type Resources = Record<string, Locale>;

export const genericOidcResources: Resources = {
  en: { 'identity-generic-oidc': en },
  nl: { 'identity-generic-oidc': nl },
  de: { 'identity-generic-oidc': de },
  es: { 'identity-generic-oidc': es },
  fr: { 'identity-generic-oidc': fr },
  pt: { 'identity-generic-oidc': pt },
};
