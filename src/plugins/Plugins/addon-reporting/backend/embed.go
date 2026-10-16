// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"sync"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/fontkit"
)

// Embedded assets.
//
// The fonts and logos are compiled into the plugin's shared object
// rather than read from disk at runtime. The plugin tarball only carries
// the manifest, the .so, the UMD bundle, i18n and icons, so anything the
// exporters need has to travel inside the binary — and a report that
// silently loses its typeface because an asset directory was not shipped
// is worse than one that fails loudly at build time.
//
// Inter Tight and JetBrains Mono are both SIL Open Font License 1.1,
// which permits embedding and redistribution. The licence text ships
// alongside them in assets/fonts/OFL.txt and is reproduced in the
// plugin's LICENSE notice.

//go:embed assets/fonts/InterTight-Regular.ttf
var fontInterRegular []byte

//go:embed assets/fonts/InterTight-SemiBold.ttf
var fontInterSemiBold []byte

//go:embed assets/fonts/InterTight-Bold.ttf
var fontInterBold []byte

//go:embed assets/fonts/JetBrainsMono-Regular.ttf
var fontJetBrainsMono []byte

//go:embed assets/img/logo-wordmark.png
var logoWordmarkPNG []byte

//go:embed assets/img/logo-mark.png
var logoMarkPNG []byte

var (
	assetsOnce sync.Once
	assetsVal  *assets
	assetsErr  error
)

// loadAssets parses the embedded fonts once and caches the result. Font
// parsing is a few milliseconds, but a busy install generating several
// reports has no reason to repeat it.
func loadAssets() (*assets, error) {
	assetsOnce.Do(func() {
		a := &assets{
			fontRegular:  fontInterRegular,
			fontSemiBold: fontInterSemiBold,
			fontBold:     fontInterBold,
			fontMono:     fontJetBrainsMono,
			logoWordmark: logoWordmarkPNG,
			logoMark:     logoMarkPNG,
		}
		var err error
		if a.faceRegular, err = fontkit.Parse(fontInterRegular); err != nil {
			assetsErr = fmt.Errorf("reporting: parse regular face: %w", err)
			return
		}
		if a.faceSemiBold, err = fontkit.Parse(fontInterSemiBold); err != nil {
			assetsErr = fmt.Errorf("reporting: parse semibold face: %w", err)
			return
		}
		if cfg, err := png.DecodeConfig(bytes.NewReader(logoWordmarkPNG)); err == nil {
			a.wordmarkW, a.wordmarkH = cfg.Width, cfg.Height
		}
		assetsVal = a
	})
	return assetsVal, assetsErr
}
