// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"encoding/binary"
	"io"
	"strings"
)

// DemuxOutput holds separated stdout and stderr from a Docker stream.
type DemuxOutput struct {
	Stdout string
	Stderr string
}

// DemuxDockerStream separates a multiplexed Docker stream into stdout/stderr.
// Docker uses an 8-byte header per frame:
//   - byte 0: stream type (0=stdin, 1=stdout, 2=stderr)
//   - bytes 1-3: reserved
//   - bytes 4-7: frame size (big-endian uint32)
func DemuxDockerStream(data []byte) DemuxOutput {
	var stdout, stderr strings.Builder
	offset := 0

	for offset < len(data) {
		if offset+8 > len(data) {
			break
		}

		streamType := data[offset]
		size := binary.BigEndian.Uint32(data[offset+4 : offset+8])
		offset += 8

		if offset+int(size) > len(data) {
			break
		}

		chunk := string(data[offset : offset+int(size)])
		switch streamType {
		case 1:
			stdout.WriteString(chunk)
		case 2:
			stderr.WriteString(chunk)
		}

		offset += int(size)
	}

	return DemuxOutput{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}

// DemuxReader reads from a Docker multiplexed stream and writes to stdout/stderr writers.
func DemuxReader(r io.Reader, stdout, stderr io.Writer) error {
	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(r, header)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		streamType := header[0]
		size := binary.BigEndian.Uint32(header[4:8])

		var w io.Writer
		switch streamType {
		case 1:
			w = stdout
		case 2:
			w = stderr
		default:
			w = io.Discard
		}

		if _, err := io.CopyN(w, r, int64(size)); err != nil {
			return err
		}
	}
}
