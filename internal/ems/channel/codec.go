package channel

import (
	"fmt"
	"math"

	"fastems/internal/ems/signal"
)

// decode turns the words or bits of one block into a typed value. base is the
// block start address; the point address locates its offset inside the block.
func decode(point Point, base uint16, bits []bool, words []uint16) (signal.Value, error) {
	offset := point.Address - base
	if point.boolean() {
		if int(offset) >= len(bits) {
			return signal.Value{}, fmt.Errorf("point %s: bit %d outside block of %d", point.ID, offset, len(bits))
		}
		return signal.Flag(bits[offset]), nil
	}
	size := int(point.Size())
	if int(offset)+size > len(words) {
		return signal.Value{}, fmt.Errorf("point %s: register %d/%d outside block of %d", point.ID, offset, size, len(words))
	}
	scale := point.Scale
	if scale == 0 {
		scale = 1
	}
	var raw float64
	switch point.Format {
	case "uint16", "":
		raw = float64(words[offset])
	case "int16":
		raw = float64(int16(words[offset]))
	case "uint32":
		raw = float64(uint32(words[offset])<<16 | uint32(words[offset+1]))
	case "int32":
		raw = float64(int32(uint32(words[offset])<<16 | uint32(words[offset+1])))
	case "float32":
		raw = float64(math.Float32frombits(uint32(words[offset])<<16 | uint32(words[offset+1])))
	default:
		return signal.Value{}, fmt.Errorf("point %s: unsupported format %q", point.ID, point.Format)
	}
	value := raw*scale + point.Offset
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return signal.Value{}, fmt.Errorf("point %s: decoded value is not finite", point.ID)
	}
	return signal.Num(value), nil
}

// encode converts an engineering value into the words or coil to write.
func encode(point Point, value signal.Value) (bool, []uint16, error) {
	if point.boolean() {
		if value.Kind != signal.Bool {
			return false, nil, fmt.Errorf("point %s expects bool", point.ID)
		}
		return value.Bool, nil, nil
	}
	if value.Kind != signal.Number {
		return false, nil, fmt.Errorf("point %s expects a number", point.ID)
	}
	if !value.Valid() {
		return false, nil, fmt.Errorf("point %s: value is not finite", point.ID)
	}
	scale := point.Scale
	if scale == 0 {
		scale = 1
	}
	raw := (value.Num - point.Offset) / scale
	switch point.Format {
	case "uint16", "":
		if raw < 0 || raw > math.MaxUint16 || math.Abs(raw-math.Round(raw)) > 1e-6 {
			return false, nil, fmt.Errorf("point %s: value %g does not fit an unsigned 16-bit register", point.ID, value.Num)
		}
		return false, []uint16{uint16(math.Round(raw))}, nil
	case "int16":
		if raw < math.MinInt16 || raw > math.MaxInt16 || math.Abs(raw-math.Round(raw)) > 1e-6 {
			return false, nil, fmt.Errorf("point %s: value %g does not fit a signed 16-bit register", point.ID, value.Num)
		}
		return false, []uint16{uint16(int16(math.Round(raw)))}, nil
	case "uint32":
		if raw < 0 || raw > math.MaxUint32 || math.Abs(raw-math.Round(raw)) > 1e-6 {
			return false, nil, fmt.Errorf("point %s: value %g does not fit an unsigned 32-bit register pair", point.ID, value.Num)
		}
		bits := uint32(math.Round(raw))
		return false, []uint16{uint16(bits >> 16), uint16(bits)}, nil
	case "int32":
		if raw < math.MinInt32 || raw > math.MaxInt32 || math.Abs(raw-math.Round(raw)) > 1e-6 {
			return false, nil, fmt.Errorf("point %s: value %g does not fit a signed 32-bit register pair", point.ID, value.Num)
		}
		bits := uint32(int32(math.Round(raw)))
		return false, []uint16{uint16(bits >> 16), uint16(bits)}, nil
	case "float32":
		bits := math.Float32bits(float32(raw))
		return false, []uint16{uint16(bits >> 16), uint16(bits)}, nil
	default:
		return false, nil, fmt.Errorf("point %s: unsupported format %q", point.ID, point.Format)
	}
}
