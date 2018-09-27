package gopoet

import (
	"reflect"
	"testing"
)

func TestImportPackages(t *testing.T) {
	t.Run("RegisterImport", func(t *testing.T) {
		doRegisterImport(t, (*Imports).RegisterImport)
	})
	t.Run("RegisterImportForPackage", func(t *testing.T) {
		doRegisterImport(t, func(imp *Imports, pkgPath, name string) string {
			return imp.RegisterImportForPackage(Package{Name: name, ImportPath: pkgPath})
		})
	})
}

func doRegisterImport(t *testing.T, fn func(imp *Imports, pkgPath, name string) string) {
	checkPrefix := func(actual, expected string) {
		if actual != expected {
			t.Errorf("wrong import prefix: expected %q, got %q", expected, actual)
		}
	}

	imp := NewImportsFor("foo.bar/baz")

	// no conflict
	p := fn(imp, "foo.bar/fizzbuzz", "fizzbuzz")
	checkPrefix(p, "fizzbuzz.")
	p = fn(imp, "foo.bar/fubar", "fubar")
	checkPrefix(p, "fubar.")

	// repeated register returns same prefix
	p = fn(imp, "foo.bar/fizzbuzz", "fizzbuzz")
	checkPrefix(p, "fizzbuzz.")
	p = fn(imp, "foo.bar/fubar", "fubar")
	checkPrefix(p, "fubar.")

	// self import returns empty prefix
	p = fn(imp, "foo.bar/baz", "baz")
	checkPrefix(p, "")

	// conflicts
	p = fn(imp, "foo.bar.2/fubar", "fubar")
	checkPrefix(p, "fubar1.")
	p = fn(imp, "foo.bar.3/fubar", "fubar")
	checkPrefix(p, "fubar2.")
	p = fn(imp, "foo.bar.2/fizzbuzz", "fizzbuzz")
	checkPrefix(p, "fizzbuzz1.")
	p = fn(imp, "foo.bar.3/fizzbuzz", "fizzbuzz")
	checkPrefix(p, "fizzbuzz2.")

	// name doesn't match last path element
	p = fn(imp, "foo.bar.4/fubar", "fubar_v4")
	checkPrefix(p, "fubar_v4.")

	// unknown name will use last path element and assume it's an alias
	p = fn(imp, "foo.bar/fuzzywuzzy", "")
	checkPrefix(p, "fuzzywuzzy.")
	// one that conflicts
	p = fn(imp, "foo.bar.5/fubar", "")
	checkPrefix(p, "fubar3.")

	// query via PrefixForPackage
	p = imp.PrefixForPackage("foo.bar/fizzbuzz")
	checkPrefix(p, "fizzbuzz.")
	p = imp.PrefixForPackage("foo.bar/fubar")
	checkPrefix(p, "fubar.")
	p = imp.PrefixForPackage("foo.bar/baz")
	checkPrefix(p, "")
	p = imp.PrefixForPackage("foo.bar.2/fubar")
	checkPrefix(p, "fubar1.")
	p = imp.PrefixForPackage("foo.bar.3/fubar")
	checkPrefix(p, "fubar2.")
	p = imp.PrefixForPackage("foo.bar.2/fizzbuzz")
	checkPrefix(p, "fizzbuzz1.")
	p = imp.PrefixForPackage("foo.bar.3/fizzbuzz")
	checkPrefix(p, "fizzbuzz2.")
	p = imp.PrefixForPackage("foo.bar.4/fubar")
	checkPrefix(p, "fubar_v4.")
	p = imp.PrefixForPackage("foo.bar/fuzzywuzzy")
	checkPrefix(p, "fuzzywuzzy.")
	p = imp.PrefixForPackage("foo.bar.5/fubar")
	checkPrefix(p, "fubar3.")
	expectToPanic(t, func() {
		imp.PrefixForPackage("something/never/imported")
	})

	// check which will use aliases in an import statement
	// as well as that they are properly sorted
	specs := imp.ImportSpecs()
	expected := []ImportSpec{
		{ImportPath: "foo.bar.2/fizzbuzz", PackageAlias: "fizzbuzz1"},
		{ImportPath: "foo.bar.2/fubar", PackageAlias: "fubar1"},
		{ImportPath: "foo.bar.3/fizzbuzz", PackageAlias: "fizzbuzz2"},
		{ImportPath: "foo.bar.3/fubar", PackageAlias: "fubar2"},
		{ImportPath: "foo.bar.4/fubar"},
		{ImportPath: "foo.bar.5/fubar", PackageAlias: "fubar3"},
		{ImportPath: "foo.bar/fizzbuzz"},
		{ImportPath: "foo.bar/fubar"},
		// alias since actual package name was unknown:
		{ImportPath: "foo.bar/fuzzywuzzy", PackageAlias: "fuzzywuzzy"},
	}
	if !reflect.DeepEqual(specs, expected) {
		t.Errorf("unexpected import specs\nExpected:\n%v\nActual:\n%v", expected, specs)
	}
}

func expectToPanic(t *testing.T, fn func()) {
	defer func() {
		p := recover()
		if p == nil {
			t.Error("expected panic but nothing recovered")
		}
	}()
	fn()
}

// TODO: tests for symbol and typename importing/re-writing
