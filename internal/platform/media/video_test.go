package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func mkBox(name string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	b := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(b, uint32(8+len(body)))
	copy(b[4:], name)
	return append(b, body...)
}

// mvhd versi 0: version/flags(4) created(4) modified(4) timescale(4) duration(4) + sisa dummy.
func mkMvhd(scale, dur uint32) []byte {
	p := make([]byte, 100)
	binary.BigEndian.PutUint32(p[12:], scale)
	binary.BigEndian.PutUint32(p[16:], dur)
	return mkBox("mvhd", p)
}

func mkMP4(scale, dur uint32) []byte {
	udta := mkBox("udta", mkBox("\xa9xyz", []byte("+06.2433+106.7989/")))
	return bytes.Join([][]byte{
		mkBox("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41")),
		mkBox("moov", mkMvhd(scale, dur), udta),
		mkBox("mdat", []byte("frame-data")),
	}, nil)
}

func TestProcessVideo(t *testing.T) {
	in := mkMP4(1000, 12_500)
	v, err := ProcessVideo(in)
	if err != nil || v.DurationMs != 12_500 {
		t.Fatalf("durasi: %+v %v", v, err)
	}
	if bytes.Contains(v.Data, []byte("+106.7989")) || bytes.Contains(v.Data, []byte("udta")) {
		t.Error("metadata lokasi harus dibuang")
	}
	if len(v.Data) != len(in) || !bytes.Contains(v.Data, []byte("frame-data")) {
		t.Error("ukuran file dan data video tidak boleh berubah")
	}
	if !bytes.Contains(in, []byte("+106.7989")) {
		t.Error("buffer input tidak boleh diubah")
	}

	if _, err := ProcessVideo(mkMP4(1000, 30_001)); !errors.Is(err, ErrVideoTooLong) {
		t.Errorf("30,001 detik: %v", err)
	}
	if v, err := ProcessVideo(mkMP4(600, 18_000)); err != nil || v.DurationMs != 30_000 {
		t.Errorf("tepat 30 detik harus lolos: %+v %v", v, err)
	}
	for name, bad := range map[string][]byte{
		"bukan mp4":     []byte("GIF89a......................"),
		"tanpa moov":    mkBox("ftyp", []byte("isom\x00\x00\x02\x00")),
		"timescale 0":   mkMP4(0, 100),
		"terpotong":     mkMP4(1000, 100)[:30],
		"ukuran bohong": append(mkBox("ftyp", []byte("isom\x00\x00\x02\x00")), 0xff, 0xff, 0xff, 0xff, 'm', 'o', 'o', 'v'),
	} {
		if _, err := ProcessVideo(bad); !errors.Is(err, ErrCorruptVideo) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
