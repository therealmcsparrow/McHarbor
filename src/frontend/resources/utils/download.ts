// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * currentLanguage reads the stored UI language for an `Accept-Language` header.
 *
 * A file download goes through `fetch` rather than the API client, so it does not
 * inherit the header the client adds — and a server-rendered filename or error
 * would come back in the wrong language without it.
 */
export function currentLanguage(): string {
  const stored = localStorage.getItem('mcharbor-language');
  if (!stored) return 'en';
  try {
    return JSON.parse(stored)?.state?.language || 'en';
  } catch {
    // An older build stored the bare code rather than a Zustand envelope.
    return stored;
  }
}

/** filenameFromDisposition prefers the server's filename over the caller's guess. */
export function filenameFromDisposition(disposition: string | null, fallback: string): string {
  const match = disposition?.match(/filename="([^"]+)"/);
  return match?.[1] || fallback;
}

/** slugForFilename folds a display name into something every filesystem accepts. */
export function slugForFilename(itemName: string, fallback: string): string {
  const slug = itemName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');
  return slug || fallback;
}

/**
 * saveBlob hands the browser a file to save.
 *
 * The object URL is revoked immediately after the click: the browser has already
 * taken its own reference by then, and leaving it alive holds the whole blob in
 * memory for the life of the document.
 */
export function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

/**
 * downloadFile fetches path and saves what comes back, naming the file from the
 * server's Content-Disposition when it sent one.
 *
 * It throws on a non-2xx so a TanStack mutation reports it through the usual
 * toast, rather than silently saving an error page as a .json file.
 */
export async function downloadFile(path: string, fallbackFilename: string): Promise<void> {
  const response = await fetch(path, {
    credentials: 'include',
    headers: { 'Accept-Language': currentLanguage() },
  });
  if (!response.ok) {
    throw new Error(`download failed with status ${response.status}`);
  }
  const blob = await response.blob();
  saveBlob(blob, filenameFromDisposition(response.headers.get('content-disposition'), fallbackFilename));
}
