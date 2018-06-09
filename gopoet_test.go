package gopoet_test

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jhump/gopoet"
)

var regenerate = false

func TestSimpleEndToEnd(t *testing.T) {
	// No comments, structs and interfaces created from TypeNames, just one file and package.

	fooState := gopoet.NewTypeSpec("FooState", gopoet.Int64Type)
	foo := gopoet.NewConst("FOO").Initialize("%s(%d)", fooState, 101)
	bar := gopoet.NewConst("BAR").Initialize("%s(%d)", fooState, 202)
	vfoo := gopoet.NewVar("vfoo").SetType(gopoet.StringType)
	vmap := gopoet.NewVar("vmap").SetInitializer(gopoet.
		Printlnf("map[string]%s{", fooState).
		Printlnf("    %q: %s,", "foo", foo).
		Printlnf("    %q: %s,", "bar", bar).
		Println("}"))

	fooDetails := gopoet.NewTypeSpec("FooDetails", gopoet.StructType(
		gopoet.FieldType{Name: "ID", Type: gopoet.Int64Type, Tag: `json:"id"`},
		gopoet.FieldType{Name: "Name", Type: gopoet.StringType, Tag: `json:"name"`},
		gopoet.FieldType{Name: "Categories", Type: gopoet.SliceType(gopoet.StringType), Tag: `json:"categories"`},
	))
	fooInterface := gopoet.NewTypeSpec("IFoo", gopoet.InterfaceType(
		[]gopoet.Symbol{gopoet.NewSymbol("fmt", "Stringer")},
		gopoet.MethodType{Name: "DoIt", Signature: gopoet.Signature{
			Args: []gopoet.ArgType{
				{Type: gopoet.NamedType(gopoet.NewSymbol("net/http", "RoundTripper"))},
				{Type: gopoet.BoolType},
			},
			Results: []gopoet.ArgType{{Type: gopoet.ErrorType}},
		}},
	))

	f1 := gopoet.NewGoFile("basic.go", "foo.bar/baz", "baz").
		AddType(fooState).
		AddType(fooDetails).
		AddType(fooInterface).
		AddElement(gopoet.NewConstDecl(foo, bar)).
		AddElement(gopoet.NewVarDecl(vfoo, vmap)).
		AddElement(gopoet.NewFunc("MakeIt").
			AddArg("foo", fooState.ToTypeName()).
			AddArg("bar", gopoet.SliceType(gopoet.StringType)).
			AddArg("baz", gopoet.FuncTypeVariadic([]gopoet.ArgType{
				{Type: gopoet.ChannelType(gopoet.StructType(), reflect.BothDir)},
				{Type: gopoet.SliceType(gopoet.BoolType)},
			},
				nil)).
			AddResult("", gopoet.ErrorType).
			Printlnf("if foo == %s {", foo).
			Printlnf("    return %s(%s(bar))", gopoet.NewSymbol("errors", "New"), gopoet.NewSymbol("strings", "Join")).
			Printlnf("}")).
		AddElement(gopoet.NewMethod(gopoet.NewReceiverForType("s", fooState), "String").
			AddResult("", gopoet.StringType).
			Printlnf("return %q", "foo")).
		AddElement(gopoet.NewMethod(gopoet.NewPointerReceiverForType("d", fooDetails), "String").
			AddResult("", gopoet.StringType).
			Printlnf("return d.Name"))

	if regenerate {
		err := gopoet.WriteGoFilesToFileSystem("./test-outputs", f1)
		if err != nil {
			if fe, ok := err.(*gopoet.FormatError); ok {
				t.Errorf("unexpected formatter error: %v\n%s", err, string(fe.Unformatted))
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}
		return
	}

	var buf bytes.Buffer
	err := gopoet.WriteGoFile(&buf, f1)
	if err != nil {
		if fe, ok := err.(*gopoet.FormatError); ok {
			t.Fatalf("unexpected formatter error: %v\n%s", err, string(fe.Unformatted))
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	golden, err := ioutil.ReadFile(filepath.Join("./test-outputs", f1.Package().ImportPath, f1.Name))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	actual := buf.Bytes()
	if !bytes.Equal(golden, actual) {
		t.Errorf("wrong generated output!\nExpected:\n%s\nActual:\n%s", golden, actual)
	}
}
