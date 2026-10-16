// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Bundled i18n resources for the identity-entra-id plugin. The host's
// PluginFormOutlet passes its own t() shim through ctx.t (preferred),
// but identity-entra-id's ConfigForm / SetupGuide also call
// useTranslation('identity-entra-id') at module-eval time — without
// an initialised i18next instance react-i18next emits
// NO_I18NEXT_INSTANCE and falls back to the raw key.
//
// Resources are registered under the plugin-slug namespace
// ('identity-entra-id'), matching the namespace ConfigForm /
// SetupGuide request via useTranslation('identity-entra-id').
// Registering under the default 'translation' namespace would leave
// the lookup failing — see task #537 journal.
//
// Phase 3 ships all 6 locales (en/nl/de/es/fr/pt). NL/DE/ES/FR/PT
// mirror EN content as placeholders until translated — same pattern
// as comm-signal's nl/de files.

import en from '../../i18n/en/common.json';
import nl from '../../i18n/nl/common.json';
import de from '../../i18n/de/common.json';
import es from '../../i18n/es/common.json';
import fr from '../../i18n/fr/common.json';
import pt from '../../i18n/pt/common.json';

type Locale = Record<string, Record<string, unknown>>;
type Resources = Record<string, Locale>;

export const identityEntraIdResources: Resources = {
  en: { 'identity-entra-id': en },
  nl: { 'identity-entra-id': nl },
  de: { 'identity-entra-id': de },
  es: { 'identity-entra-id': es },
  fr: { 'identity-entra-id': fr },
  pt: { 'identity-entra-id': pt },
};
