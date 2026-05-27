package main

import (
	"encoding/binary"
	"fmt"
	"math"
)

// MMDB Data Decoder (MessagePack-like with MMDB extensions)

type mmdbDecoder struct {
	data   []byte
	offset int
}

func (d *mmdbDecoder) decode() (interface{}, error) {
	if d.offset >= len(d.data) {
		return nil, fmt.Errorf("mmdb decode: offset out of range")
	}
	ctrl := d.data[d.offset]
	d.offset++

	switch {
	case ctrl <= 0xbf:
		return d.decodeExtended(ctrl)
	case ctrl >= 0xc0 && ctrl <= 0xdf:
		return d.decodeExtended(ctrl)
	case ctrl >= 0xe0:
		return int(int8(ctrl)), nil
	default:
		return nil, fmt.Errorf("mmdb decode: unknown control byte 0x%02x", ctrl)
	}
}

func (d *mmdbDecoder) decodeExtended(ctrl byte) (interface{}, error) {
	switch {
	case ctrl == 0xc0:
		return nil, nil
	case ctrl == 0xc1:
		return nil, fmt.Errorf("mmdb: reserved 0xc1")
	case ctrl == 0xc2:
		return false, nil
	case ctrl == 0xc3:
		return true, nil
	case ctrl == 0xc4:
		return d.readBytes(1)
	case ctrl == 0xc5:
		return d.readBytes(2)
	case ctrl == 0xc6:
		return d.readBytes(4)
	case ctrl == 0xc7:
		return d.readBytes(8)
	case ctrl == 0xc8:
		return d.readUint(1)
	case ctrl == 0xc9:
		return d.readUint(2)
	case ctrl == 0xca:
		return d.readFloat32()
	case ctrl == 0xcb:
		return d.readFloat64()
	case ctrl == 0xcc:
		return d.readUint8()
	case ctrl == 0xcd:
		return d.readUint16()
	case ctrl == 0xce:
		return d.readUint32()
	case ctrl == 0xcf:
		return d.readUint64()
	case ctrl == 0xd0:
		return d.readInt8()
	case ctrl == 0xd1:
		return d.readInt16()
	case ctrl == 0xd2:
		return d.readInt32()
	case ctrl == 0xd3:
		return d.readInt64()
	case ctrl == 0xd4:
		return d.readFixExt(1)
	case ctrl == 0xd5:
		return d.readFixExt(2)
	case ctrl == 0xd6:
		return d.readFixExt(4)
	case ctrl == 0xd7:
		return d.readFixExt(8)
	case ctrl == 0xd8:
		return d.readFixExt(16)
	case ctrl == 0xd9:
		return d.readStr(1)
	case ctrl == 0xda:
		return d.readStr(2)
	case ctrl == 0xdb:
		return d.readStr(4)
	case ctrl == 0xdc:
		return d.readArray(2)
	case ctrl == 0xdd:
		return d.readArray(4)
	case ctrl == 0xde:
		return d.readMap(2)
	case ctrl == 0xdf:
		return d.readMap(4)
	default:
		if ctrl <= 0x7f {
			return int(ctrl), nil
		}
		if ctrl >= 0x80 && ctrl <= 0x8f {
			return d.readMap(int(ctrl & 0x0f))
		}
		if ctrl >= 0x90 && ctrl <= 0x9f {
			return d.readArray(int(ctrl & 0x0f))
		}
		if ctrl >= 0xa0 && ctrl <= 0xbf {
			return d.readStr(int(ctrl & 0x1f))
		}
		return nil, fmt.Errorf("mmdb: unhandled control 0x%02x", ctrl)
	}
}

func (d *mmdbDecoder) readBytes(sizeLen int) ([]byte, error) {
	size := d.readUintRaw(sizeLen)
	if d.offset+int(size) > len(d.data) {
		return nil, fmt.Errorf("mmdb: bytes out of range")
	}
	result := make([]byte, size)
	copy(result, d.data[d.offset:d.offset+int(size)])
	d.offset += int(size)
	return result, nil
}

func (d *mmdbDecoder) readStr(sizeLen int) (string, error) {
	raw, err := d.readBytes(sizeLen)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (d *mmdbDecoder) readArray(sizeLen int) ([]interface{}, error) {
	count := int(d.readUintRaw(sizeLen))
	result := make([]interface{}, count)
	for i := 0; i < count; i++ {
		v, err := d.decode()
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

func (d *mmdbDecoder) readMap(sizeLen int) (map[string]interface{}, error) {
	count := int(d.readUintRaw(sizeLen))
	result := make(map[string]interface{}, count)
	for i := 0; i < count; i++ {
		keyVal, err := d.decode()
		if err != nil {
			return nil, err
		}
		key, ok := keyVal.(string)
		if !ok {
			key = toString(keyVal)
		}
		val, err := d.decode()
		if err != nil {
			return nil, err
		}
		result[key] = val
	}
	return result, nil
}

func (d *mmdbDecoder) readUint(sizeLen int) (uint64, error) {
	return d.readUintRaw(sizeLen), nil
}

func (d *mmdbDecoder) readUintRaw(sizeLen int) uint64 {
	if d.offset+sizeLen > len(d.data) {
		return 0
	}
	var v uint64
	for i := 0; i < sizeLen; i++ {
		v = (v << 8) | uint64(d.data[d.offset+i])
	}
	d.offset += sizeLen
	return v
}

func (d *mmdbDecoder) readUint8() (uint8, error) {
	if d.offset >= len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint8")
	}
	v := d.data[d.offset]
	d.offset++
	return v, nil
}

func (d *mmdbDecoder) readUint16() (uint16, error) {
	if d.offset+2 > len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint16")
	}
	v := binary.BigEndian.Uint16(d.data[d.offset:])
	d.offset += 2
	return v, nil
}

func (d *mmdbDecoder) readUint32() (uint32, error) {
	if d.offset+4 > len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint32")
	}
	v := binary.BigEndian.Uint32(d.data[d.offset:])
	d.offset += 4
	return v, nil
}

func (d *mmdbDecoder) readUint64() (uint64, error) {
	if d.offset+8 > len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint64")
	}
	v := binary.BigEndian.Uint64(d.data[d.offset:])
	d.offset += 8
	return v, nil
}

func (d *mmdbDecoder) readInt8() (int8, error) {
	v, err := d.readUint8()
	return int8(v), err
}

func (d *mmdbDecoder) readInt16() (int16, error) {
	v, err := d.readUint16()
	return int16(v), err
}

func (d *mmdbDecoder) readInt32() (int32, error) {
	v, err := d.readUint32()
	return int32(v), err
}

func (d *mmdbDecoder) readInt64() (int64, error) {
	v, err := d.readUint64()
	return int64(v), err
}

func (d *mmdbDecoder) readFloat32() (float32, error) {
	v, err := d.readUint32()
	return float32frombits(v), err
}

func (d *mmdbDecoder) readFloat64() (float64, error) {
	v, err := d.readUint64()
	return float64frombits(v), err
}

func float32frombits(b uint32) float32 {
	return math.Float32frombits(b)
}

func float64frombits(b uint64) float64 {
	return math.Float64frombits(b)
}

func (d *mmdbDecoder) readFixExt(size int) ([]byte, error) {
	if d.offset+size > len(d.data) {
		return nil, fmt.Errorf("mmdb: fixext out of range")
	}
	result := make([]byte, size)
	copy(result, d.data[d.offset:d.offset+size])
	d.offset += size
	return result, nil
}

func toUint32(v interface{}) uint32 {
	switch val := v.(type) {
	case uint32:
		return val
	case float64:
		return uint32(val)
	case uint64:
		return uint32(val)
	case int:
		return uint32(val)
	case int64:
		return uint32(val)
	}
	return 0
}

func toUint64(v interface{}) uint64 {
	switch val := v.(type) {
	case uint64:
		return val
	case float64:
		return uint64(val)
	case uint32:
		return uint64(val)
	case int:
		return uint64(val)
	}
	return 0
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		result = append(result, toString(item))
	}
	return result
}
