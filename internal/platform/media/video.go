package media

import (
	"encoding/binary"
	"errors"
)

const (
	MaxVideoSize   = 30 << 20
	maxVideoMillis = 30_000
)

var (
	ErrVideoTooLong = errors.New("video terlalu panjang, maksimal 30 detik")
	ErrCorruptVideo = errors.New("file video rusak atau bukan MP4")
)

type Video struct {
	Data       []byte
	DurationMs int
}

// IsMP4 mengenali kontainer MP4/MOV dari isinya (box `ftyp` di awal file), bukan dari nama file.
func IsMP4(b []byte) bool { return len(b) >= 12 && string(b[4:8]) == "ftyp" }

// ProcessVideo membaca durasi dari box moov/mvhd dan menolak video lebih dari 30 detik. Metadata
// (box udta dan meta di dalam moov, tempat lokasi GPS biasa disimpan) ditimpa menjadi box `free`
// dengan ukuran sama, jadi offset data video tidak bergeser. Video tidak di-transcode.
func ProcessVideo(in []byte) (*Video, error) {
	if !IsMP4(in) {
		return nil, ErrCorruptVideo
	}
	data := append([]byte(nil), in...) // jangan ubah buffer milik pemanggil
	moov, ok := findBox(data, 0, len(data), "moov")
	if !ok {
		return nil, ErrCorruptVideo
	}
	mvhd, ok := findBox(data, moov.body, moov.end, "mvhd")
	if !ok {
		return nil, ErrCorruptVideo
	}
	ms, ok := durationMs(data[mvhd.body:mvhd.end])
	if !ok {
		return nil, ErrCorruptVideo
	}
	if ms > maxVideoMillis {
		return nil, ErrVideoTooLong
	}
	for _, name := range []string{"udta", "meta"} {
		for off := moov.body; off < moov.end; {
			b, ok := nextBox(data, off, moov.end)
			if !ok {
				break
			}
			if string(data[b.start+4:b.start+8]) == name {
				copy(data[b.start+4:], "free")
				clear(data[b.body:b.end])
			}
			off = b.end
		}
	}
	return &Video{Data: data, DurationMs: int(ms)}, nil
}

type box struct{ start, body, end int }

// nextBox membaca header box di off dan memastikan semuanya muat sebelum limit.
func nextBox(data []byte, off, limit int) (box, bool) {
	if off+8 > limit {
		return box{}, false
	}
	size, header := uint64(binary.BigEndian.Uint32(data[off:])), 8
	switch size {
	case 1: // ukuran 64-bit
		if off+16 > limit {
			return box{}, false
		}
		size, header = binary.BigEndian.Uint64(data[off+8:]), 16
	case 0: // sampai akhir
		size = uint64(limit - off)
	}
	if size < uint64(header) || size > uint64(limit-off) {
		return box{}, false
	}
	return box{start: off, body: off + header, end: off + int(size)}, true
}

func findBox(data []byte, from, to int, name string) (box, bool) {
	for off := from; off < to; {
		b, ok := nextBox(data, off, to)
		if !ok {
			return box{}, false
		}
		if string(data[b.start+4:b.start+8]) == name {
			return b, true
		}
		off = b.end
	}
	return box{}, false
}

// durationMs membaca mvhd versi 0 (32-bit) dan 1 (64-bit).
func durationMs(mvhd []byte) (uint64, bool) {
	if len(mvhd) < 1 {
		return 0, false
	}
	var scale, dur uint64
	switch mvhd[0] {
	case 0:
		if len(mvhd) < 20 {
			return 0, false
		}
		scale, dur = uint64(binary.BigEndian.Uint32(mvhd[12:])), uint64(binary.BigEndian.Uint32(mvhd[16:]))
	case 1:
		if len(mvhd) < 32 {
			return 0, false
		}
		scale, dur = uint64(binary.BigEndian.Uint32(mvhd[20:])), binary.BigEndian.Uint64(mvhd[24:])
	default:
		return 0, false
	}
	if scale == 0 || dur > 1<<50 {
		return 0, false
	}
	return dur * 1000 / scale, true
}
