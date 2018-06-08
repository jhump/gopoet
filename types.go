package gopoet

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"

	"go/types"
)

// TypeKind is an enumeration of the allowed categories of TypeNames.
type TypeKind int

const (
	// KindInvalid is not a valid kind. A valid TypeName will return a non-zero
	// kind.
	KindInvalid TypeKind = iota
	// KindNamed indicates a named type. The only information available in the
	// TypeName is the package and name of the type, not its underlying type.
	// The type's Symbol() method can be used to get the details of the name.
	KindNamed
	// KindBasic indicates an unnamed basic type (i.e. primitive). The type's
	// BasicKind() method can be used to determine which basic type it is.
	//
	// Note that though the "go/types" package considers an unsafe.Pointer to be
	// a basic type, this package does not. An unsafe.Pointer will instead be
	// represented by a TypeName that is a named type whose symbol indicates
	// "unsafe" as the package and "Pointer" as the element name.
	KindBasic
	// KindPtr indicates is a pointer. The type's Elem() method can be used to
	// determine the type to which it points.
	KindPtr
	// KindSlice indicates a slice. The type's Elem() method can be used to
	// determine the slice's element type.
	KindSlice
	// KindArray indicates an array. The type's Elem() and Len() methods can be
	// used to determine array's element type and length, respectively.
	KindArray
	// KindMap indicates a map. The type's Key() and Elem() methods can be used
	// to determine the map's key and value types, respectively.
	KindMap
	// KindChan indicates a chan. The type's Elem() and Dir() methods can be
	// used to determine the element type and channel direction, respectively.
	KindChan
	// KindFunc indicates a func. The type's Signature() method can be used to
	// inspect the function's signature, including argument and result types.
	KindFunc
	// KindStruct indicates a struct type. The type's Fields() method can be
	// used to interrogate details about the struct. This kind is only returned
	// for type names that represent unnamed struct types.
	KindStruct
	// KindInterface indicates an interface type. The type's Methods() and
	// Embeds() methods can be used to determine the explicit methods and
	// the embedded interfaces, respectively. This kind is only returned for
	// type names that represent unnamed interface types.
	KindInterface
)

// TypeName represents a Go type, nominally. It is suitable for rendering a type
// use in source. However, it is not sufficient for doing type analysis or for
// defining a type as named types only refer to their package and name, not the
// underlying type.
type TypeName interface {
	fmt.Stringer

	// Kind returns the kind of type this instance represents.
	Kind() TypeKind
	// Symbol returns the symbol that this named type represents, or nil if
	// this type name does not represent a named type.
	Symbol() *Symbol
	// Elem returns this instance's element type. This is applicable only to
	// pointer, slice, array, map, and channel type names. For map type names,
	// the element type is the type of values in the map. For type names that
	// are not applicable, it returns nil
	Elem() TypeName
	// Key returns this instance's key type, or nil if this type name does not
	// represent a map type.
	Key() TypeName
	// Len returns the size of this array type, or -1 if this type name does
	// not represent an array.
	Len() int64
	// Dir returns the direction of this channel type, or zero if this type
	// does not represent an array.
	Dir() reflect.ChanDir
	// BasicKind returns the kind of scalar/basic type this instance represents,
	// or reflect.Invalid if this type name does not represent a basic type.
	BasicKind() reflect.Kind
	// Signature returns the signature of this function or nil if this type
	// name does represent a function type.
	Signature() *Signature
	// Fields returns the fields of this struct type or nil if this type name
	// does not represent a struct type. Named struct types will return a kind
	// of KindNamed and nil from this method; only unnamed struct types will
	// contain field information.
	Fields() []FieldType
	// Methods returns the explicit methods of this interface type or nil if
	// this type name does not represent an interface type. Named interface
	// types will return a kind of KindNamed and nil from this method; only
	// unnamed interface types will contain method information.
	Methods() []MethodType
	// Embeds returns the symbols for the named interfaces that this interface
	// type embeds or nil if this type name does not represent an interface
	// type. Named interface types will return a kind of KindNamed and nil from
	// this method; only unnamed interface types will contain embedded interface
	// information.
	Embeds() []*Symbol

	// isTypeNameOrSpec is a marker method for places that accept either a
	// TypeName or a *TypeSpec.
	isTypeNameOrSpec()
}

// Signature represents a function signature, which includes the types of the
// arguments and return values. If the last argument is a slice, the function
// might also be variadic or not.
//
// Signatures do not carry receiver information for methods. In the case of
// MethodType instances, the receiver is implied by the enclosing TypeName. For
// FuncSpec instances, the receiver is a separate field of the FuncSpec,
// distinct from the signature.
type Signature struct {
	Args       []ArgType
	Results    []ArgType
	IsVariadic bool
}

// AddArg adds an argument to the signature. The given name can be blank for
// signatures with unnamed arguments. If the signature has a mix of named and
// unnamed args, they will all be rendered as named, but "_" will be used for
// unnamed ones. This method returns the signature, for method chaining.
func (s *Signature) AddArg(name string, t TypeName) *Signature {
	s.Args = append(s.Args, ArgType{Name: name, Type: t})
	return s
}

// AddResult adds a result value to the signature. The given name can be blank
// for signatures with unnamed results. If the signature has a mix of named and
// unnamed results, they will all be rendered as named, but "_" will be used for
// unnamed ones. This method returns the signature, for method chaining.
func (s *Signature) AddResult(name string, t TypeName) *Signature {
	s.Results = append(s.Results, ArgType{Name: name, Type: t})
	return s
}

// SetVariadic sets whether this signature represents a variadic function or
// not. The resulting signature will be invalid if variadic is set to true but
// the last argument is not a slice. This method returns the signature, for
// method chaining.
func (s *Signature) SetVariadic(isVariadic bool) *Signature {
	s.IsVariadic = isVariadic
	return s
}

// ArgType represents the name and type of a function argument or result value.
// The name is optional, but the type is not. See Signature.
type ArgType struct {
	Name string
	Type TypeName
}

// FieldType represents a field in a TypeName whose kind is KindStruct. It
// details the field's name and type as well as an optional struct tag.
type FieldType struct {
	Name string
	Type TypeName
	Tag  reflect.StructTag
}

// MethodType represents a method in a TypeName whose kind is KindInterface. It
// details the method's name and signature.
type MethodType struct {
	Name      string
	Signature Signature
}

type basicTypeName reflect.Kind

var _ TypeName = basicTypeName(0)

func (t basicTypeName) Kind() TypeKind          { return KindBasic }
func (t basicTypeName) Symbol() *Symbol         { return nil }
func (t basicTypeName) Elem() TypeName          { return nil }
func (t basicTypeName) Key() TypeName           { return nil }
func (t basicTypeName) Len() int64              { return -1 }
func (t basicTypeName) Dir() reflect.ChanDir    { return 0 }
func (t basicTypeName) BasicKind() reflect.Kind { return reflect.Kind(t) }
func (t basicTypeName) Signature() *Signature   { return nil }
func (t basicTypeName) Fields() []FieldType     { return nil }
func (t basicTypeName) Methods() []MethodType   { return nil }
func (t basicTypeName) Embeds() []*Symbol       { return nil }
func (t basicTypeName) String() string          { return typeNameToString(t) }
func (t basicTypeName) isTypeNameOrSpec()       {}

type namedTypeName Symbol

var _ TypeName = &namedTypeName{}

func (t *namedTypeName) Kind() TypeKind          { return KindNamed }
func (t *namedTypeName) Symbol() *Symbol         { return (*Symbol)(t) }
func (t *namedTypeName) Elem() TypeName          { return nil }
func (t *namedTypeName) Key() TypeName           { return nil }
func (t *namedTypeName) Len() int64              { return -1 }
func (t *namedTypeName) Dir() reflect.ChanDir    { return 0 }
func (t *namedTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t *namedTypeName) Signature() *Signature   { return nil }
func (t *namedTypeName) Fields() []FieldType     { return nil }
func (t *namedTypeName) Methods() []MethodType   { return nil }
func (t *namedTypeName) Embeds() []*Symbol       { return nil }
func (t *namedTypeName) String() string          { return typeNameToString(t) }
func (t *namedTypeName) isTypeNameOrSpec()       {}

type ptrTypeName struct {
	elem TypeName
}

var _ TypeName = ptrTypeName{}

func (t ptrTypeName) Kind() TypeKind          { return KindPtr }
func (t ptrTypeName) Symbol() *Symbol         { return nil }
func (t ptrTypeName) Elem() TypeName          { return t.elem }
func (t ptrTypeName) Key() TypeName           { return nil }
func (t ptrTypeName) Len() int64              { return -1 }
func (t ptrTypeName) Dir() reflect.ChanDir    { return 0 }
func (t ptrTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t ptrTypeName) Signature() *Signature   { return nil }
func (t ptrTypeName) Fields() []FieldType     { return nil }
func (t ptrTypeName) Methods() []MethodType   { return nil }
func (t ptrTypeName) Embeds() []*Symbol       { return nil }
func (t ptrTypeName) String() string          { return typeNameToString(t) }
func (t ptrTypeName) isTypeNameOrSpec()       {}

type sliceTypeName struct {
	elem TypeName
}

var _ TypeName = sliceTypeName{}

func (t sliceTypeName) Kind() TypeKind          { return KindSlice }
func (t sliceTypeName) Symbol() *Symbol         { return nil }
func (t sliceTypeName) Elem() TypeName          { return t.elem }
func (t sliceTypeName) Key() TypeName           { return nil }
func (t sliceTypeName) Len() int64              { return -1 }
func (t sliceTypeName) Dir() reflect.ChanDir    { return 0 }
func (t sliceTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t sliceTypeName) Signature() *Signature   { return nil }
func (t sliceTypeName) Fields() []FieldType     { return nil }
func (t sliceTypeName) Methods() []MethodType   { return nil }
func (t sliceTypeName) Embeds() []*Symbol       { return nil }
func (t sliceTypeName) String() string          { return typeNameToString(t) }
func (t sliceTypeName) isTypeNameOrSpec()       {}

type arrayTypeName struct {
	elem   TypeName
	length int64
}

var _ TypeName = arrayTypeName{}

func (t arrayTypeName) Kind() TypeKind          { return KindArray }
func (t arrayTypeName) Symbol() *Symbol         { return nil }
func (t arrayTypeName) Elem() TypeName          { return t.elem }
func (t arrayTypeName) Key() TypeName           { return nil }
func (t arrayTypeName) Len() int64              { return t.length }
func (t arrayTypeName) Dir() reflect.ChanDir    { return 0 }
func (t arrayTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t arrayTypeName) Signature() *Signature   { return nil }
func (t arrayTypeName) Fields() []FieldType     { return nil }
func (t arrayTypeName) Methods() []MethodType   { return nil }
func (t arrayTypeName) Embeds() []*Symbol       { return nil }
func (t arrayTypeName) String() string          { return typeNameToString(t) }
func (t arrayTypeName) isTypeNameOrSpec()       {}

type mapTypeName struct {
	elem, key TypeName
}

var _ TypeName = mapTypeName{}

func (t mapTypeName) Kind() TypeKind          { return KindMap }
func (t mapTypeName) Symbol() *Symbol         { return nil }
func (t mapTypeName) Elem() TypeName          { return t.elem }
func (t mapTypeName) Key() TypeName           { return t.key }
func (t mapTypeName) Len() int64              { return -1 }
func (t mapTypeName) Dir() reflect.ChanDir    { return 0 }
func (t mapTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t mapTypeName) Signature() *Signature   { return nil }
func (t mapTypeName) Fields() []FieldType     { return nil }
func (t mapTypeName) Methods() []MethodType   { return nil }
func (t mapTypeName) Embeds() []*Symbol       { return nil }
func (t mapTypeName) String() string          { return typeNameToString(t) }
func (t mapTypeName) isTypeNameOrSpec()       {}

type chanTypeName struct {
	elem TypeName
	dir  reflect.ChanDir
}

var _ TypeName = chanTypeName{}

func (t chanTypeName) Kind() TypeKind          { return KindChan }
func (t chanTypeName) Symbol() *Symbol         { return nil }
func (t chanTypeName) Elem() TypeName          { return t.elem }
func (t chanTypeName) Key() TypeName           { return nil }
func (t chanTypeName) Len() int64              { return -1 }
func (t chanTypeName) Dir() reflect.ChanDir    { return t.dir }
func (t chanTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t chanTypeName) Signature() *Signature   { return nil }
func (t chanTypeName) Fields() []FieldType     { return nil }
func (t chanTypeName) Methods() []MethodType   { return nil }
func (t chanTypeName) Embeds() []*Symbol       { return nil }
func (t chanTypeName) String() string          { return typeNameToString(t) }
func (t chanTypeName) isTypeNameOrSpec()       {}

type funcTypeName Signature

var _ TypeName = &funcTypeName{}

func (t *funcTypeName) Kind() TypeKind          { return KindFunc }
func (t *funcTypeName) Symbol() *Symbol         { return nil }
func (t *funcTypeName) Elem() TypeName          { return nil }
func (t *funcTypeName) Key() TypeName           { return nil }
func (t *funcTypeName) Len() int64              { return -1 }
func (t *funcTypeName) Dir() reflect.ChanDir    { return 0 }
func (t *funcTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t *funcTypeName) Signature() *Signature   { return (*Signature)(t) }
func (t *funcTypeName) Fields() []FieldType     { return nil }
func (t *funcTypeName) Methods() []MethodType   { return nil }
func (t *funcTypeName) Embeds() []*Symbol       { return nil }
func (t *funcTypeName) String() string          { return typeNameToString(t) }
func (t *funcTypeName) isTypeNameOrSpec()       {}

type structTypeName struct {
	fields []FieldType
}

var _ TypeName = &structTypeName{}

func (t *structTypeName) Kind() TypeKind          { return KindStruct }
func (t *structTypeName) Symbol() *Symbol         { return nil }
func (t *structTypeName) Elem() TypeName          { return nil }
func (t *structTypeName) Key() TypeName           { return nil }
func (t *structTypeName) Len() int64              { return -1 }
func (t *structTypeName) Dir() reflect.ChanDir    { return 0 }
func (t *structTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t *structTypeName) Signature() *Signature   { return nil }
func (t *structTypeName) Fields() []FieldType     { return append([]FieldType{}, t.fields...) }
func (t *structTypeName) Methods() []MethodType   { return nil }
func (t *structTypeName) Embeds() []*Symbol       { return nil }
func (t *structTypeName) String() string          { return typeNameToString(t) }
func (t *structTypeName) isTypeNameOrSpec()       {}

type interfaceTypeName struct {
	methods []MethodType
	embeds  []*Symbol
}

var _ TypeName = &interfaceTypeName{}

func (t *interfaceTypeName) Kind() TypeKind          { return KindStruct }
func (t *interfaceTypeName) Symbol() *Symbol         { return nil }
func (t *interfaceTypeName) Elem() TypeName          { return nil }
func (t *interfaceTypeName) Key() TypeName           { return nil }
func (t *interfaceTypeName) Len() int64              { return -1 }
func (t *interfaceTypeName) Dir() reflect.ChanDir    { return 0 }
func (t *interfaceTypeName) BasicKind() reflect.Kind { return reflect.Invalid }
func (t *interfaceTypeName) Signature() *Signature   { return nil }
func (t *interfaceTypeName) Fields() []FieldType     { return nil }
func (t *interfaceTypeName) Methods() []MethodType   { return append([]MethodType{}, t.methods...) }
func (t *interfaceTypeName) Embeds() []*Symbol       { return append([]*Symbol{}, t.embeds...) }
func (t *interfaceTypeName) String() string          { return typeNameToString(t) }
func (t *interfaceTypeName) isTypeNameOrSpec()       {}

var (
	basicTypes = map[reflect.Kind]struct{}{
		reflect.Bool:       {},
		reflect.String:     {},
		reflect.Int:        {},
		reflect.Int8:       {},
		reflect.Int16:      {},
		reflect.Int32:      {},
		reflect.Int64:      {},
		reflect.Uint:       {},
		reflect.Uint8:      {},
		reflect.Uint16:     {},
		reflect.Uint32:     {},
		reflect.Uint64:     {},
		reflect.Uintptr:    {},
		reflect.Float32:    {},
		reflect.Float64:    {},
		reflect.Complex64:  {},
		reflect.Complex128: {},
	}
)

// TypeNameForGoType converts the given "go/types" type into a TypeName.
func TypeNameForGoType(t types.Type) TypeName {
	switch t := t.(type) {
	case *types.Named:
		obj := t.Obj()
		return NamedType(&Symbol{
			Name:    obj.Name(),
			Package: PackageForGoType(obj.Pkg()),
		})

	case *types.Basic:
		var k reflect.Kind
		switch t.Kind() {
		case types.Bool:
			k = reflect.Bool
		case types.Int:
			k = reflect.Int
		case types.Int8:
			k = reflect.Int8
		case types.Int16:
			k = reflect.Int16
		case types.Int32:
			k = reflect.Int32
		case types.Int64:
			k = reflect.Int64
		case types.Uint:
			k = reflect.Uint
		case types.Uint8:
			k = reflect.Uint8
		case types.Uint16:
			k = reflect.Uint16
		case types.Uint32:
			k = reflect.Uint32
		case types.Uint64:
			k = reflect.Uint64
		case types.Uintptr:
			k = reflect.Uintptr
		case types.Float32:
			k = reflect.Float32
		case types.Float64:
			k = reflect.Float64
		case types.Complex64:
			k = reflect.Complex64
		case types.Complex128:
			k = reflect.Complex128
		case types.String:
			k = reflect.String
		case types.UnsafePointer:
			return UnsafePointerType
		default:
			bt := types.Typ[t.Kind()]
			if bt == nil {
				panic(fmt.Sprintf("basic type %s cannot be represented as TypeName", bt.Name()))
			} else {
				panic(fmt.Sprintf("basic kind %d(?) cannot be represented as TypeName", t.Kind()))
			}
		}
		return BasicType(k)

	case *types.Map:
		return MapType(TypeNameForGoType(t.Key()), TypeNameForGoType(t.Elem()))

	case *types.Slice:
		return SliceType(TypeNameForGoType(t.Elem()))

	case *types.Array:
		return ArrayType(TypeNameForGoType(t.Elem()), t.Len())

	case *types.Pointer:
		return PointerType(TypeNameForGoType(t.Elem()))

	case *types.Chan:
		var dir reflect.ChanDir
		switch t.Dir() {
		case types.SendRecv:
			dir = reflect.BothDir
		case types.SendOnly:
			dir = reflect.SendDir
		case types.RecvOnly:
			dir = reflect.RecvDir
		default:
			panic(fmt.Sprintf("channel direction %d is not valid and cannot be represented by a TypeName", t.Dir()))
		}
		return ChannelType(TypeNameForGoType(t.Elem()), dir)

	case *types.Signature:
		var sig Signature
		signatureFromGoType(t, &sig)
		return FuncTypeFromSig(&sig)

	case *types.Struct:
		fields := make([]FieldType, t.NumFields())
		for i := 0; i < t.NumFields(); i++ {
			f := t.Field(i)
			var name string
			if !f.Anonymous() {
				name = f.Name()
			}
			fields[i] = FieldType{
				Name: name,
				Type: TypeNameForGoType(f.Type()),
				Tag:  reflect.StructTag(t.Tag(i)),
			}
		}
		return StructType(fields...)

	case *types.Interface:
		embeds := make([]*Symbol, t.NumEmbeddeds())
		for i := 0; i < t.NumEmbeddeds(); i++ {
			obj := t.Embedded(i).Obj()
			embeds[i] = &Symbol{
				Name:    obj.Name(),
				Package: PackageForGoType(obj.Pkg()),
			}
		}
		methods := make([]MethodType, t.NumExplicitMethods())
		for i := 0; i < t.NumExplicitMethods(); i++ {
			method := t.ExplicitMethod(i)
			methods[i].Name = method.Name()
			signatureFromGoType(method.Type().(*types.Signature), &methods[i].Signature)
		}
		return InterfaceType(embeds, methods...)

	default:
		panic(fmt.Sprintf("%T cannot be represented as TypeName", t))
	}
}

func signatureFromGoType(t *types.Signature, sig *Signature) {
	sig.Args = argsTypesForGoType(t.Params())
	sig.Results = argsTypesForGoType(t.Results())
	sig.IsVariadic = t.Variadic()
	recv := t.Recv()
	if recv == nil {
		return
	}
	if _, ok := recv.Type().Underlying().(*types.Interface); ok {
		return
	}
	// non-interface receiver type; push as first arg
	recvArg := ArgType{
		Name: recv.Name(),
		Type: TypeNameForGoType(recv.Type()),
	}
	sig.Args = append([]ArgType{recvArg}, sig.Args...)
}

func argsTypesForGoType(t *types.Tuple) []ArgType {
	args := make([]ArgType, t.Len())
	for i := 0; i < t.Len(); i++ {
		args[i] = ArgType{
			Name: t.At(i).Name(),
			Type: TypeNameForGoType(t.At(i).Type()),
		}
	}
	return args
}

// TypeNameForGoType converts the given reflect.Type into a TypeName. Note that
// reflect.Type instances do not carry information about embedded interfaces,
// for unnamed interface types. So the resulting TypeName will contain all
// methods as if they were all explicit, even if some come from embedded
// interfaces in the type's source definition.
func TypeNameForReflectType(t reflect.Type) TypeName {
	if _, ok := basicTypes[t.Kind()]; ok && t.PkgPath() == "" {
		return BasicType(t.Kind())
	}

	if t.Name() != "" {
		return NamedType(&Symbol{
			Name:    t.Name(),
			Package: Package{ImportPath: t.PkgPath()},
		})
	}

	switch t.Kind() {
	case reflect.Ptr:
		return PointerType(TypeNameForReflectType(t.Elem()))

	case reflect.Slice:
		return SliceType(TypeNameForReflectType(t.Elem()))

	case reflect.Array:
		return ArrayType(TypeNameForReflectType(t.Elem()), int64(t.Len()))

	case reflect.Chan:
		return ChannelType(TypeNameForReflectType(t.Elem()), t.ChanDir())

	case reflect.Func:
		var sig Signature
		signatureFromReflectType(t, &sig)
		return FuncTypeFromSig(&sig)

	case reflect.Struct:
		fields := make([]FieldType, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fields[i] = FieldType{
				Name: field.Name,
				Type: TypeNameForReflectType(field.Type),
				Tag:  field.Tag,
			}
		}
		return StructType(fields...)

	case reflect.Interface:
		methods := make([]MethodType, t.NumMethod())
		for i := 0; i < t.NumMethod(); i++ {
			method := t.Method(i)
			methods[i].Name = method.Name
			signatureFromReflectType(method.Type, &methods[i].Signature)
		}
		return InterfaceType(nil, methods...)

	default:
		panic(fmt.Sprintf("unrecognized kind %v", t.Kind()))
	}
}

func signatureFromReflectType(t reflect.Type, sig *Signature) {
	sig.IsVariadic = t.IsVariadic()
	sig.Args = make([]ArgType, t.NumIn())
	for i := 0; i < t.NumIn(); i++ {
		sig.Args[i] = ArgType{
			Type: TypeNameForReflectType(t.In(i)),
		}
	}
	sig.Results = make([]ArgType, t.NumOut())
	for i := 0; i < t.NumOut(); i++ {
		sig.Args[i] = ArgType{
			Type: TypeNameForReflectType(t.Out(i)),
		}
	}
}

// BasicType returns a TypeName for the given basic kind. If the given kind is
// reflect.UnsafePointer, a named type is returned whose symbol represents the
// "unsafe.Pointer" name. Other kinds result in a basic type. If the given kind
// is not valid for a basic type (e.g. map, slice, array, chan, func, struct, or
// interface), this function will panic.
func BasicType(k reflect.Kind) TypeName {
	if k == reflect.UnsafePointer {
		return UnsafePointerType
	}
	if _, ok := basicTypes[k]; ok {
		return basicTypeName(k)
	}
	panic(fmt.Sprintf("kind %v is not a basic type", k))
}

// PointerType returns a TypeName that represents a pointer to the given type.
func PointerType(t TypeName) TypeName {
	if t == nil {
		panic("cannot create a pointer with nil element type")
	}
	return ptrTypeName{elem: t}
}

// SliceType returns a TypeName that represents a slice whose elements are of
// the given type.
func SliceType(t TypeName) TypeName {
	if t == nil {
		panic("cannot create a slice with nil element type")
	}
	return sliceTypeName{elem: t}
}

// ArrayType returns a TypeName that represents an array whose elements are of
// the given type and that has the given length.
func ArrayType(t TypeName, length int64) TypeName {
	if t == nil || length < 0 {
		panic("cannot create an array with nil element type or negative length")
	}
	return arrayTypeName{elem: t, length: length}
}

// MapType returns a TypeName that represents a map whose keys and values are of
// the given types.
func MapType(k, v TypeName) TypeName {
	if k == nil || v == nil {
		panic("cannot create a map with nil key or element type")
	}
	return mapTypeName{key: k, elem: v}
}

// NamedType returns a TypeName that represents a named type where the given
// symbol is the type's qualified name.
func NamedType(sym *Symbol) TypeName {
	if sym == nil {
		panic("cannot create a named type with nil symbol")
	}
	return (*namedTypeName)(sym)
}

// ChannelType returns a TypeName that represents a channel whose elements are
// of the given type and that has the given direction. If a direction of 0 is
// provided, the returned channel will be bi-directional.
func ChannelType(t TypeName, dir reflect.ChanDir) TypeName {
	if dir == 0 {
		dir = reflect.BothDir
	}
	if t == nil || dir&^reflect.BothDir != 0 {
		panic("cannot create a channel type with nil element type or invalid direction")
	}
	return chanTypeName{elem: t, dir: dir}
}

// FuncType returns a TypeName that represents a non-variadic function with the
// given argument and result value types.
func FuncType(args []ArgType, results []ArgType) TypeName {
	return FuncTypeFromSig(&Signature{
		Args:    args,
		Results: results,
	})
}

// FuncTypeVariadic returns a TypeName that represents a variadic function with
// the given argument and result value types. If the last argument type given is
// not a slice, the returned function type is invalid.
func FuncTypeVariadic(args []ArgType, results []ArgType) TypeName {
	return FuncTypeFromSig(&Signature{
		Args:       args,
		Results:    results,
		IsVariadic: true,
	})
}

// FuncTypeFromSig returns a TypeName that represents a function with the given
// signature.
func FuncTypeFromSig(sig *Signature) TypeName {
	for _, t := range sig.Args {
		if t.Type == nil {
			panic("cannot create a function type with a nil argument type")
		}
	}
	for _, t := range sig.Results {
		if t.Type == nil {
			panic("cannot create a function type with a nil result type")
		}
	}
	if sig.IsVariadic && (len(sig.Args) == 0 || sig.Args[len(sig.Args)-1].Type.Kind() != KindSlice) {
		panic("cannot create a function type with variadic signature where last arg is not a slice")
	}
	return (*funcTypeName)(sig)
}

// StructType returns a TypeName that represents an unnamed struct type with the
// given fields.
func StructType(fields ...FieldType) TypeName {
	return &structTypeName{fields: fields}
}

// InterfaceType returns a TypeName that represents an unnamed interface type
// with the given embedded interfaces and explicit methods.
func InterfaceType(embeds []*Symbol, methods ...MethodType) TypeName {
	return &interfaceTypeName{
		methods: methods,
		embeds:  embeds,
	}
}

var (
	IntType           = BasicType(reflect.Int)
	Int8Type          = BasicType(reflect.Int8)
	Int16Type         = BasicType(reflect.Int16)
	Int32Type         = BasicType(reflect.Int32)
	Int64Type         = BasicType(reflect.Int64)
	RuneType          = Int32Type
	UintType          = BasicType(reflect.Uint)
	Uint8Type         = BasicType(reflect.Uint8)
	Uint16Type        = BasicType(reflect.Uint16)
	Uint32Type        = BasicType(reflect.Uint32)
	Uint64Type        = BasicType(reflect.Uint64)
	ByteType          = Uint8Type
	UintptrType       = BasicType(reflect.Uintptr)
	BoolType          = BasicType(reflect.Bool)
	StringType        = BasicType(reflect.String)
	Float32Type       = BasicType(reflect.Float32)
	Float64Type       = BasicType(reflect.Float64)
	Complex64Type     = BasicType(reflect.Complex64)
	Complex128Type    = BasicType(reflect.Complex128)
	ErrorType         = NamedType(&Symbol{Name: "error"})
	UnsafePointerType = NamedType(&Symbol{
		Name:    "Pointer",
		Package: Package{ImportPath: "unsafe", Name: "unsafe"},
	})
)

func typeNameToString(tn TypeName) string {
	switch tn.Kind() {
	case KindBasic:
		if tn.BasicKind() == reflect.Uint8 {
			return "byte"
		} else if tn.BasicKind() == reflect.Int32 {
			return "rune"
		} else {
			return tn.BasicKind().String()
		}
	case KindNamed:
		return tn.Symbol().String()
	default:
		var buf bytes.Buffer
		typeNameToBuffer(tn, &buf)
		return buf.String()
	}
}

func typeNameToBuffer(tn TypeName, b *bytes.Buffer) {
	switch tn.Kind() {
	case KindBasic:
		if tn.BasicKind() == reflect.Uint8 {
			b.WriteString("byte")
		} else if tn.BasicKind() == reflect.Int32 {
			b.WriteString("rune")
		} else {
			b.WriteString(tn.BasicKind().String())
		}
	case KindNamed:
		sym := tn.Symbol()
		if sym.Package.Name != "" {
			b.WriteString(sym.Package.Name)
			b.WriteRune('.')
		}
		b.WriteString(sym.Name)
	case KindMap:
		b.WriteString("map[")
		typeNameToBuffer(tn.Key(), b)
		b.WriteRune(']')
		typeNameToBuffer(tn.Elem(), b)
	case KindSlice:
		b.WriteString("[]")
		typeNameToBuffer(tn.Elem(), b)
	case KindArray:
		b.WriteRune('[')
		fmt.Fprint(b, tn.Len())
		b.WriteRune(']')
		typeNameToBuffer(tn.Elem(), b)
	case KindPtr:
		b.WriteRune('*')
		typeNameToBuffer(tn.Elem(), b)
	case KindChan:
		if tn.Dir() == reflect.RecvDir {
			b.WriteString("<-")
		}
		b.WriteString("chan")
		if tn.Dir() == reflect.SendDir {
			b.WriteString("<-")
		}
		b.WriteRune(' ')
		typeNameToBuffer(tn.Elem(), b)
	case KindFunc:
		b.WriteString("func")
		signatureToBuffer(tn.Signature(), b)
	case KindStruct:
		b.WriteString("struct{")
		for i, fld := range tn.Fields() {
			if i > 0 {
				b.WriteString("; ")
			}
			if fld.Name != "" {
				b.WriteString(fld.Name)
				b.WriteRune(' ')
			}
			typeNameToBuffer(fld.Type, b)
			if fld.Tag != "" {
				b.WriteRune(' ')
				tag := string(fld.Tag)
				if strconv.CanBackquote(tag) {
					b.WriteRune('`')
					b.WriteString(tag)
					b.WriteRune('`')
				} else {
					b.WriteString(strconv.Quote(tag))
				}
			}
		}
		b.WriteRune('}')
	case KindInterface:
		b.WriteString("interface{")
		for i, emb := range tn.Embeds() {
			if i > 0 {
				b.WriteString("; ")
			}
			if emb.Package.Name != "" {
				b.WriteString(emb.Package.Name)
				b.WriteRune('.')
			}
			b.WriteString(emb.Name)
		}
		for i, mtd := range tn.Methods() {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(mtd.Name)
			signatureToBuffer(&mtd.Signature, b)
		}
		b.WriteRune('}')
	}
}

func signatureToBuffer(sig *Signature, b *bytes.Buffer) {
	b.WriteRune('(')
	namedArgs := false
	for _, arg := range sig.Args {
		if arg.Name != "" {
			namedArgs = true
			break
		}
	}
	for i, arg := range sig.Args {
		if i > 0 {
			b.WriteString(", ")
		}
		if namedArgs {
			if arg.Name != "" {
				b.WriteString(arg.Name)
				b.WriteRune(' ')
			} else {
				b.WriteString("_ ")
			}
		}
		if sig.IsVariadic && i == len(sig.Args)-1 {
			b.WriteString("...")
			typeNameToBuffer(arg.Type.Elem(), b)
		} else {
			typeNameToBuffer(arg.Type, b)
		}
	}
	b.WriteRune(')')
	if len(sig.Results) > 0 {
		b.WriteRune(' ')
		namedResults := false
		for _, r := range sig.Results {
			if r.Name != "" {
				namedResults = true
				break
			}
		}
		if len(sig.Results) > 1 || namedResults {
			b.WriteRune('(')
		}
		for i, r := range sig.Results {
			if i > 0 {
				b.WriteString(", ")
			}
			if namedResults {
				if r.Name != "" {
					b.WriteString(r.Name)
					b.WriteRune(' ')
				} else {
					b.WriteString("_ ")
				}
			}
			typeNameToBuffer(r.Type, b)
		}
		if len(sig.Results) > 1 || namedResults {
			b.WriteRune(')')
		}
	}
}
