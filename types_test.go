package gopoet

import (
	"reflect"
	"testing"
)

func TestNewType(t *testing.T) {
	t.Run("KindNamed", func(t *testing.T) {
		sym := Symbol{Package: NewPackage("foo/bar"), Name: "Baz"}
		tn := NamedType(sym)
		checkTypeName(t, tn, KindNamed, sym)

		sym = Symbol{Package: NewPackage("unsafe"), Name: "Pointer"}
		tn = UnsafePointerType
		checkTypeName(t, tn, KindNamed, sym)
		tn = BasicType(reflect.UnsafePointer) // special-case: this returns named type
		checkTypeName(t, tn, KindNamed, sym)

		sym = Symbol{Name: "error"}
		tn = ErrorType
		checkTypeName(t, tn, KindNamed, sym)
	})

	t.Run("KindBasic", func(t *testing.T) {
		basicKinds := map[reflect.Kind]TypeName{
			reflect.Int:        IntType,
			reflect.Int8:       Int8Type,
			reflect.Int16:      Int16Type,
			reflect.Int32:      Int32Type,
			reflect.Int64:      Int64Type,
			reflect.Uint:       UintType,
			reflect.Uint8:      Uint8Type,
			reflect.Uint16:     Uint16Type,
			reflect.Uint32:     Uint32Type,
			reflect.Uint64:     Uint64Type,
			reflect.Float32:    Float32Type,
			reflect.Float64:    Float64Type,
			reflect.Complex64:  Complex64Type,
			reflect.Complex128: Complex128Type,
			reflect.Bool:       BoolType,
			reflect.String:     StringType,
			reflect.Uintptr:    UintptrType,
		}
		for bk, tn := range basicKinds {
			checkTypeName(t, tn, KindBasic, bk)
			tn = BasicType(bk)
			checkTypeName(t, tn, KindBasic, bk)
		}
	})

	elems := []TypeName{BasicType(reflect.Int), ErrorType, UnsafePointerType, SliceType(BasicType(reflect.String))}

	t.Run("KindSlice", func(t *testing.T) {
		for _, e := range elems {
			tn := SliceType(e)
			checkTypeName(t, tn, KindSlice, e)
		}
	})

	t.Run("KindPtr", func(t *testing.T) {
		for _, e := range elems {
			tn := PointerType(e)
			checkTypeName(t, tn, KindPtr, e)
		}
	})

	t.Run("KindArray", func(t *testing.T) {
		for _, e := range elems {
			for i := 1; i <= 10000; i *= 10 {
				tn := ArrayType(e, int64(i))
				checkTypeName(t, tn, KindArray, []interface{}{i, e})
			}
		}
	})

	t.Run("KindMap", func(t *testing.T) {
		for _, k := range elems {
			for _, e := range elems {
				tn := MapType(k, e)
				checkTypeName(t, tn, KindMap, []interface{}{k, e})
			}
		}
	})

	t.Run("KindChan", func(t *testing.T) {
		for _, e := range elems {
			for _, dir := range []reflect.ChanDir{reflect.SendDir, reflect.RecvDir, reflect.BothDir} {
				tn := ChannelType(e, dir)
				checkTypeName(t, tn, KindChan, []interface{}{dir, e})
			}
		}
	})

	t.Run("KindStruct", func(t *testing.T) {
		tn := StructType()
		checkTypeName(t, tn, KindStruct, []FieldType{})

		fields := []FieldType{
			{Name: "Foo", Type: IntType},
			{Name: "bar", Type: StringType},
			{Name: "baz", Type: SliceType(ByteType)},
		}
		tn = StructType(fields...)
		checkTypeName(t, tn, KindStruct, fields)
	})

	t.Run("KindInterface", func(t *testing.T) {
		tn := InterfaceType(nil)
		checkTypeName(t, tn, KindInterface, []interface{}{[]MethodType{}, []Symbol{}})

		methods := []MethodType{
			{Name: "Foo", Signature: Signature{
				Args: []ArgType{
					{Name: "a", Type: IntType},
					{Name: "b", Type: StringType},
				},
			}},
			{Name: "Bar", Signature: Signature{
				Results: []ArgType{{Type: ErrorType}},
			}},
		}
		tn = InterfaceType(nil, methods...)
		checkTypeName(t, tn, KindInterface, []interface{}{methods, []Symbol{}})

		embeds := []Symbol{
			{Name: "Stringer", Package: NewPackage("fmt")},
			{Name: "Reader", Package: NewPackage("io")},
		}
		tn = InterfaceType(embeds)
		checkTypeName(t, tn, KindInterface, []interface{}{[]MethodType{}, embeds})

		tn = InterfaceType(embeds, methods...)
		checkTypeName(t, tn, KindInterface, []interface{}{methods, embeds})
	})

	t.Run("KindFunc", func(t *testing.T) {
		args := []ArgType{
			{Name: "a", Type: IntType},
			{Name: "b", Type: StringType},
		}
		rets := []ArgType{{Type: ErrorType}}
		sig := &Signature{Args: args, Results: rets}

		tn := FuncType(args, rets)
		checkTypeName(t, tn, KindFunc, sig)

		tn = FuncTypeFromSig(sig)
		checkTypeName(t, tn, KindFunc, sig)

		sig.IsVariadic = true
		func() {
			// variadic function not allowed if last argument is not a slice
			defer func() {
				p := recover()
				if p == nil {
					t.Error("expecting a panic due to bad signature but nothing recovered")
				}
			}()
			FuncTypeVariadic(args, rets)
		}()

		args = append(args, ArgType{Type: SliceType(StringType)})
		sig.Args = args

		tn = FuncTypeVariadic(args, rets)
		checkTypeName(t, tn, KindFunc, sig)
	})
}

func TestTypeNameForGoType(t *testing.T) {
	// TODO
}

func TestTypeNameForReflectType(t *testing.T) {
	// TODO
}

func checkTypeName(t *testing.T, tn TypeName, expectedKind TypeKind, val interface{}) {
	if tn.Kind() != expectedKind {
		t.Errorf("TypeName has wrong kind: %v != %v", tn.Kind(), expectedKind)
	}

	var expectedSym Symbol
	if expectedKind == KindNamed {
		expectedSym = val.(Symbol)
	}
	if tn.Symbol() != expectedSym {
		t.Errorf("Symbol() returned wrong value: %v != %v", tn.Symbol(), expectedSym)
	}

	var expectedBasicKind reflect.Kind
	if expectedKind == KindBasic {
		expectedBasicKind = val.(reflect.Kind)
	} else {
		expectedBasicKind = reflect.Invalid
	}
	if tn.BasicKind() != expectedBasicKind {
		t.Errorf("BasicKind() returned wrong value: %v != %v", tn.BasicKind(), expectedBasicKind)
	}

	var expectedElem TypeName
	if expectedKind == KindPtr || expectedKind == KindSlice {
		expectedElem = val.(TypeName)
	} else if expectedKind == KindArray || expectedKind == KindChan || expectedKind == KindMap {
		expectedElem = val.([]interface{})[1].(TypeName)
	}
	if !reflect.DeepEqual(tn.Elem(), expectedElem) {
		t.Errorf("Elem() returned wrong value: %v != %v", tn.Elem(), expectedElem)
	}

	var expectedLen int
	if expectedKind == KindArray {
		expectedLen = val.([]interface{})[0].(int)
	} else {
		expectedLen = -1
	}
	if tn.Len() != int64(expectedLen) {
		t.Errorf("Len() returned wrong value: %v != %v", tn.Len(), expectedLen)
	}

	var expectedKey TypeName
	if expectedKind == KindMap {
		expectedKey = val.([]interface{})[0].(TypeName)
	}
	if tn.Key() != expectedKey {
		t.Errorf("Key() returned wrong value: %v != %v", tn.Key(), expectedKey)
	}

	var expectedDir reflect.ChanDir
	if expectedKind == KindChan {
		expectedDir = val.([]interface{})[0].(reflect.ChanDir)
	}
	if tn.Dir() != expectedDir {
		t.Errorf("Dir() returned wrong value: %v != %v", tn.Dir(), expectedDir)
	}

	var expectedSig *Signature
	if expectedKind == KindFunc {
		expectedSig = val.(*Signature)
	}
	if !reflect.DeepEqual(tn.Signature(), expectedSig) {
		t.Errorf("Signature() returned wrong value: %v != %v", tn.Signature(), expectedSig)
	}

	var expectedFields []FieldType
	if expectedKind == KindStruct {
		expectedFields = val.([]FieldType)
	}
	if !reflect.DeepEqual(tn.Fields(), expectedFields) {
		t.Errorf("Fields() returned wrong value: %v != %v", tn.Signature(), expectedFields)
	}

	var expectedMethods []MethodType
	var expectedEmbeds []Symbol
	if expectedKind == KindInterface {
		v := val.([]interface{})
		expectedMethods = v[0].([]MethodType)
		expectedEmbeds = v[1].([]Symbol)
	}
	if !reflect.DeepEqual(tn.Methods(), expectedMethods) {
		t.Errorf("Methods() returned wrong value: %v != %v", tn.Methods(), expectedMethods)
	}
	if !reflect.DeepEqual(tn.Embeds(), expectedEmbeds) {
		t.Errorf("Embeds() returned wrong value: %v != %v", tn.Embeds(), expectedEmbeds)
	}
}
