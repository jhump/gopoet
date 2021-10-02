package gopoet

import (
	"testing"

	"github.com/dgraph-io/badger/v3"
	"github.com/google/go-cmp/cmp"
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
	t.Helper()
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
	if diff := cmp.Diff(expected, specs); diff != "" {
		t.Errorf("unexpected import specs (-expected, +actual):\n  %s", diff)
	}
}

func TestImportSpecsForFile(t *testing.T) {
	buildFile := func(fileName, packagePath, packageName string, fn func(f *GoFile)) *GoFile {
		f := NewGoFile(fileName, packagePath, packageName)
		fn(f)
		return f
	}
	type ensureImportedExample struct {
		input Symbol
		want  string
	}
	for _, tt := range []struct {
		name    string
		f       *GoFile
		want    []ImportSpec
		symbols []ensureImportedExample
	}{
		{
			name: "simple",
			f: buildFile("a.go", "x/y/z", "z", func(f *GoFile) {
				f.EnsureImported(NewSymbol("x/foo", "Example"))
			}),
			want: []ImportSpec{
				{PackageAlias: "", ImportPath: "x/foo"},
			},
		},
		{
			name: "collision",
			f: buildFile("a.go", "x/y/z", "z", func(f *GoFile) {
				f.EnsureImported(NewSymbol("x/foo", "ExampleX"))
				f.EnsureImported(NewSymbol("y/foo", "ExampleY"))
			}),
			want: []ImportSpec{
				{PackageAlias: "", ImportPath: "x/foo"},
				{PackageAlias: "foo1", ImportPath: "y/foo"},
			},
			symbols: []ensureImportedExample{
				{
					input: NewSymbol("y/foo", "Bar"),
					want:  "foo1.Bar",
				},
			},
		},
		{
			name: "RegisterImportForPackage respects aliases old API",
			f: buildFile("a.go", "x/y/z", "z", func(f *GoFile) {
				f.RegisterImportForPackage(Package{Name: "fooalias", ImportPath: "x/foo"})
			}),
			want: []ImportSpec{
				{PackageAlias: "fooalias", ImportPath: "x/foo"},
			},
		},
		{
			name: "RegisterImportForPackage respects aliases new method",
			f: buildFile("a.go", "x/y/z", "z", func(f *GoFile) {
				f.RegisterAliasedImport("x/foo", "fooalias")
			}),
			want: []ImportSpec{
				{PackageAlias: "fooalias", ImportPath: "x/foo"},
			},
		},
		{
			name: "RegisterImportForPackage consistent with EnsureImported",
			f: buildFile("a.go", "x/y/z", "z", func(f *GoFile) {
				f.RegisterImportForPackage(Package{Name: "fooalias", ImportPath: "x/foo"})
			}),
			want: []ImportSpec{
				{PackageAlias: "fooalias", ImportPath: "x/foo"},
			},
			symbols: []ensureImportedExample{
				{
					input: NewSymbol("x/foo", "Bar"),
					want:  "fooalias.Bar",
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.ImportSpecs()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected diff in ImportSpecs() of file (-want, +got):\n%s", diff)
			}
			for _, ttt := range tt.symbols {
				got := tt.f.EnsureImported(ttt.input)
				if got.String() != ttt.want {
					t.Errorf("EnsureImported(%s) got %q, wanted %q %d", ttt.input, got, ttt.want, badger.BlockCache)
				}
			}
		})
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
