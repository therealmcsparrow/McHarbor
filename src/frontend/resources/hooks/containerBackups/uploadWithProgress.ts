// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from 'i18next';
import type { ApiResponse } from '@core/api/client';
import { apiErrorOf } from '@resources/utils/api-mutation';

export type UploadProgress = {
  loaded: number;
  total: number;
  /** "checking" once every byte is sent and the server is still reading the archive. */
  phase: 'uploading' | 'checking';
};

function acceptLanguage(): string {
  const stored = typeof window !== 'undefined' ? localStorage.getItem('mcharbor-language') : null;
  if (!stored) return 'en';
  try {
    return JSON.parse(stored)?.state?.language || 'en';
  } catch {
    return stored;
  }
}

/**
 * postFormWithProgress posts a multipart body over XMLHttpRequest because fetch exposes no
 * upload progress. It resolves with the envelope's data and throws the same ApiError
 * assertSuccess would for a failed envelope.
 */
export function postFormWithProgress<T>(
  url: string,
  form: FormData,
  onProgress?: (progress: UploadProgress) => void,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open('POST', url);
    request.withCredentials = true;
    request.setRequestHeader('Accept-Language', acceptLanguage());

    let sent = 0;
    request.upload.onprogress = (progressEvent) => {
      if (!onProgress || !progressEvent.lengthComputable) return;
      sent = progressEvent.total;
      onProgress({
        loaded: progressEvent.loaded,
        total: progressEvent.total,
        phase: progressEvent.loaded >= progressEvent.total ? 'checking' : 'uploading',
      });
    };
    request.upload.onload = () => {
      onProgress?.({ loaded: sent, total: sent, phase: 'checking' });
    };
    request.onerror = () => {
      reject(new Error(i18n.t('errors.network', { ns: 'common' })));
    };
    request.onabort = () => {
      reject(new Error(i18n.t('errors.network', { ns: 'common' })));
    };
    request.onload = () => {
      let envelope: ApiResponse<T>;
      try {
        envelope = JSON.parse(request.responseText) as ApiResponse<T>;
      } catch {
        reject(new Error(i18n.t('errors.nonJson', { ns: 'common', status: request.status })));
        return;
      }
      if (!envelope.success) {
        reject(apiErrorOf(envelope));
        return;
      }
      resolve(envelope.data as T);
    };
    request.send(form);
  });
}
