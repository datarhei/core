package flvio

import (
	"fmt"
	"github.com/datarhei/joy4/utils/bits/pio"
	"math"
	"strings"
	"time"
)

type AMF0ParseError struct {
	Offset  int
	Message string
	Next    *AMF0ParseError
}

func (self *AMF0ParseError) Error() string {
	s := []string{}
	for p := self; p != nil; p = p.Next {
		s = append(s, fmt.Sprintf("%s:%d", p.Message, p.Offset))
	}
	return "amf0 parse error: " + strings.Join(s, ",")
}

func amf0ParseErr(message string, offset int, err error) error {
	next, _ := err.(*AMF0ParseError)
	return &AMF0ParseError{
		Offset:  offset,
		Message: message,
		Next:    next,
	}
}

type AMFMap map[string]interface{}
type AMFArray []interface{}
type AMFECMAArray map[string]interface{}

func parseBEFloat64(b []byte) float64 {
	return math.Float64frombits(pio.U64BE(b))
}

func fillBEFloat64(b []byte, f float64) int {
	pio.PutU64BE(b, math.Float64bits(f))
	return 8
}

const lenAMF0Number = 9

func fillAMF0Number(b []byte, f float64) int {
	b[0] = numbermarker
	fillBEFloat64(b[1:], f)
	return lenAMF0Number
}

const (
	numbermarker = iota
	booleanmarker
	stringmarker
	objectmarker
	movieclipmarker
	nullmarker
	undefinedmarker
	referencemarker
	ecmaarraymarker
	objectendmarker
	strictarraymarker
	datemarker
	longstringmarker
	unsupportedmarker
	recordsetmarker
	xmldocumentmarker
	typedobjectmarker
	avmplusobjectmarker
)

// amf0Value returns the AMF type for values of the same underlying type, e.g. AMFMap for a
// map[string]interface{}.
func amf0Value(val interface{}) interface{} {
	switch val := val.(type) {
	case map[string]interface{}:
		return AMFMap(val)
	case []interface{}:
		return AMFArray(val)
	}
	return val
}

func LenAMF0Val(_val interface{}) (n int) {
	switch val := amf0Value(_val).(type) {
	case int8:
		n += lenAMF0Number
	case int16:
		n += lenAMF0Number
	case int32:
		n += lenAMF0Number
	case int64:
		n += lenAMF0Number
	case int:
		n += lenAMF0Number
	case uint8:
		n += lenAMF0Number
	case uint16:
		n += lenAMF0Number
	case uint32:
		n += lenAMF0Number
	case uint64:
		n += lenAMF0Number
	case uint:
		n += lenAMF0Number
	case float32:
		n += lenAMF0Number
	case float64:
		n += lenAMF0Number

	case string:
		u := len(val)
		if u <= 0xFFFF { // the length of a string has 16 bits, of a long string 32 bits
			n += 3
		} else {
			n += 5
		}
		n += int(u)

	case AMFECMAArray:
		n += 5
		for k, v := range val {
			n += 2 + len(k)
			n += LenAMF0Val(v)
		}
		n += 3

	case AMFMap:
		n++
		for k, v := range val {
			if len(k) > 0 {
				n += 2 + len(k)
				n += LenAMF0Val(v)
			}
		}
		n += 3

	case AMFArray:
		n += 5
		for _, v := range val {
			n += LenAMF0Val(v)
		}

	case time.Time:
		n += 1 + 8 + 2

	case bool:
		n += 2

	case nil:
		n++

	default:
		// unsupported types are encoded as null, such that the message stays valid
		n++
	}

	return
}

func FillAMF0Val(b []byte, _val interface{}) (n int) {
	switch val := amf0Value(_val).(type) {
	case int8:
		n += fillAMF0Number(b[n:], float64(val))
	case int16:
		n += fillAMF0Number(b[n:], float64(val))
	case int32:
		n += fillAMF0Number(b[n:], float64(val))
	case int64:
		n += fillAMF0Number(b[n:], float64(val))
	case int:
		n += fillAMF0Number(b[n:], float64(val))
	case uint8:
		n += fillAMF0Number(b[n:], float64(val))
	case uint16:
		n += fillAMF0Number(b[n:], float64(val))
	case uint32:
		n += fillAMF0Number(b[n:], float64(val))
	case uint64:
		n += fillAMF0Number(b[n:], float64(val))
	case uint:
		n += fillAMF0Number(b[n:], float64(val))
	case float32:
		n += fillAMF0Number(b[n:], float64(val))
	case float64:
		n += fillAMF0Number(b[n:], float64(val))

	case string:
		u := len(val)
		if u <= 0xFFFF { // the length of a string has 16 bits, of a long string 32 bits
			b[n] = stringmarker
			n++
			pio.PutU16BE(b[n:], uint16(u))
			n += 2
		} else {
			b[n] = longstringmarker
			n++
			pio.PutU32BE(b[n:], uint32(u))
			n += 4
		}
		copy(b[n:], []byte(val))
		n += len(val)

	case AMFECMAArray:
		b[n] = ecmaarraymarker
		n++
		pio.PutU32BE(b[n:], uint32(len(val)))
		n += 4
		for k, v := range val {
			pio.PutU16BE(b[n:], uint16(len(k)))
			n += 2
			copy(b[n:], []byte(k))
			n += len(k)
			n += FillAMF0Val(b[n:], v)
		}
		pio.PutU24BE(b[n:], 0x000009)
		n += 3

	case AMFMap:
		b[n] = objectmarker
		n++
		for k, v := range val {
			if len(k) > 0 {
				pio.PutU16BE(b[n:], uint16(len(k)))
				n += 2
				copy(b[n:], []byte(k))
				n += len(k)
				n += FillAMF0Val(b[n:], v)
			}
		}
		pio.PutU24BE(b[n:], 0x000009)
		n += 3

	case AMFArray:
		b[n] = strictarraymarker
		n++
		pio.PutU32BE(b[n:], uint32(len(val)))
		n += 4
		for _, v := range val {
			n += FillAMF0Val(b[n:], v)
		}

	case time.Time:
		b[n] = datemarker
		n++
		u := val.UnixNano()
		f := float64(u / 1000000)
		n += fillBEFloat64(b[n:], f)
		pio.PutU16BE(b[n:], uint16(0))
		n += 2

	case bool:
		b[n] = booleanmarker
		n++
		var u uint8
		if val {
			u = 1
		} else {
			u = 0
		}
		b[n] = u
		n++

	case nil:
		b[n] = nullmarker
		n++

	default:
		// unsupported types are encoded as null, such that the message stays valid
		b[n] = nullmarker
		n++
	}

	return
}

// MaxAMF0Depth is the maximum nesting depth of objects and arrays when parsing AMF0 values.
const MaxAMF0Depth = 64

func ParseAMF0Val(b []byte) (val interface{}, n int, err error) {
	return parseAMF0Val(b, 0, 0)
}

func parseAMF0Val(b []byte, offset int, depth int) (val interface{}, n int, err error) {
	if depth > MaxAMF0Depth {
		err = amf0ParseErr("depth", offset+n, err)
		return
	}
	if len(b) < n+1 {
		err = amf0ParseErr("marker", offset+n, err)
		return
	}
	marker := b[n]
	n++

	switch marker {
	case numbermarker:
		if len(b) < n+8 {
			err = amf0ParseErr("number", offset+n, err)
			return
		}
		val = parseBEFloat64(b[n:])
		n += 8

	case booleanmarker:
		if len(b) < n+1 {
			err = amf0ParseErr("boolean", offset+n, err)
			return
		}
		val = b[n] != 0
		n++

	case stringmarker:
		if len(b) < n+2 {
			err = amf0ParseErr("string.length", offset+n, err)
			return
		}
		length := int(pio.U16BE(b[n:]))
		n += 2

		if len(b) < n+length {
			err = amf0ParseErr("string.body", offset+n, err)
			return
		}
		val = string(b[n : n+length])
		n += length

	case objectmarker, typedobjectmarker:
		if marker == typedobjectmarker {
			// the class name is followed by the properties as in an object, the class name is dropped
			if len(b) < n+2 {
				err = amf0ParseErr("typedobject.classname.length", offset+n, err)
				return
			}
			length := int(pio.U16BE(b[n:]))
			n += 2
			if len(b) < n+length {
				err = amf0ParseErr("typedobject.classname.body", offset+n, err)
				return
			}
			n += length
		}

		obj := AMFMap{}
		for {
			if len(b) < n+2 {
				err = amf0ParseErr("object.key.length", offset+n, err)
				return
			}
			length := int(pio.U16BE(b[n:]))
			n += 2
			if length == 0 {
				break
			}

			if len(b) < n+length {
				err = amf0ParseErr("object.key.body", offset+n, err)
				return
			}
			okey := string(b[n : n+length])
			n += length

			var nval int
			var oval interface{}
			if oval, nval, err = parseAMF0Val(b[n:], offset+n, depth+1); err != nil {
				err = amf0ParseErr("object.val", offset+n, err)
				return
			}
			n += nval

			obj[okey] = oval
		}
		if len(b) < n+1 {
			err = amf0ParseErr("object.end", offset+n, err)
			return
		}
		n++
		val = obj

	case nullmarker:
	case undefinedmarker:

	case ecmaarraymarker:
		if len(b) < n+4 {
			err = amf0ParseErr("array.count", offset+n, err)
			return
		}
		n += 4

		obj := AMFMap{}
		for {
			if len(b) < n+2 {
				err = amf0ParseErr("array.key.length", offset+n, err)
				return
			}
			length := int(pio.U16BE(b[n:]))
			n += 2

			if length == 0 {
				break
			}

			if len(b) < n+length {
				err = amf0ParseErr("array.key.body", offset+n, err)
				return
			}
			okey := string(b[n : n+length])
			n += length

			var nval int
			var oval interface{}
			if oval, nval, err = parseAMF0Val(b[n:], offset+n, depth+1); err != nil {
				err = amf0ParseErr("array.val", offset+n, err)
				return
			}
			n += nval

			obj[okey] = oval
		}
		if len(b) < n+1 {
			err = amf0ParseErr("array.end", offset+n, err)
			return
		}
		n += 1
		val = obj

	case objectendmarker:
		if len(b) < n+3 {
			err = amf0ParseErr("objectend", offset+n, err)
			return
		}
		n += 3

	case strictarraymarker:
		if len(b) < n+4 {
			err = amf0ParseErr("strictarray.count", offset+n, err)
			return
		}
		count := int(pio.U32BE(b[n:]))
		n += 4

		// each value takes at least one byte (the marker)
		if count > len(b)-n {
			err = amf0ParseErr("strictarray.count", offset+n, err)
			return
		}

		obj := make(AMFArray, count)
		for i := 0; i < int(count); i++ {
			var nval int
			if obj[i], nval, err = parseAMF0Val(b[n:], offset+n, depth+1); err != nil {
				err = amf0ParseErr("strictarray.val", offset+n, err)
				return
			}
			n += nval
		}
		val = obj

	case datemarker:
		if len(b) < n+8+2 {
			err = amf0ParseErr("date", offset+n, err)
			return
		}
		ts := parseBEFloat64(b[n:])
		n += 8 + 2

		val = time.Unix(int64(ts/1000), (int64(ts)%1000)*1000000)

	case longstringmarker:
		if len(b) < n+4 {
			err = amf0ParseErr("longstring.length", offset+n, err)
			return
		}
		length := int(pio.U32BE(b[n:]))
		n += 4

		if len(b) < n+length {
			err = amf0ParseErr("longstring.body", offset+n, err)
			return
		}
		val = string(b[n : n+length])
		n += length

	default:
		err = amf0ParseErr(fmt.Sprintf("invalidmarker=%d", marker), offset+n, err)
		return
	}

	return
}
