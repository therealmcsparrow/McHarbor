# Themes: Reference

| Setting | Default | What it does |
|---|---|---|
| **Variant** | Dark | Light or Dark. McHarbor is dark-first; light is a full theme, not an afterthought |
| **Theme accent** | Blue | Blue, Green, Purple, Orange, Rose, Cyan |
| **Base font size** | shipped value | "The root size every other text size is derived from", in px. The derived scale is shown as you change it |
| **Motion** | follows the OS | Animations and transitions, with the currently applied state shown: reduced motion or full motion |
| **Let users override these defaults** | On | "When off, every account uses the defaults above and the personal appearance controls are disabled" |
| **Reset overrides** | — | "Every account goes back to the defaults above." Clears appearance choices saved on individual accounts |
| **Colour variables** | theme values | Per-variable OKLCH overrides for the active variant, badged **overridden** |

**Reset overrides** affects everyone at once, so it is worth announcing before you press
it. **Reset variables** returns the palette to the active theme’s values; **Reset theme to
defaults** resets the whole section.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| A user cannot change their theme | Overrides are switched off | Turn **Let users override these defaults** back on |
| A colour looks wrong in one variant only | Overrides apply to the active variant alone | Check and fix both variants |
| Everyone’s personal theme disappeared | **Reset overrides** was used | It clears saved choices on every account; users can set them again if overrides are allowed |
| Motion still animates after turning it down | Motion follows the operating system preference | The section reports what is currently applied, not only what is configured |

## Related

- [Themes](./README.md): what the section is for
- [Settings](../../README.md): the other groups
