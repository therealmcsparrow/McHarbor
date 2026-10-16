// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { usePermission } from '@core/auth/usePermission';
import { useDocumentExporter } from '@resources/hooks/useRunbookExport';

/**
 * useRunbookDocumentFormats reports whether PDF and DOCX are reachable.
 *
 * The writers live in a plugin that declares a `document-export` capability,
 * so the host offers those two formats only while such a plugin is loaded and
 * the caller holds the key it names — a menu entry that always 404s is worse
 * than a menu with one item on it. JSON is the host's own route and is always
 * available.
 */
export function useRunbookDocumentFormats(): { available: boolean } {
  const exporter = useDocumentExporter();
  const mayGenerate = usePermission(exporter?.permission ?? []);
  return { available: Boolean(exporter) && mayGenerate };
}
