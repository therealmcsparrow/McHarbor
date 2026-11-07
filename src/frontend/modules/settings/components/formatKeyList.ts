// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** formatKeyList joins permission keys with the active language's own "and", so no English word is spliced into a translated sentence. */
export function formatKeyList(keys: readonly string[], language: string): string {
  return new Intl.ListFormat(language, { style: 'long', type: 'conjunction' }).format(keys);
}
