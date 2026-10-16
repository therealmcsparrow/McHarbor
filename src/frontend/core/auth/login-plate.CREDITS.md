# Login plate credits

The three photographs behind the login screen, rotating on a 30-second cycle.
All are free-licence Unsplash photographs, downloaded and served locally from
`src/frontend/public/` — the login page makes no third-party request.

Attribution is **not required** by the Unsplash License. It is recorded here so
the provenance of every shipped asset is auditable, which matters for a product
distributed under its own licence, and the photographer's name is also shown in
the plate's title block while their photo is on screen.

> Unsplash License: free for commercial and non-commercial use, no permission
> needed. Images may not be sold without significant modification, and may not
> be compiled to replicate a competing stock-image service.
> <https://unsplash.com/license>

| File              | Subject                                | Photographer                                                        | Photo                                     |
| ----------------- | -------------------------------------- | ------------------------------------------------------------------- | ----------------------------------------- |
| `login-plate-1.*` | Tower cranes at sunset                 | Roman Chukhin ([@chukhin](https://unsplash.com/@chukhin))           | <https://unsplash.com/photos/8uOgRsRuI4k> |
| `login-plate-2.*` | Port gantry cranes at sunset, Oakland  | Cedric Letsch ([@cedricletsch](https://unsplash.com/@cedricletsch)) | <https://unsplash.com/photos/XL_MT3iS_s8> |
| `login-plate-3.*` | Milwaukee pierhead lighthouse, moonlit | Mick Haupt ([@rocinante_11](https://unsplash.com/@rocinante_11))    | <https://unsplash.com/photos/DabGDzEadTQ> |

Each ships as AVIF, WebP and JPEG. AVIF is 15.1 / 16.9 / 11.4 KB — 43 KB for the
whole set, which is why all three can be mounted at once and crossfaded without
a swap ever waiting on the network.

## The selection rule

**No third-party branding, no recognisable people, no property marks.** The
Unsplash License covers the photograph; it does not grant rights over a
trademark inside the frame, and a login screen carrying another company's livery
invites an association claim against a product being licensed to others. Every
plate above is clean, and every rejection below failed on exactly that.

## Rejected

- _Stacked shipping containers_ (Marcus Reubenstein) — frame dominated by Maersk marks.
- _Cargo port crane, Vancouver_ (Kyle Ryan) — Maersk / Evergreen liveries across the lower third.
- _Keelung port at dusk_ (Winston Chen) — Yang Ming, Wan Hai, ONE and Maersk liveries.
- _Night container terminal_ (Bob Brewer) — a single operator name on the crane beam; the best shot of the set and still not worth the argument.
- _Crane at a port_ (Wade Allen) — SpaceX booster and branding at Port Canaveral.
- _Brighton pier at night_ (Bruce Barrow) — grainy, ~90% black, and a `BRIGHTON` neon sign in frame.
- _Lights reflecting in water_ (Iain) — ~90% black; collapses in a tall crop.
- _Crane on a barge, Rotterdam_ (Georg Eiermann) — flat grey sky, busy, `FIGEE` marking.
- Several otherwise-ideal server-room shots (Planet Volumes, Alex Shuper, Getty) — **Unsplash+ paid licence**, not the free one. These are easy to mistake for free: they appear in ordinary search results and only the `plus.unsplash.com` source and a licence line distinguish them.

## Processing

Downloaded at `w=1800&q=72`, then `magick -resize 1400x -strip` at quality 46
(AVIF), 72 (WebP) and 78 (JPEG). Metadata stripped, so no EXIF GPS or camera
serials ship with the build.

## Adding or replacing a plate

1. Check the licence is the free Unsplash License, not Unsplash+.
2. Check the frame for trademarks, faces and property marks.
3. Generate the three formats at 1400px with the command above.
4. Add the file stem and the photographer to `PLATES` in `LoginPlate.tsx` — the
   title block reads the credit from there.
5. Record it in this file.
