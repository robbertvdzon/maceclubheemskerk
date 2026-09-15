package content

import (
	"encoding/binary"
	"errors"
	"io"
)

// Inspect MP4 metadata without allocating memory for sample tables or video data.
// Playback support is intentionally limited to H.264 with optional AAC audio.
// Validation is not a full video decoder; the player also reports damaged streams.
func validateMP4(r io.ReaderAt, size int64) error {
	bad := errors.New("Gebruik een geldig MP4-bestand met H.264-video en eventueel AAC-geluid.")
	if size < 32 {
		return bad
	}
	boxes, ftyp, moov, mdat, video, samples := 0, false, false, false, false, false
	var walk func(int64, int64, int) error
	walk = func(start, end int64, depth int) error {
		if depth > 10 {
			return bad
		}
		for pos := start; pos < end; {
			boxes++
			if boxes > 10000 || end-pos < 8 {
				return bad
			}
			var header [16]byte
			if _, e := r.ReadAt(header[:8], pos); e != nil {
				return bad
			}
			length := int64(binary.BigEndian.Uint32(header[:4]))
			head := int64(8)
			kind := string(header[4:8])
			if length == 1 {
				if end-pos < 16 {
					return bad
				}
				if _, e := r.ReadAt(header[8:], pos+8); e != nil {
					return bad
				}
				n := binary.BigEndian.Uint64(header[8:])
				if n > uint64(end-pos) {
					return bad
				}
				length = int64(n)
				head = 16
			} else if length == 0 {
				length = end - pos
			}
			if length < head || length > end-pos {
				return bad
			}
			body, finish := pos+head, pos+length
			switch kind {
			case "ftyp":
				if depth != 0 || ftyp || length-head < 8 || length-head > 1024 {
					return bad
				}
				ftyp = true
				var brand [4]byte
				if _, e := r.ReadAt(brand[:], body); e != nil {
					return bad
				}
				switch string(brand[:]) {
				case "isom", "iso2", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "M4V ":
				default:
					return bad
				}
			case "moov":
				if depth != 0 || moov {
					return bad
				}
				moov = true
				if e := walk(body, finish, depth+1); e != nil {
					return e
				}
			case "trak", "mdia", "minf", "stbl":
				if e := walk(body, finish, depth+1); e != nil {
					return e
				}
			case "mdat":
				if depth != 0 || finish-body == 0 {
					return bad
				}
				mdat = true
			case "mvex", "moof":
				return errors.New("Gebruik een volledig MP4-bestand; losse videofragmenten worden niet ondersteund.")
			case "stsd":
				if depth != 5 || finish-body < 8 {
					return bad
				}
				var full [8]byte
				if _, e := r.ReadAt(full[:], body); e != nil {
					return bad
				}
				count := binary.BigEndian.Uint32(full[4:])
				if count == 0 || count > 8 {
					return bad
				}
				p := body + 8
				for j := uint32(0); j < count; j++ {
					var sample [8]byte
					if finish-p < 8 {
						return bad
					}
					if _, e := r.ReadAt(sample[:], p); e != nil {
						return bad
					}
					n := int64(binary.BigEndian.Uint32(sample[:4]))
					if n < 16 || n > finish-p {
						return bad
					}
					switch string(sample[4:]) {
					case "avc1", "avc3":
						if n < 86 {
							return bad
						}
						var wh [4]byte
						if _, e := r.ReadAt(wh[:], p+32); e != nil {
							return bad
						}
						w, h := binary.BigEndian.Uint16(wh[:2]), binary.BigEndian.Uint16(wh[2:])
						if w == 0 || h == 0 || w > 8192 || h > 8192 {
							return bad
						}
						video = true
					case "mp4a":
						if n < 36 {
							return bad
						}
					default:
						return bad
					}
					p += n
				}
				if p != finish {
					return bad
				}
			case "stsz":
				if finish-body < 12 {
					return bad
				}
				var st [12]byte
				if _, e := r.ReadAt(st[:], body); e != nil {
					return bad
				}
				count := int64(binary.BigEndian.Uint32(st[8:]))
				if count < 1 || count > 10000000 {
					return bad
				}
				if binary.BigEndian.Uint32(st[4:8]) == 0 && count > (finish-body-12)/4 {
					return bad
				}
				samples = true
			case "dref":
				if finish-body < 8 {
					return bad
				}
				var full [8]byte
				if _, e := r.ReadAt(full[:], body); e != nil {
					return bad
				}
				count := binary.BigEndian.Uint32(full[4:])
				if count != 1 || finish-body != 20 {
					return bad
				}
				var entry [12]byte
				if _, e := r.ReadAt(entry[:], body+8); e != nil {
					return bad
				}
				if binary.BigEndian.Uint32(entry[:4]) != 12 || string(entry[4:8]) != "url " || binary.BigEndian.Uint32(entry[8:]) != 1 {
					return bad
				}
			case "dinf":
				if e := walk(body, finish, depth+1); e != nil {
					return e
				}
			}
			pos = finish
		}
		return nil
	}
	if err := walk(0, size, 0); err != nil {
		return err
	}
	if !ftyp || !moov || !mdat || !video || !samples {
		return bad
	}
	return nil
}
