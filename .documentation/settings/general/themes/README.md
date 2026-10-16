# Customization

**Settings → General → Customization** · `?tab=themes`

Theme variant, accent colour, fonts, motion, the application logo, and per-variable
colour overrides.

## Permissions

| Key | What it unlocks |
|---|---|
| _readable in Settings_ | See the section |
| `settings.manage` | Change the defaults or reset overrides |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- The variant, accent, font size and motion every account starts from.
- Whether individual users may depart from those defaults.
- The application logo shown to everyone, signed in or not.
- Per-variable OKLCH colour overrides for the active variant.

## Application logo

**Application logo** replaces the McHarbor mark with your own. There are four slots:
two marks, each with a light and a dark version:

| Mark | Where it appears | Shape |
|---|---|---|
| **Wordmark** | Expanded sidebar, login card | Wide |
| **Icon** | Collapsed sidebar rail (56px) | Square |

The app renders two marks because the collapsed rail is 56px wide and cannot fit a
lockup. It offers two variants because a mark drawn for a white page frequently
disappears on the dark rail.

**The dark slot is an override, not a requirement.** What actually renders is:

| Active theme | Resolves to |
|---|---|
| Light | your light upload → the McHarbor mark |
| Dark | your dark upload → your light upload → the McHarbor mark |

So one usable logo is uploaded once, into the light slot, and serves both themes. Fill
the dark slot only when that single image does not survive the flip. The shipped
McHarbor mark is the default for **both** variants.

Every slot is independent. Upload only the wordmark and the collapsed rail keeps the
shipped icon; **Use default** on any slot restores that one surface alone.

- **Formats**: SVG, PNG, JPEG, WebP, GIF. SVG is the right answer for a logo because it stays
  sharp on the rail and on a HiDPI login screen.
- **Size**: 512 KB per image. A logo that needs more than that is a photograph.
- **Scope**: app-wide, and unlike the colour variables there is no personal override;
  an install has one identity. The login page is included, which means the logo is
  visible **before anyone signs in**. Do not upload a mark you would not put on a public
  page.
- **Preview**: each slot previews on its own ground (the light slots on a light
  backdrop, the dark slots on a dark one) regardless of the theme you happen to be in,
  so both are honest at once. A dark slot left empty previews the light upload, which is
  exactly what dark mode would render.

The images are stored in the database, not on disk, so they survive a container rebuild
and are included in a backup.

A PNG rendition of each **light** mark is generated in your browser when you upload it,
because consumers that write documents rather than pages embed raster images only. The
[Reporting](../../../reporting/configuration.md) add-on reads it, which is
how a rebranded install produces rebranded reports without uploading the same logo twice.
A logo set before this existed has no rendition; re-upload it once to generate one.

## Colour variables

Beneath the variant and accent sits the full palette: "each variable is an OKLCH colour.
Overrides apply to the active variant only and are stored with your account."

Two things follow from that sentence:

- **Per variant.** Overriding a colour in Dark leaves Light untouched. A theme that looks
  right in one variant can be unreadable in the other, so check both.
- **Per account.** Variable overrides are personal, not an app-wide rebrand. For that, use
  a theme pack.

OKLCH is perceptual: changing lightness moves the colour the amount your eye expects, which
is what keeps a hand-edited palette from developing contrast holes.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Profile](../../../screens/profile/README.md): the personal side of these settings
- [General](../general/README.md): language, time zone, formats
