# Third-party notices — addon-kvm

This add-on is licensed under the McHarbor-AddOns License (see `LICENSE`). Its browser bundle,
`frontend/addon-kvm.umd.cjs`, also contains the third-party code below, which is licensed
separately and stays under its own terms.

## spice-html5 (SPICE console client)

| | |
|---|---|
| Package | `@spice-project/spice-html5` |
| Version | 0.2.1, pinned exactly in `frontend/package.json` |
| License | GNU Lesser General Public License, version 3 or (at your option) any later version (`LGPL-3.0-or-later`) |
| Copyright | Copyright (C) 2012, 2014 Jeremy P. White and the SPICE Project contributors |
| Project | https://www.spice-space.org |
| Source | https://gitlab.freedesktop.org/spice/spice-html5 (the `0.2.1` release) |
| Package page | https://www.npmjs.com/package/@spice-project/spice-html5 |
| License texts | `COPYING` and `COPYING.LESSER` in the package, https://www.gnu.org/licenses/lgpl-3.0.txt and https://www.gnu.org/licenses/gpl-3.0.txt |

The package also carries a few files under their own notices, kept in the package's
`src/thirdparty/` folder: the RSA and big-number code (`jsbn`, `rsa`, `rng`, `prng4`) is
Copyright (c) 2003-2005 Tom Wu, under the BSD-style terms stated at the top of each file.

### What McHarbor does with it

The console pane (`frontend/src/components/SpiceConsolePane.tsx`,
`frontend/src/hooks/useSpiceConsole.ts` and `frontend/src/spice-session.ts`) loads the library and
talks to it through its public `SpiceMainConn` interface. The library is bundled, not dynamically
linked, and it is only initialised the first time an operator connects with SPICE.

McHarbor makes **one** change to the library, at build time and not on disk. The build
transform `spiceChannelUrls()` in `frontend/vite-plugin.config.ts` rewrites the single expression

```text
uri: this.ws.url,
```

in `src/main.js` to

```text
uri: typeof this.channel_uri === 'function' ? this.channel_uri() : this.ws.url,
```

so that each SPICE channel can open on its own single-use console ticket. The package in
`node_modules` is unmodified. If upstream changes that expression, the transform throws and the
build fails rather than shipping every channel on one URL.

### How to obtain the source

- From upstream: clone https://gitlab.freedesktop.org/spice/spice-html5 and check out the release
  tagged `0.2.1`, or run `npm pack @spice-project/spice-html5@0.2.1`.
- From this add-on: `frontend/package-lock.json` records the exact tarball and its integrity hash,
  and `npm install` in `frontend/` restores it. The complete source of the add-on, including the
  build transform above, is this directory.

### How to replace it with your own version

You may replace the library with a modified or newer version and rebuild the add-on:

1. In `frontend/`, change the `@spice-project/spice-html5` version in `package.json`, or point it at
   your own build (a local path, a git URL or a tarball).
2. Run `npm install`, then `npm run build`. This rewrites `frontend/addon-kvm.umd.cjs`.
3. Rebuild the plugin package with `scripts/development/build-plugin.sh` (see its usage text), which runs the
   frontend build again and packages the result.

The build fails with "spice-html5 main.js no longer opens channels on this.ws.url" if the library
no longer contains the expression in `spiceChannelUrls()`. In that case update the transform to
match, or remove it and supply per-channel URLs in your own way.

### A note on the packaged file

The plugin packager copies `plugin.yaml`, `i18n/`, `icons/`, `migrations/`, the `.so` and
`frontend/*.umd.cjs` into the plugin tarball. It does not copy this file, and the minifier removes
the licence comment from the bundled library. A tarball therefore does not carry this notice by
itself, so it has to be made available alongside the tarball wherever the plugin is distributed.
