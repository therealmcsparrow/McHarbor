// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import "testing"

func TestSPICEOptionsRenderIntoTheSpiceArgument(t *testing.T) {
	paths := PathsFor("vm1")
	spec := spiceTestSpec(true)
	spec.SpiceOptions = SpiceOptions{
		ImageCompression:      "off",
		JPEGWANCompression:    "never",
		ZlibGLZWANCompression: "never",
		StreamingVideo:        "filter",
		PlaybackCompression:   "on",
		AgentMouse:            "on",
	}
	argv, err := BuildArgv("/usr/bin/qemu-system-x86_64", spec, paths)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := argAfter(argv, "-spice")
	if !ok {
		t.Fatalf("no -spice in %v", argv)
	}
	want := "unix=on,addr=" + paths.SPICESocket + ",disable-ticketing=on" +
		",image-compression=off,jpeg-wan-compression=never,zlib-glz-wan-compression=never" +
		",streaming-video=filter,playback-compression=on,agent-mouse=on"
	if got != want {
		t.Fatalf("-spice = %q\nwant      %q", got, want)
	}
}

// An unknown or empty value is dropped so QEMU keeps its own default: QEMU
// refuses to start on a value it does not know.
func TestSPICEOptionsDropUnknownAndEmptyValues(t *testing.T) {
	paths := PathsFor("vm1")
	spec := spiceTestSpec(true)
	spec.SpiceOptions = SpiceOptions{
		ImageCompression:   "lz4",
		JPEGWANCompression: "always",
		StreamingVideo:     "all,addr=/tmp/evil",
		AgentMouse:         "",
	}
	argv, err := BuildArgv("/usr/bin/qemu-system-x86_64", spec, paths)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := argAfter(argv, "-spice")
	want := "unix=on,addr=" + paths.SPICESocket + ",disable-ticketing=on,jpeg-wan-compression=always"
	if got != want {
		t.Fatalf("-spice = %q\nwant      %q", got, want)
	}
}

func TestSPICEOptionsAreIgnoredWithoutSPICE(t *testing.T) {
	spec := spiceTestSpec(false)
	spec.SpiceOptions = SpiceOptions{ImageCompression: "off"}
	argv, err := BuildArgv("/usr/bin/qemu-system-x86_64", spec, PathsFor("vm1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range argv {
		if arg == "-spice" {
			t.Fatalf("SPICE options produced a -spice for a guest without SPICE: %v", argv)
		}
	}
}
