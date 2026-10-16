// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Turning an uploaded brand mark into something a document exporter can embed.
//
// McHarbor's own UI renders whatever was uploaded — SVG included, which is what
// a logo should be. Consumers that write documents rather than pages cannot:
// the reporting plugin's PDF and DOCX writers are hand-rolled and embed raster
// images only, and its `mark-image.ts` explains why pulling an SVG rasteriser
// into a Go module is the wrong trade for a cover logo.
//
// The browser already has a rasteriser. So the light marks are rasterised here,
// once, at upload time, and the rendition is stored next to the original. A
// consumer gets a PNG without either side growing a dependency, and re-uploading
// the logo regenerates it, so the two cannot drift apart.

/** Longest edge the rendition is drawn at. A cover wordmark prints a couple of
 *  inches wide, so past ~1600px no reader can tell, and the base64 stays small
 *  enough to sit in a settings row that is read on every page load. */
export const MARK_RASTER_MAX_SIDE = 1600;

/** Per-side pixel ceiling for a raster upload, mirroring the plugin's own. */
const MAX_RASTER_PX = 4000;

/**
 * Rasterise a `data:` URI to base64 PNG **without** the `data:` prefix, which
 * is the form document exporters store.
 *
 * Returns '' rather than throwing: the rendition is a convenience for other
 * consumers, and failing to produce one must never block an operator from
 * setting the logo they can plainly see in the preview.
 */
export async function rasterizeMark(dataUri: string, maxSide = MARK_RASTER_MAX_SIDE): Promise<string> {
  if (!dataUri) return '';
  try {
    const isSvg = dataUri.startsWith('data:image/svg+xml');
    const pngDataUrl = isSvg ? await svgToPNG(decodeDataUri(dataUri), maxSide) : await rasterToPNG(dataUri);
    return pngDataUrl.slice(pngDataUrl.indexOf(',') + 1);
  } catch {
    return '';
  }
}

/** The payload of a base64 `data:` URI, as text. */
function decodeDataUri(dataUri: string): string {
  const base64 = dataUri.slice(dataUri.indexOf(',') + 1);
  const binary = atob(base64);
  const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

async function rasterToPNG(dataUri: string): Promise<string> {
  const img = await loadImage(dataUri);
  if (img.naturalWidth > MAX_RASTER_PX || img.naturalHeight > MAX_RASTER_PX) {
    throw new Error('mark exceeds the pixel ceiling');
  }
  // Already PNG — re-encoding would only cost quality and time.
  if (dataUri.startsWith('data:image/png')) return dataUri;
  return draw(img, img.naturalWidth, img.naturalHeight);
}

async function svgToPNG(source: string, maxSide: number): Promise<string> {
  const { width, height } = intrinsicSize(source);
  const scale = maxSide / Math.max(width, height);
  const w = Math.min(MAX_RASTER_PX, Math.max(1, Math.round(width * scale)));
  const h = Math.min(MAX_RASTER_PX, Math.max(1, Math.round(height * scale)));

  // McHarbor's own marks declare `width="100%" height="100%"`, which gives an
  // <img> nothing to size itself from — it would land on the 300x150 default
  // and come out blurred. Pinning the attributes is what makes it crisp.
  const url = URL.createObjectURL(new Blob([withExplicitSize(source, w, h)], { type: 'image/svg+xml;charset=utf-8' }));
  try {
    return draw(await loadImage(url), w, h);
  } finally {
    URL.revokeObjectURL(url);
  }
}

function draw(img: HTMLImageElement, w: number, h: number): string {
  const canvas = document.createElement('canvas');
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('no 2d context');
  ctx.drawImage(img, 0, 0, w, h);
  return canvas.toDataURL('image/png');
}

/** viewBox first, explicit width/height second, a square as the last resort. */
function intrinsicSize(source: string): { width: number; height: number } {
  const svg = parseSVG(source);
  const box = svg.getAttribute('viewBox');
  if (box) {
    const parts = box
      .trim()
      .split(/[\s,]+/)
      .map(Number);
    const width = parts[2] ?? 0;
    const height = parts[3] ?? 0;
    if (parts.length === 4 && width > 0 && height > 0) return { width, height };
  }
  const width = Number.parseFloat(svg.getAttribute('width') ?? '');
  const height = Number.parseFloat(svg.getAttribute('height') ?? '');
  if (width > 0 && height > 0) return { width, height };
  return { width: 512, height: 512 };
}

function withExplicitSize(source: string, w: number, h: number): string {
  const svg = parseSVG(source);
  svg.setAttribute('width', String(w));
  svg.setAttribute('height', String(h));
  return new XMLSerializer().serializeToString(svg);
}

function parseSVG(source: string): SVGSVGElement {
  const doc = new DOMParser().parseFromString(source, 'image/svg+xml');
  const svg = doc.querySelector('svg');
  if (!svg || doc.querySelector('parsererror')) throw new Error('unparseable svg');
  return svg as SVGSVGElement;
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error('image failed to decode'));
    img.src = src;
  });
}
