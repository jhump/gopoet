package gopoet_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/jhump/gopoet"
)

func TestEndToEnd(t *testing.T) {
	fooState := gopoet.NewTypeSpec("FooState", gopoet.Int64Type)
	foo := gopoet.NewConst("FOO").Initialize("%s(%d)", fooState, 101)
	bar := gopoet.NewConst("BAR").Initialize("%s(%d)", fooState, 202)
	vfoo := gopoet.NewVar("vfoo").SetType(gopoet.StringType)
	vmap := gopoet.NewVar("vmap").SetInitializer(gopoet.
		Printlnf("map[string]%s{", fooState).
		Printlnf("    %q: %s,", "foo", foo).
		Printlnf("    %q: %s,", "bar", bar).
		Println("}"))

	f1 := gopoet.NewGoFile("basic.go", "foo.bar/baz", "baz").
		AddType(fooState).
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
			Printlnf("return %q", "foo"))

	err := gopoet.WriteGoFile(os.Stdout, f1)
	if err != nil {
		if fe, ok := err.(*gopoet.FormatError); ok {
			t.Errorf("unexpected formatter error: %v\n%s", err, string(fe.Unformatted))
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
}
