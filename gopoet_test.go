package gopoet_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jhump/gopoet"
)

// regenerate: If true "golden" outputs are re-generated when the test is run.
// If false, the actual generated code is compared against previously-generated
// outpus in the "test-outputs" sub-folder.
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

	file := gopoet.NewGoFile("basic.go", "foo.bar/baz", "baz").
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
			Printlnf("    return %s(%s(bar, %s))", gopoet.NewSymbol("errors", "New"), gopoet.NewSymbol("strings", "Join"), vfoo).
			Printlnf("}").
			Printlnf("bools := make([]bool, len(bar))").
			Printlnf("for i := range bar {").
			Printlnf("    _, bools[i] = vmap[bar[i]]").
			Printlnf("}").
			Printlnf("ch := make(chan struct{}, 1024)").
			Printlnf("baz(ch, bools...)").
			Printlnf("return nil")).
		AddElement(gopoet.NewMethod(gopoet.NewReceiverForType("s", fooState), "String").
			AddResult("", gopoet.StringType).
			Printlnf("return %q", "foo")).
		AddElement(gopoet.NewMethod(gopoet.NewPointerReceiverForType("d", fooDetails), "String").
			AddResult("", gopoet.StringType).
			Printlnf("return d.Name"))

	verifyOutput(t, file)
}

func TestMoreComplexEndToEnd(t *testing.T) {
	// Comments on all elements. Structs and interfaces created via TypeSpecs (NewStructTypeSpec
	// and NewInterfaceTypeSpec), multiple files and packages.

	/////////////////////////////////
	// foo.bar.2/baz/basic.go
	/////////////////////////////////

	fooState := gopoet.NewTypeSpec("FooState", gopoet.Int64Type).
		SetComment("FooState is the state of a foo")
	foo := gopoet.NewConst("FOO").
		SetComment("The state that is fooey.").
		Initialize("%s(%d)", fooState, 101)
	bar := gopoet.NewConst("BAR").
		SetComment("The state that is barry.").
		Initialize("%s(%d)", fooState, 202)
	vfoo := gopoet.NewVar("vfoo").
		SetType(gopoet.StringType).
		SetComment("Global variable that holds a foo string.")
	vmap := gopoet.NewVar("vmap").
		SetComment("Variable that is a mapping of strings to foo states.").
		SetInitializer(gopoet.
			Printlnf("map[string]%s{", fooState).
			Printlnf("    %q: %s,", "foo", foo).
			Printlnf("    %q: %s,", "bar", bar).
			Println("}"))

	fooDetails := gopoet.NewStructTypeSpec("FooDetails",
		gopoet.NewField("ID", gopoet.Int64Type).
			SetTag(`json:"id"`).
			SetComment("ID is a unique identifier for this foo"),
		gopoet.NewField("Name", gopoet.StringType).
			SetTag(`json:"name"`).
			SetComment("Name is the display name for this foo"),
		gopoet.NewField("Categories", gopoet.SliceType(gopoet.StringType)).
			SetTag(`json:"categories"`).
			SetComment("Categories is a slice of category names"),
	).SetComment("FooDetails provide the gory details for your foo.")

	fooInterface := gopoet.NewInterfaceTypeSpec("IFoo",
		gopoet.NewInterfaceEmbed(gopoet.NewSymbol("fmt", "Stringer")).
			SetComment("All IFoos can print to string."),
		gopoet.NewInterfaceMethod("DoIt").
			AddArg("", gopoet.NamedType(gopoet.NewSymbol("net/http", "RoundTripper"))).
			AddArg("", gopoet.BoolType).
			AddResult("", gopoet.ErrorType).
			SetComment("DoIt massages the foo into producing one or more frobnitz."),
	).SetComment("IFoo is for producing frobnitzes.")

	f1 := gopoet.NewGoFile("basic.go", "foo.bar.2/baz", "baz").
		AddType(fooState).
		AddType(fooDetails).
		AddType(fooInterface).
		AddElement(gopoet.NewConstDecl(foo, bar).
			SetComment("some constants")).
		AddElement(gopoet.NewVarDecl(vfoo, vmap).
			SetComment("some variables")).
		AddElement(gopoet.NewFunc("MakeIt").
			SetComment("MakeIt does a bunch of silly things, possibly returning an error.").
			AddArg("foo", fooState.ToTypeName()).
			AddArg("bar", gopoet.SliceType(gopoet.StringType)).
			AddArg("baz", gopoet.FuncTypeVariadic([]gopoet.ArgType{
				{Type: gopoet.ChannelType(gopoet.StructType(), reflect.BothDir)},
				{Type: gopoet.SliceType(gopoet.BoolType)},
			}, nil)).
			AddResult("", gopoet.ErrorType).
			Printlnf("if foo == %s {", foo).
			Printlnf("    return %s(%s(bar, %s))", gopoet.NewSymbol("errors", "New"), gopoet.NewSymbol("strings", "Join"), vfoo).
			Printlnf("}").
			Printlnf("bools := make([]bool, len(bar))").
			Printlnf("for i := range bar {").
			Printlnf("    _, bools[i] = vmap[bar[i]]").
			Printlnf("}").
			Printlnf("ch := make(chan struct{}, 1024)").
			Printlnf("baz(ch, bools...)").
			Printlnf("return nil")).
		AddElement(gopoet.NewMethod(gopoet.NewReceiverForType("s", fooState), "String").
			SetComment("String implements the fmt.Stringer interface").
			AddResult("", gopoet.StringType).
			Printlnf("return %q", "foo")).
		AddElement(gopoet.NewMethod(gopoet.NewPointerReceiverForType("d", fooDetails), "String").
			SetComment("String implements the fmt.Stringer interface").
			AddResult("", gopoet.StringType).
			Printlnf("return d.Name"))

	// TODO: more packages and files

	/////////////////////////////////
	// foo.bar.2/fizz/foo.go
	/////////////////////////////////

	/////////////////////////////////
	// foo.bar.2/fizz/bar.go
	/////////////////////////////////

	/////////////////////////////////
	// foo.bar.2/buzz/main.go
	/////////////////////////////////

	verifyOutput(t, f1)
}

func verifyOutput(t *testing.T, files ...*gopoet.GoFile) {
	if regenerate {
		err := gopoet.WriteGoFilesToFileSystem("./test-outputs", files...)
		if err != nil {
			if fe, ok := err.(*gopoet.FormatError); ok {
				t.Errorf("unexpected formatter error: %v\n%s", err, string(fe.Unformatted))
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}
		return
	}

	outputs := map[string]*bytes.Buffer{}
	err := gopoet.WriteGoFiles(func(path string) (io.WriteCloser, error) {
		var buf bytes.Buffer
		outputs[path] = &buf
		return nopCloser{w: &buf}, nil
	}, files...)
	if err != nil {
		if fe, ok := err.(*gopoet.FormatError); ok {
			t.Fatalf("unexpected formatter error: %v\n%s", err, string(fe.Unformatted))
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	for path, buf := range outputs {
		golden, err := ioutil.ReadFile(filepath.Join("./test-outputs", path))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := buf.Bytes()
		if !bytes.Equal(golden, actual) {
			t.Errorf("wrong generated output!\nExpected:\n%s\nActual:\n%s", golden, actual)
		}
	}
}

type nopCloser struct {
	w io.Writer
}

func (c nopCloser) Write(p []byte) (n int, err error) {
	return c.w.Write(p)
}

func (c nopCloser) Close() error {
	return nil
}
