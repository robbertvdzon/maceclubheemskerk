package content

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

// JPEG camera orientation is applied before metadata is removed by re-encoding.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for p := 2; p+4 <= len(data); {
		if data[p] != 0xff {
			return 1
		}
		marker := data[p+1]
		if marker == 0xda || marker == 0xd9 {
			return 1
		}
		n := int(binary.BigEndian.Uint16(data[p+2 : p+4]))
		if n < 2 || p+2+n > len(data) {
			return 1
		}
		segment := data[p+4 : p+2+n]
		p += 2 + n
		if marker != 0xe1 || !bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			continue
		}
		tiff := segment[6:]
		if len(tiff) < 8 {
			return 1
		}
		var order binary.ByteOrder
		if string(tiff[:2]) == "II" {
			order = binary.LittleEndian
		} else if string(tiff[:2]) == "MM" {
			order = binary.BigEndian
		} else {
			return 1
		}
		if order.Uint16(tiff[2:4]) != 42 {
			return 1
		}
		offset := uint64(order.Uint32(tiff[4:8]))
		if offset+2 > uint64(len(tiff)) {
			return 1
		}
		count := uint64(order.Uint16(tiff[offset : offset+2]))
		for i := uint64(0); i < count; i++ {
			at := offset + 2 + i*12
			if at+12 > uint64(len(tiff)) {
				return 1
			}
			entry := tiff[at : at+12]
			if order.Uint16(entry[:2]) == 0x0112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
				v := int(order.Uint16(entry[8:10]))
				if v >= 1 && v <= 8 {
					return v
				}
				return 1
			}
		}
	}
	return 1
}

type orientedImage struct {
	image.Image
	orientation int
}

func (o orientedImage) Bounds() image.Rectangle {
	b := o.Image.Bounds()
	if o.orientation >= 5 {
		return image.Rect(0, 0, b.Dy(), b.Dx())
	}
	return image.Rect(0, 0, b.Dx(), b.Dy())
}
func (o orientedImage) At(x, y int) color.Color {
	b := o.Image.Bounds()
	w, h := b.Dx(), b.Dy()
	switch o.orientation {
	case 2:
		x = w - 1 - x
	case 3:
		x, y = w-1-x, h-1-y
	case 4:
		y = h - 1 - y
	case 5:
		x, y = y, x
	case 6:
		x, y = y, h-1-x
	case 7:
		x, y = w-1-y, h-1-x
	case 8:
		x, y = w-1-y, x
	}
	return o.Image.At(x+b.Min.X, y+b.Min.Y)
}
