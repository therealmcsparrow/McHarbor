// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import i18n from 'i18next';
import { api } from '@core/api/client';
import { apiClientPath, fillPath, usePluginCapabilities } from '@core/plugins/capabilities';
import type { PluginCapability } from '@core/plugins/loader';
import { assertSuccess } from '@resources/utils/api-mutation';
import { downloadFile, slugForFilename } from '@resources/utils/download';

/**
 * useExportRunbook saves one runbook as a portable JSON file.
 *
 * It goes through fetch rather than the API client because the response is a
 * file, not an envelope: the client would try to read it as
 * `{ success, data }` and throw on the raw document.
 */
export function useExportRunbook() {
  return useMutation({
    mutationFn: ({ id, name: itemName }: { id: string; name: string }) =>
      downloadFile(`/api/runbooks/${id}/export`, `${slugForFilename(itemName, 'runbook')}.mcharbor-runbook.json`),
    meta: {
      success: i18n.t('runbooks:list.exported'),
      error: i18n.t('runbooks:list.exportFailed'),
    },
  });
}

const DOCUMENT_FORMATS = ['pdf', 'docx'];

/**
 * useDocumentExporter is the loaded document-export provider that renders both runbook
 * formats and names the two routes to call, or undefined while none is loaded.
 */
export function useDocumentExporter(): PluginCapability | undefined {
  return usePluginCapabilities('document-export').find(
    (c) => c.generatePath && c.exportPath && DOCUMENT_FORMATS.every((format) => c.formats?.includes(format)),
  );
}

/**
 * useExportRunbookDocument saves one runbook as a PDF or a DOCX.
 *
 * The host has no document writer; the reporting plugin does, and it already
 * knows how to narrow a report to one object. So this generates the `runbooks`
 * report scoped to this runbook and then downloads that report in the format
 * asked for — two calls, because generating and rendering are two steps there,
 * and the generated document deliberately stays in the report library.
 */
export function useExportRunbookDocument() {
  const exporter = useDocumentExporter();
  return useMutation({
    mutationFn: async ({ id, name: itemName, format }: { id: string; name: string; format: 'pdf' | 'docx' }) => {
      if (!exporter?.generatePath || !exporter.exportPath) {
        throw new Error(i18n.t('runbooks:list.exportNoProvider'));
      }
      const report = await api
        .post<{ id: string }>(apiClientPath(exporter.generatePath), {
          type: 'runbooks',
          subject_kind: 'runbook',
          subject_id: id,
          subject_name: itemName,
        })
        .then(assertSuccess);
      await downloadFile(
        `${fillPath(exporter.exportPath, { id: report.id })}?format=${format}`,
        `${slugForFilename(itemName, 'runbook')}.${format}`,
      );
    },
    meta: {
      success: i18n.t('runbooks:list.exported'),
      error: i18n.t('runbooks:list.exportFailed'),
    },
  });
}
