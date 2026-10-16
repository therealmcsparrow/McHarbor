// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Turning an operator's chosen mark into something the exporters can embed.
//
// The PDF and Word writers in this plugin are hand-rolled and embed raster
// images only — neither can rasterise vector artwork, and pulling an SVG
// rasteriser into the plugin's Go module to fix that would be a heavy
// dependency for a logo on a cover page. The browser already has one, so an
// uploaded SVG is drawn onto a canvas here and travels to the backend as PNG.
// The operator still picks either format; the backend only ever stores one.

/** Decoded-size ceiling, matching `maxMarkBytes` in the Go store. */
export const maxMarkBytes = 2 * 1024 * 1024;

/** Per-side pixel ceiling, matching `maxMarkPx` in the Go store. */
export const maxMarkPx = 4000;

export type MarkError = 'badImage' | 'tooLarge';

/** Thrown for anything the settings page should explain rather than swallow. */
export class MarkImageError extends Error {
  readonly reason: MarkError;

  constructor(reason: MarkError) {
    super(reason);
    this.name = 'MarkImageError';
    this.reason = reason;
  }
}

/**
 * Read one picked file and return base64 PNG with no `data:` prefix.
 *
 * `maxSide` is the longest edge the rasterised SVG is drawn at. A cover
 * wordmark is printed a couple of inches wide, so ~1600px is already past
 * the point a reader could tell — and it keeps the settings blob small
 * enough to sit in a KV row that is read on every catalog request.
 */
export async function fileToMarkPNG(file: File, maxSide: number): Promise<string> {
  const isSvg = file.type === 'image/svg+xml' || /\.svg$/i.test(file.name);
  const dataUrl = isSvg ? await svgToPNGDataURL(await file.text(), maxSide) : await rasterToPNGDataURL(file);
  const base64 = dataUrl.slice(dataUrl.indexOf(',') + 1);
  if (decodedBytes(base64) > maxMarkBytes) throw new MarkImageError('tooLarge');
  return base64;
}

/** Base64 carries 3 bytes per 4 characters, less whatever the padding claims. */
function decodedBytes(base64: string): number {
  const padding = base64.endsWith('==') ? 2 : base64.endsWith('=') ? 1 : 0;
  return Math.floor((base64.length * 3) / 4) - padding;
}

async function rasterToPNGDataURL(file: File): Promise<string> {
  if (file.size > maxMarkBytes) throw new MarkImageError('tooLarge');
  const dataUrl = await readAsDataURL(file);
  const img = await loadImage(dataUrl);
  if (img.naturalWidth > maxMarkPx || img.naturalHeight > maxMarkPx) {
    throw new MarkImageError('tooLarge');
  }
  if (dataUrl.startsWith('data:image/png')) return dataUrl;
  // A JPEG or WebP dropped on the picker still decodes; re-encode it as PNG
  // so the backend has exactly one format to reason about.
  return draw(img, img.naturalWidth, img.naturalHeight);
}

async function svgToPNGDataURL(source: string, maxSide: number): Promise<string> {
  const { width, height } = intrinsicSize(source);
  const scale = maxSide / Math.max(width, height);
  const w = Math.min(maxMarkPx, Math.max(1, Math.round(width * scale)));
  const h = Math.min(maxMarkPx, Math.max(1, Math.round(height * scale)));

  // These marks declare `width="100%" height="100%"`, which gives an
  // <img> nothing to size itself from — it would land on the 300x150
  // default and come out blurred. Pinning the attributes to the target
  // pixel size is what makes the raster crisp.
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
  if (!ctx) throw new MarkImageError('badImage');
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
  if (!svg || doc.querySelector('parsererror')) throw new MarkImageError('badImage');
  return svg as SVGSVGElement;
}

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
    reader.onerror = () => reject(new MarkImageError('badImage'));
    reader.readAsDataURL(file);
  });
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new MarkImageError('badImage'));
    img.src = src;
  });
}
