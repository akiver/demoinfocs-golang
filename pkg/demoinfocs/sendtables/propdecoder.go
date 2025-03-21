//nolint:varcheck,deadcode
package sendtables

import (
	"fmt"
	"math"

	r3 "github.com/golang/geo/r3"

	bit "github.com/markus-wa/demoinfocs-golang/v4/internal/bitread"
)

const (
	propTypeInt int = iota
	propTypeFloat
	propTypeVector
	propTypeVectorXY
	propTypeString
	propTypeArray
	propTypeDataTable
	propTypeInt64
)

// PropertyType identifies the data type of a property.
type PropertyType int

// PropType constants are duplicated as publics so we don't have to convert ints to PropertyType for normal use.

const (
	PropTypeInt PropertyType = iota
	PropTypeFloat
	PropTypeVector
	PropTypeVectorXY
	PropTypeString
	PropTypeArray
	PropTypeDataTable
	PropTypeInt64
	PropTypeAny
)

const (
	coordFractionalBitsMp             = 5
	coordFractionalBitsMpLowPrecision = 3
	coordDenominator                  = 1 << coordFractionalBitsMp
	coordResolution                   = 1.0 / coordDenominator
	coordDenominatorLowPrecision      = 1 << coordFractionalBitsMpLowPrecision
	coordResolutionLowPrecision       = 1.0 / coordDenominatorLowPrecision
	coordIntegerBitsMp                = 11
	coordIntegerBits                  = 14
)

const (
	normalFractBits   = 11
	normalDenominator = 1 << (normalFractBits - 1)
	normalResolution  = 1.0 / normalDenominator
)

const (
	dataTableMaxStringBits   = 9
	dataTableMaxStringLength = 1 << dataTableMaxStringBits
)

const (
	propFlagUnsigned sendPropertyFlags = (1 << iota)
	propFlagCoord
	propFlagNoScale
	propFlagRoundDown
	propFlagRoundUp
	propFlagNormal
	propFlagExclude
	propFlagXYZE
	propFlagInsideArray
	propFlagProxyAlwaysYes
	propFlagIsVectorElement
	propFlagCollapsible
	propFlagCoordMp
	propFlagCoordMpLowPrecision
	propFlagCoordMpIntegral
	propFlagCellCoord
	propFlagCellCoordLowPrecision
	propFlagCellCoordIntegral
	propFlagChangesOften
	propFlagVarInt
)

const specialFloatFlags = propFlagNoScale | propFlagCoord | propFlagCellCoord | propFlagNormal | propFlagCoordMp | propFlagCoordMpLowPrecision | propFlagCoordMpIntegral | propFlagCellCoordLowPrecision | propFlagCellCoordIntegral

var propDecoder propertyDecoder

// PropertyValue stores parsed & decoded send-table values.
// For instance player health, location etc.
type PropertyValue struct {
	VectorVal r3.Vector       // Deprecated, use R3Vec() instead
	IntVal    int             // Deprecated, use Int() instead
	Int64Val  int64           // Deprecated, use Int64() instead
	ArrayVal  []PropertyValue // Deprecated.
	StringVal string          // Deprecated, use Str() instead
	FloatVal  float32         // Deprecated, use Float() instead
	Any       any
	S2        bool
}

func (v PropertyValue) R3Vec() r3.Vector {
	if v.S2 {
		fs := v.Any.([]float32)

		return r3.Vector{
			X: float64(fs[0]),
			Y: float64(fs[1]),
			Z: float64(fs[2]),
		}
	}

	return v.VectorVal
}

func (v PropertyValue) R3VecOrNil() *r3.Vector {
	if v.S2 {
		if v.Any == nil {
			return nil
		}

		fs := v.Any.([]float32)

		return &r3.Vector{
			X: float64(fs[0]),
			Y: float64(fs[1]),
			Z: float64(fs[2]),
		}
	}

	return &v.VectorVal
}

func (v PropertyValue) Int() int {
	if v.S2 {
		return int(v.Any.(int32))
	}

	return v.IntVal
}

func (v PropertyValue) Int64() int64 {
	if v.S2 {
		return v.Any.(int64)
	}

	return v.Int64Val
}

func (v PropertyValue) S2UInt64() uint64 {
	return v.Any.(uint64)
}

func (v PropertyValue) S2UInt32() uint32 {
	return v.Any.(uint32)
}

func (v PropertyValue) Handle() uint64 {
	if v.S2 {
		return v.S2UInt64()
	}

	if v.IntVal < 0 {
		panic("Handle is negative")
	}

	return uint64(v.IntVal)
}

func (v PropertyValue) Float() float32 {
	if v.S2 {
		return v.Any.(float32)
	}

	return v.FloatVal
}

func (v PropertyValue) Str() string {
	if v.S2 {
		return v.Any.(string)
	}

	return v.StringVal
}

func (v PropertyValue) String() string {
	return fmt.Sprint(v.Any)
}

// BoolVal returns true if IntVal > 0.
func (v PropertyValue) BoolVal() bool {
	if v.S2 {
		return v.Any.(bool)
	}

	return v.IntVal > 0
}

type propertyDecoder struct{}

func (propertyDecoder) decodeProp(prop *property, reader *bit.BitReader) {
	switch prop.entry.prop.rawType {
	case propTypeFloat:
		prop.value.FloatVal = propDecoder.decodeFloat(prop.entry.prop, reader)
		prop.value.Any = prop.value.FloatVal

	case propTypeInt:
		prop.value.IntVal = propDecoder.decodeInt(prop.entry.prop, reader)
		prop.value.Any = prop.value.IntVal

	case propTypeVectorXY:
		prop.value.VectorVal = propDecoder.decodeVectorXY(prop.entry.prop, reader)
		prop.value.Any = prop.value.VectorVal

	case propTypeVector:
		prop.value.VectorVal = propDecoder.decodeVector(prop.entry.prop, reader)
		prop.value.Any = prop.value.VectorVal

	case propTypeArray:
		prop.value.ArrayVal = propDecoder.decodeArray(prop.entry, reader)
		prop.value.Any = prop.value.ArrayVal

	case propTypeString:
		prop.value.StringVal = propDecoder.decodeString(reader)
		prop.value.Any = prop.value.StringVal

	case propTypeInt64:
		prop.value.Int64Val = propDecoder.decodeInt64(prop.entry.prop, reader)
		prop.value.Any = prop.value.Int64Val

	default:
		panic(fmt.Sprintf("Unknown prop type %d", prop.entry.prop.rawType))
	}
}

func (propertyDecoder) decodeInt(prop *sendTableProperty, reader *bit.BitReader) int {
	if prop.flags.hasFlagSet(propFlagVarInt) {
		if prop.flags.hasFlagSet(propFlagUnsigned) {
			return int(reader.ReadVarInt32())
		}

		return int(reader.ReadSignedVarInt32())
	}

	if prop.flags.hasFlagSet(propFlagUnsigned) {
		return int(reader.ReadInt(prop.numberOfBits))
	}

	return reader.ReadSignedInt(prop.numberOfBits)
}

func (propertyDecoder) decodeInt64(prop *sendTableProperty, reader *bit.BitReader) int64 {
	if prop.flags.hasFlagSet(propFlagVarInt) {
		if prop.flags.hasFlagSet(propFlagUnsigned) {
			return int64(reader.ReadVarInt64())
		}

		return reader.ReadSignedVarInt64()
	}

	var (
		high       uint
		low        uint
		isNegative = false
	)

	if prop.flags.hasFlagSet(propFlagUnsigned) {
		low = reader.ReadInt(32)
		high = reader.ReadInt(prop.numberOfBits - 32)
	} else {
		isNegative = reader.ReadBit()
		low = reader.ReadInt(32)
		high = reader.ReadInt(prop.numberOfBits - 32 - 1)
	}

	var result = (int64(high) << 32) | int64(low)

	if isNegative {
		result = -result
	}

	return result
}

func (propertyDecoder) decodeFloat(prop *sendTableProperty, reader *bit.BitReader) float32 {
	if prop.flags&specialFloatFlags != 0 {
		return propDecoder.decodeSpecialFloat(prop, reader)
	}

	dwInterp := reader.ReadInt(prop.numberOfBits)

	return prop.lowValue + ((prop.highValue - prop.lowValue) * (float32(dwInterp) / float32((int(1)<<uint(prop.numberOfBits))-1)))
}

func (propertyDecoder) decodeSpecialFloat(prop *sendTableProperty, reader *bit.BitReader) float32 {
	// Because multiple flags can be set this order is fixed for now (priorities).
	// TODO: Would be more efficient to first check the most common ones tho.
	if prop.flags.hasFlagSet(propFlagCoord) {
		return propDecoder.readBitCoord(reader)
	} else if prop.flags.hasFlagSet(propFlagCoordMp) {
		return propDecoder.readBitCoordMp(reader, false, false)
	} else if prop.flags.hasFlagSet(propFlagCoordMpLowPrecision) {
		return propDecoder.readBitCoordMp(reader, false, true)
	} else if prop.flags.hasFlagSet(propFlagCoordMpIntegral) {
		return propDecoder.readBitCoordMp(reader, true, false)
	} else if prop.flags.hasFlagSet(propFlagNoScale) {
		return reader.ReadFloat()
	} else if prop.flags.hasFlagSet(propFlagNormal) {
		return propDecoder.readBitNormal(reader)
	} else if prop.flags.hasFlagSet(propFlagCellCoord) {
		return propDecoder.readBitCellCoord(reader, prop.numberOfBits, false, false)
	} else if prop.flags.hasFlagSet(propFlagCellCoordLowPrecision) {
		return propDecoder.readBitCellCoord(reader, prop.numberOfBits, true, false)
	} else if prop.flags.hasFlagSet(propFlagCellCoordIntegral) {
		return propDecoder.readBitCellCoord(reader, prop.numberOfBits, false, true)
	}

	panic(fmt.Sprintf("Unexpected special float flag (Flags %v)", prop.flags))
}

func (propertyDecoder) readBitCoord(reader *bit.BitReader) float32 {
	var (
		intVal, fractVal int
		res              float32
		isNegative       = false
	)

	intVal = int(reader.ReadInt(1))
	fractVal = int(reader.ReadInt(1))

	if intVal|fractVal != 0 {
		isNegative = reader.ReadBit()

		if intVal == 1 {
			intVal = int(reader.ReadInt(coordIntegerBits) + 1)
		}

		if fractVal == 1 {
			fractVal = int(reader.ReadInt(coordFractionalBitsMp))
		}

		res = float32(intVal) + (float32(fractVal) * coordResolution)
	}

	if isNegative {
		res *= -1
	}

	return res
}

func (propertyDecoder) readBitCoordMp(reader *bit.BitReader, isIntegral bool, isLowPrecision bool) float32 {
	var (
		res        float32
		isNegative = false
	)

	inBounds := reader.ReadBit()

	if isIntegral {
		if reader.ReadBit() {
			isNegative = reader.ReadBit()

			if inBounds {
				res = float32(reader.ReadInt(coordIntegerBitsMp) + 1)
			} else {
				res = float32(reader.ReadInt(coordIntegerBits) + 1)
			}
		}
	} else {
		readIntVal := reader.ReadBit()
		isNegative = reader.ReadBit()

		var intVal int
		if readIntVal {
			if inBounds {
				intVal = int(reader.ReadInt(coordIntegerBitsMp)) + 1
			} else {
				intVal = int(reader.ReadInt(coordIntegerBits)) + 1
			}
		}
		if isLowPrecision {
			res = float32(intVal) + (float32(reader.ReadInt(coordFractionalBitsMpLowPrecision)) * coordResolutionLowPrecision)
		} else {
			res = float32(intVal) + (float32(reader.ReadInt(coordFractionalBitsMp)) * coordResolution)
		}
	}

	if isNegative {
		res *= -1
	}

	return res
}

func (propertyDecoder) readBitNormal(reader *bit.BitReader) float32 {
	isNegative := reader.ReadBit()

	fractVal := reader.ReadInt(normalFractBits)

	res := float32(fractVal) * normalResolution

	if isNegative {
		res *= -1
	}

	return res
}

func (propertyDecoder) readBitCellCoord(reader *bit.BitReader, bits int, isIntegral bool, isLowPrecision bool) float32 {
	var (
		intVal, fractVal int
		res              float32
	)

	if isIntegral {
		res = float32(reader.ReadInt(bits))
	} else {
		intVal = int(reader.ReadInt(bits))
		if isLowPrecision {
			fractVal = int(reader.ReadInt(coordFractionalBitsMpLowPrecision))

			res = float32(intVal) + (float32(fractVal) * (coordResolutionLowPrecision))
		} else {
			fractVal = int(reader.ReadInt(coordFractionalBitsMp))

			res = float32(intVal) + (float32(fractVal) * (coordResolution))
		}
	}

	return res
}

func (propertyDecoder) decodeVector(prop *sendTableProperty, reader *bit.BitReader) r3.Vector {
	res := r3.Vector{
		X: float64(propDecoder.decodeFloat(prop, reader)),
		Y: float64(propDecoder.decodeFloat(prop, reader)),
	}

	if !prop.flags.hasFlagSet(propFlagNormal) {
		res.Z = float64(propDecoder.decodeFloat(prop, reader))
	} else {
		absolute := res.X*res.X + res.Y*res.Y
		if absolute < 1 {
			res.Z = math.Sqrt(1 - absolute)
		} else {
			res.Z = 0
		}

		if reader.ReadBit() {
			res.Z *= -1
		}
	}

	return res
}

func (propertyDecoder) decodeArray(fProp *flattenedPropEntry, reader *bit.BitReader) []PropertyValue {
	numBits := int(math.Floor(math.Log2(float64(fProp.prop.numberOfElements))) + 1)

	res := make([]PropertyValue, int(reader.ReadInt(numBits)))

	tmp := &property{
		entry: &flattenedPropEntry{prop: fProp.arrayElementProp},
	}

	for i := range res {
		propDecoder.decodeProp(tmp, reader)
		res[i] = tmp.value
	}

	return res
}

func (propertyDecoder) decodeString(reader *bit.BitReader) string {
	length := int(reader.ReadInt(dataTableMaxStringBits))
	if length > dataTableMaxStringLength {
		length = dataTableMaxStringLength
	}

	return reader.ReadCString(length)
}

func (propertyDecoder) decodeVectorXY(prop *sendTableProperty, reader *bit.BitReader) r3.Vector {
	return r3.Vector{
		X: float64(propDecoder.decodeFloat(prop, reader)),
		Y: float64(propDecoder.decodeFloat(prop, reader)),
	}
}
