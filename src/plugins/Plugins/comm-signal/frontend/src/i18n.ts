// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Bundled i18n resources for the comm-signal plugin. The host's
// PluginFormOutlet passes its own t() shim through ctx.t (preferred),
// but comm-signal's ConfigForm / FieldRenderer also call
// useTranslation() at module-eval time — without an initialised
// i18next instance react-i18next emits NO_I18NEXT_INSTANCE and falls
// back to the raw key. We import the locale JSON files and register
// them so the hook has resources to draw from.

import en from '../../i18n/en/common.json';
import nl from '../../i18n/nl/common.json';
import de from '../../i18n/de/common.json';

type Locale = { translation: Record<string, unknown> };
type Resources = Record<string, Locale>;

export const commSignalResources: Resources = {
  en: { translation: en },
  nl: { translation: nl },
  de: { translation: de },
};
