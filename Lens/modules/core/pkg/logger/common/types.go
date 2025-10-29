package common

import (
	"context"
	"fmt"
	"go.uber.org/zap/zapcore"
	"math"
	"strconv"
	"time"
)

type ContextHook func(context.Context) []Field

// A FieldType indicates which member of the Field union struct should be used
// and how it should be serialized.
type FieldType uint8

const (
	// UnknownType is the default field type. Attempting to add it to an encoder will panic.
	UnknownType FieldType = iota
	// ArrayMarshalerType indicates that the field carries an ArrayMarshaler.
	ArrayMarshalerType
	// ObjectMarshalerType indicates that the field carries an ObjectMarshaler.
	ObjectMarshalerType
	// BinaryType indicates that the field carries an opaque binary blob.
	BinaryType
	// BoolType indicates that the field carries a bool.
	BoolType
	// ByteStringType indicates that the field carries UTF-8 encoded bytes.
	ByteStringType
	// Complex128Type indicates that the field carries a complex128.
	Complex128Type
	// Complex64Type indicates that the field carries a complex128.
	Complex64Type
	// DurationType indicates that the field carries a time.Duration.
	DurationType
	// Float64Type indicates that the field carries a float64.
	Float64Type
	// Float32Type indicates that the field carries a float32.
	Float32Type
	// Int64Type indicates that the field carries an int64.
	Int64Type
	// Int32Type indicates that the field carries an int32.
	Int32Type
	// Int16Type indicates that the field carries an int16.
	Int16Type
	// Int8Type indicates that the field carries an int8.
	Int8Type
	// StringType indicates that the field carries a string.
	StringType
	// TimeType indicates that the field carries a time.Time.
	TimeType
	// Uint64Type indicates that the field carries a uint64.
	Uint64Type
	// Uint32Type indicates that the field carries a uint32.
	Uint32Type
	// Uint16Type indicates that the field carries a uint16.
	Uint16Type
	// Uint8Type indicates that the field carries a uint8.
	Uint8Type
	// UintptrType indicates that the field carries a uintptr.
	UintptrType
	// ReflectType indicates that the field carries an interface{}, which should
	// be serialized using reflection.
	ReflectType
	// NamespaceType signals the beginning of an isolated namespace. All
	// subsequent fields should be added to the new namespace.
	NamespaceType
	// StringerType indicates that the field carries a fmt.Stringer.
	StringerType
	// ErrorType indicates that the field carries an error.
	ErrorType
	// SkipType indicates that the field is a no-op.
	SkipType
)

// A Field is a marshaling operation used to add a key-value pair to a logger's
// context. Most fields are lazily marshaled, so it's inexpensive to add fields
// to disabled debug-level log statements.
type Field struct {
	Key       string
	Type      FieldType
	Integer   int64
	String    string
	Interface interface{}
}

// Skip constructs a no-op field, which is often useful when handling invalid
// inputs in other Field constructors.
func Skip() Field {
	return Field{Type: SkipType}
}

// Binary constructs a field that carries an opaque binary blob.
//
// Binary data is serialized in an encoding-appropriate format. For example,
// zap's JSON encoder base64-encodes binary blobs. To log UTF-8 encoded text,
// use ByteString.
func Binary(key string, val []byte) Field {
	return Field{Key: key, Type: BinaryType, Interface: val}
}

// Bool constructs a field that carries a bool.
func Bool(key string, val bool) Field {
	var ival int64
	if val {
		ival = 1
	}
	return Field{Key: key, Type: BoolType, Integer: ival}
}

// ByteString constructs a field that carries UTF-8 encoded text as a []byte.
// To log opaque binary blobs (which aren't necessarily valid UTF-8), use
// Binary.
func ByteString(key string, val []byte) Field {
	return Field{Key: key, Type: ByteStringType, Interface: val}
}

// Complex128 constructs a field that carries a complex number. Unlike most
// numeric fields, this costs an allocation (to convert the complex128 to
// interface{}).
func Complex128(key string, val complex128) Field {
	return Field{Key: key, Type: Complex128Type, Interface: val}
}

// Complex64 constructs a field that carries a complex number. Unlike most
// numeric fields, this costs an allocation (to convert the complex64 to
// interface{}).
func Complex64(key string, val complex64) Field {
	return Field{Key: key, Type: Complex64Type, Interface: val}
}

// Float64 constructs a field that carries a float64. The way the
// floating-point value is represented is encoder-dependent, so marshaling is
// necessarily lazy.
func Float64(key string, val float64) Field {
	return Field{Key: key, Type: Float64Type, Integer: int64(math.Float64bits(val))}
}

// Float32 constructs a field that carries a float32. The way the
// floating-point value is represented is encoder-dependent, so marshaling is
// necessarily lazy.
func Float32(key string, val float32) Field {
	return Field{Key: key, Type: Float32Type, Integer: int64(math.Float32bits(val))}
}

// Int constructs a field with the given key and value.
func Int(key string, val int) Field {
	return Int64(key, int64(val))
}

// Int64 constructs a field with the given key and value.
func Int64(key string, val int64) Field {
	return Field{Key: key, Type: Int64Type, Integer: val}
}

// Int32 constructs a field with the given key and value.
func Int32(key string, val int32) Field {
	return Field{Key: key, Type: Int32Type, Integer: int64(val)}
}

// Int16 constructs a field with the given key and value.
func Int16(key string, val int16) Field {
	return Field{Key: key, Type: Int16Type, Integer: int64(val)}
}

// Int8 constructs a field with the given key and value.
func Int8(key string, val int8) Field {
	return Field{Key: key, Type: Int8Type, Integer: int64(val)}
}

// String constructs a field with the given key and value.
func String(key string, val string) Field {
	return Field{Key: key, Type: StringType, String: val}
}

// Uint constructs a field with the given key and value.
func Uint(key string, val uint) Field {
	return Uint64(key, uint64(val))
}

// Uint64 constructs a field with the given key and value.
func Uint64(key string, val uint64) Field {
	return Field{Key: key, Type: Uint64Type, Integer: int64(val)}
}

// Uint32 constructs a field with the given key and value.
func Uint32(key string, val uint32) Field {
	return Field{Key: key, Type: Uint32Type, Integer: int64(val)}
}

// Uint16 constructs a field with the given key and value.
func Uint16(key string, val uint16) Field {
	return Field{Key: key, Type: Uint16Type, Integer: int64(val)}
}

// Uint8 constructs a field with the given key and value.
func Uint8(key string, val uint8) Field {
	return Field{Key: key, Type: Uint8Type, Integer: int64(val)}
}

// Uintptr constructs a field with the given key and value.
func Uintptr(key string, val uintptr) Field {
	return Field{Key: key, Type: UintptrType, Integer: int64(val)}
}

// Reflect constructs a field with the given key and an arbitrary object. It uses
// an encoding-appropriate, reflection-based function to lazily serialize nearly
// any object into the logging context, but it's relatively slow and
// allocation-heavy. Outside tests, Any is always a better choice.
//
// If encoding fails (e.g., trying to serialize a map[int]string to JSON), Reflect
// includes the error message in the final log output.
func Reflect(key string, val interface{}) Field {
	return Field{Key: key, Type: ReflectType, Interface: val}
}

// Namespace creates a named, isolated scope within the logger's context. All
// subsequent fields will be added to the new namespace.
//
// This helps prevent key collisions when injecting loggers into sub-components
// or third-party libraries.
func Namespace(key string) Field {
	return Field{Key: key, Type: NamespaceType}
}

// Stringer constructs a field with the given key and the output of the value's
// String method. The Stringer's String method is called lazily.
func Stringer(key string, val fmt.Stringer) Field {
	return Field{Key: key, Type: StringerType, Interface: val}
}

// Time constructs a Field with the given key and value. The encoder
// controls how the time is serialized.
func Time(key string, val time.Time) Field {
	return Field{Key: key, Type: TimeType, Integer: val.UnixNano(), Interface: val.Location()}
}

// Duration constructs a field with the given key and value. The encoder
// controls how the duration is serialized.
func Duration(key string, val time.Duration) Field {
	return Field{Key: key, Type: DurationType, Integer: int64(val)}
}

// UserID constructs a field with the given user id.
func UserID(val string) Field {
	return Field{Key: "user_id", Type: StringType, String: val}
}

// UserIDInt64
func UserIDInt64(val int64) Field {
	return UserID(strconv.FormatInt(val, 10))
}

// OneID construct a field with the given device one id
func OneID(val string) Field {
	return Field{Key: "oneid", Type: StringType, String: val}
}

// OneIDInt64
func OneIDInt64(val int64) Field {
	return OneID(strconv.FormatInt(val, 10))
}

// ExtIndex constructs a field with the extra custom searchable value
func ExtIndex(val string) Field {
	return Field{Key: "ext_index", Type: StringType, String: val}
}

// Error is shorthand for the common idiom NamedError("error", err).
func ParseError(err error) Field {
	return NamedError("error", err)
}

// NamedError constructs a field that lazily stores err.Error() under the
// provided key. Errors which also implement fmt.Formatter (like those produced
// by github.com/pkg/errors) will also have their verbose representation stored
// under key+"Verbose". If passed a nil error, the field is a no-op.
//
// For the common case in which the key is simply "error", the Error function
// is shorter and less repetitive.
func NamedError(key string, err error) Field {
	if err == nil {
		return Skip()
	}
	return Field{Key: key, Type: ErrorType, Interface: err}
}

// Value gets a field's value.
func Value(f *Field) interface{} {
	switch f.Type {
	case ArrayMarshalerType,
		ObjectMarshalerType,
		BinaryType,
		ByteStringType,
		TimeType,
		ReflectType,
		StringerType,
		ErrorType:
		return f.Interface
	case BoolType,
		Complex128Type,
		Complex64Type,
		DurationType,
		Float64Type,
		Float32Type,
		Int64Type,
		Int32Type,
		Int16Type,
		Int8Type,
		Uint64Type,
		Uint32Type,
		Uint16Type,
		Uint8Type,
		UintptrType:
		return f.Integer
	case StringType:
		return f.String
	default:
		return f.Interface
	}
}

// Any takes a key and an arbitrary value and chooses the best way to represent
// them as a field, falling back to a reflection-based approach only if
// necessary.
//
// Since byte/uint8 and rune/int32 are aliases, Any can't differentiate between
// them. To minimize surprises, []byte values are treated as binary blobs, byte
// values are treated as uint8, and runes are always treated as integers.
func Any(key string, value interface{}) Field {
	switch val := value.(type) {
	case bool:
		return Bool(key, val)
	case []bool:
		return Bools(key, val)
	case complex128:
		return Complex128(key, val)
	case []complex128:
		return Complex128s(key, val)
	case complex64:
		return Complex64(key, val)
	case []complex64:
		return Complex64s(key, val)
	case float64:
		return Float64(key, val)
	case []float64:
		return Float64s(key, val)
	case float32:
		return Float32(key, val)
	case []float32:
		return Float32s(key, val)
	case int:
		return Int(key, val)
	case []int:
		return Ints(key, val)
	case int64:
		return Int64(key, val)
	case []int64:
		return Int64s(key, val)
	case int32:
		return Int32(key, val)
	case []int32:
		return Int32s(key, val)
	case int16:
		return Int16(key, val)
	case []int16:
		return Int16s(key, val)
	case int8:
		return Int8(key, val)
	case []int8:
		return Int8s(key, val)
	case string:
		return String(key, val)
	case []string:
		return Strings(key, val)
	case uint:
		return Uint(key, val)
	case []uint:
		return Uints(key, val)
	case uint64:
		return Uint64(key, val)
	case []uint64:
		return Uint64s(key, val)
	case uint32:
		return Uint32(key, val)
	case []uint32:
		return Uint32s(key, val)
	case uint16:
		return Uint16(key, val)
	case []uint16:
		return Uint16s(key, val)
	case uint8:
		return Uint8(key, val)
	case []byte:
		return Binary(key, val)
	case uintptr:
		return Uintptr(key, val)
	case []uintptr:
		return Uintptrs(key, val)
	case time.Time:
		return Time(key, val)
	case []time.Time:
		return Times(key, val)
	case time.Duration:
		return Duration(key, val)
	case []time.Duration:
		return Durations(key, val)
	case error:
		return NamedError(key, val)
	case fmt.Stringer:
		return Stringer(key, val)
	default:
		return Reflect(key, val)
	}
}

func zapFieldType(ft FieldType) zapcore.FieldType {
	switch ft {
	case UnknownType:
		return zapcore.UnknownType
	case ArrayMarshalerType:
		return zapcore.ArrayMarshalerType
	case ObjectMarshalerType:
		return zapcore.ObjectMarshalerType
	case BinaryType:
		return zapcore.BinaryType
	case BoolType:
		return zapcore.BoolType
	case ByteStringType:
		return zapcore.ByteStringType
	case Complex128Type:
		return zapcore.Complex128Type
	case Complex64Type:
		return zapcore.Complex64Type
	case DurationType:
		return zapcore.DurationType
	case Float64Type:
		return zapcore.Float64Type
	case Float32Type:
		return zapcore.Float32Type
	case Int64Type:
		return zapcore.Int64Type
	case Int32Type:
		return zapcore.Int32Type
	case Int16Type:
		return zapcore.Int16Type
	case Int8Type:
		return zapcore.Int8Type
	case StringType:
		return zapcore.StringType
	case TimeType:
		return zapcore.TimeType
	case Uint64Type:
		return zapcore.Uint64Type
	case Uint32Type:
		return zapcore.Uint32Type
	case Uint16Type:
		return zapcore.Uint16Type
	case Uint8Type:
		return zapcore.Uint8Type
	case UintptrType:
		return zapcore.UintptrType
	case ReflectType:
		return zapcore.ReflectType
	case NamespaceType:
		return zapcore.NamespaceType
	case StringerType:
		return zapcore.StringerType
	case ErrorType:
		return zapcore.ErrorType
	case SkipType:
		return zapcore.SkipType
	default:
		return zapcore.UnknownType
	}
}
