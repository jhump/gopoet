package gopoet

import (
	"fmt"
	"testing"

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
	buildFile := func(packagePath, packageName string, opts []GoFileOption, fn func(f *GoFile)) *GoFile {
		f := NewGoFile("doesntmatter.go", packagePath, packageName, opts...)
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
			f: buildFile("x/y/z", "z", nil, func(f *GoFile) {
				f.EnsureImported(NewSymbol("x/foo", "Example"))
			}),
			want: []ImportSpec{
				{PackageAlias: "", ImportPath: "x/foo"},
			},
		},
		{
			name: "collision",
			f: buildFile("x/y/z", "z", nil, func(f *GoFile) {
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
			name: "CustomPackageNameSuggester 1",
			f: buildFile("x/y/z", "z", []GoFileOption{
				GoFileImportsOption(CustomPackageNameSuggester(
					func(importPath, packageNameInPackageClause string, callback func(packageName string, isAlias bool) (keepGoing bool)) {
						if importPath == "x/foo" {
							callback("fooalias", true)
							return
						}
						panic(fmt.Errorf("unexpected import path %q", importPath))
					})),
			}, func(f *GoFile) {
				f.RegisterImport("x/foo", "")
			}),
			want: []ImportSpec{
				{PackageAlias: "fooalias", ImportPath: "x/foo"},
			},
		},
		{
			name: "CustomPackageNameSuggester 2 - alias",
			f: buildFile("x/y/z", "z", []GoFileOption{
				GoFileImportsOption(CustomPackageNameSuggester(
					func(importPath, packageNameInPackageClause string, callback func(packageName string, isAlias bool) (keepGoing bool)) {
						if importPath == "x/foo" {
							callback("foo", true)
							return
						}
						panic(fmt.Errorf("unexpected import path %q", importPath))
					})),
			}, func(f *GoFile) {
				f.RegisterImport("x/foo", "")
			}),
			want: []ImportSpec{
				{PackageAlias: "foo", ImportPath: "x/foo"},
			},
			symbols: []ensureImportedExample{
				{
					input: NewSymbol("x/foo", "Bar"),
					want:  "foo.Bar",
				},
			},
		},
		{
			name: "CustomPackageNameSuggester 3 - non alias differs from path.Base",
			f: buildFile("x/y/z", "z", []GoFileOption{
				GoFileImportsOption(CustomPackageNameSuggester(
					func(importPath, packageNameInPackageClause string, callback func(packageName string, isAlias bool) (keepGoing bool)) {
						if importPath == "x/foo" {
							callback("foonotalias", false)
							return
						}
						panic(fmt.Errorf("unexpected import path %q", importPath))
					})),
			}, func(f *GoFile) {
				f.RegisterImport("x/foo", "")
			}),
			want: []ImportSpec{
				{PackageAlias: "", ImportPath: "x/foo"},
			},
			symbols: []ensureImportedExample{
				{
					input: NewSymbol("x/foo", "Bar"),
					want:  "foonotalias.Bar",
				},
			},
		},
		{
			name: "RegisterImportForPackage should not be used to specify aliases",
			f: buildFile("x/y/z", "z", nil, func(f *GoFile) {
				f.RegisterImportForPackage(Package{Name: "notreallyanalias", ImportPath: "x/foo"})
			}),
			want: []ImportSpec{
				{PackageAlias: "", ImportPath: "x/foo"},
			},
			symbols: []ensureImportedExample{
				{
					input: NewSymbol("x/foo", "Bar"),
					want:  "notreallyanalias.Bar",
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
					t.Errorf("EnsureImported(%s) got %q, wanted %q", ttt.input, got, ttt.want)
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
