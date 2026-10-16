# Images

`/images` · `/images/:id`

Docker images on the selected environment, and what is inside each one.

## Permissions

| Key | What it unlocks |
|---|---|
| `images.view` | See images and their detail |
| `images.manage` | Pull, tag, import, export, prune |
| `images.delete` | Remove an image |
| `scans.view` / `scans.manage` | Read and run vulnerability scans |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- Table or card view of every image, with an **in use** or **unused** badge per row.
- A detail page with four tabs: Overview, Config, Labels and History.
- Layer history: the instruction that created each layer, its size and when.

## Why layer history matters

An image's size comes from its layers. A layer that adds 400 MB and a later layer that
deletes the same files leaves both in the image; the History tab is what shows you that.

Images are badged **tagged** or **dangling**. A dangling image has no repository or tag
left (usually the previous build of something rebuilt) and is the most common thing a
prune removes.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): what runs these images
- [Docker](../docker/README.md): registry credentials for pulls
- [Settings → Updates → Containers](../../settings/updates/containers/README.md): automatic image updates
