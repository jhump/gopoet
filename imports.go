package gopoet

import (
	"fmt"
	"path"
	"reflect"
	"sort"
	"strconv"
)

// Imports accumulate a set of package imports, used for generating a Go source
// file and accumulating references to other packages. As packages are imported,
// they will be assigned aliases if necessary (e.g. two imported packages
// otherwise would have the same name/prefix).
//
// Imports is not thread-safe.
type Imports struct {
	pkgPath       string
	importsByPath map[string]importDef
	pathsByName   map[string]string
}

type importDef struct {
	packageName string
	isAlias     bool
}

// NewImportsFor returns a new Imports where the source lives in pkgPath. So any
// uses of other symbols also in pkgPath will not need an import and will not
// use a package prefix (see EnsureImported).
func NewImportsFor(pkgPath string) *Imports {
	return &Imports{pkgPath: pkgPath}
}

// RegisterImportForPackage "imports" the specified package and returns the
// package prefix to use for symbols in the imported package. See
// RegisterImport for more details.
func (i *Imports) RegisterImportForPackage(pkg Package) string {
	return i.RegisterImport(pkg.ImportPath, pkg.Name)
}

// RegisterImport "imports" the specified package and returns the package prefix
// to use for symbols in the imported package. It is safe to import the same
// package repeatedly -- the same prefix will be returned every time. If an
// attempt is made to import the Imports source package (i.e. importing a
// package into itself), nothing will be done and an empty prefix will be
// returned. So such an action is safe and the returned prefix is correct for
// how symbols in the package should be referenced.
func (i *Imports) RegisterImport(importPath, packageName string) string {
	return i.prefixForPackage(importPath, packageName, true)
}

// PrefixForPackage returns a prefix to use for qualifying symbols from the
// given package. This method panics if the given package was never registered.
func (i *Imports) PrefixForPackage(importPath string) string {
	return i.prefixForPackage(importPath, "", false)
}

func (i *Imports) prefixForPackage(importPath, packageName string, registerIfNotFound bool) string {
	if importPath == i.pkgPath {
		return ""
	}
	if ex, ok := i.importsByPath[importPath]; ok {
		return ex.packageName + "."
	}

	if !registerIfNotFound {
		panic(fmt.Sprintf("Package %q never registered", importPath))
	}

	p := packageName
	if packageName == "" {
		p = path.Base(importPath)
	}
	pkgBase := p
	suffix := 1
	for {
		if _, ok := i.pathsByName[p]; !ok {
			if i.importsByPath == nil {
				i.importsByPath = map[string]importDef{}
				i.pathsByName = map[string]string{}
			}
			i.pathsByName[p] = importPath
			i.importsByPath[importPath] = importDef{
				packageName: p,
				isAlias:     p != packageName,
			}
			return p + "."
		}
		p = fmt.Sprintf("%s%d", pkgBase, suffix)
		suffix++
	}
}

// EnsureImported ensures that the given symbol is imported and returns a new
// symbol that has the correct package prefix (based on how the given symbol's
// package was imported/aliased). If the symbol is already in Imports source
// package then a symbol is returned whose Package has an empty Name. That way
// calling String() on the returned symbol will correctly elide the package
// prefix.
func (i *Imports) EnsureImported(sym Symbol) Symbol {
	return i.qualify(sym, true)
}

// Qualify returns a new symbol that has the correct package prefix, based on
// how the given symbol's package was imported/aliased. This method panics if
// symbol's package was never registered.
func (i *Imports) Qualify(sym Symbol) Symbol {
	return i.qualify(sym, false)
}

func (i *Imports) qualify(sym Symbol, registerIfNotFound bool) Symbol {
	if sym.Package.Name == "" && sym.Package.ImportPath == "" {
		return sym
	}
	name := i.prefixForPackage(sym.Package.ImportPath, sym.Package.Name, registerIfNotFound)
	if len(name) > 0 && name[len(name)-1] == '.' {
		name = name[:len(name)-1]
	}
	if name != sym.Package.Name {
		pkg := Package{Name: name, ImportPath: sym.Package.ImportPath}
		return Symbol{Package: pkg, Name: sym.Name}
	}
	return sym
}

// EnsureTypeImported ensures that any symbols referenced by the given type are
// imported and returns a new type with correct package prefixes. See
// EnsureImported for more details.
func (i *Imports) EnsureTypeImported(n TypeName) TypeName {
	return i.qualifyType(n, true)
}

// QualifyType returns a new type with correct package prefixes, based on how
// referenced type elements were actually imported/aliased. This method panics
// if any of the referenced packages were never registered.
func (i *Imports) QualifyType(n TypeName) TypeName {
	return i.qualifyType(n, false)
}

func (i *Imports) qualifyType(n TypeName, registerIfNotFound bool) TypeName {
	switch n.Kind() {
	case KindNamed:
		sym := n.Symbol()
		nsym := i.qualify(sym, registerIfNotFound)
		if nsym != sym {
			n = NamedType(nsym)
		}
	case KindPtr:
		elem := n.Elem()
		nelem := i.qualifyType(elem, registerIfNotFound)
		if nelem != elem {
			n = PointerType(nelem)
		}
	case KindSlice:
		elem := n.Elem()
		nelem := i.qualifyType(elem, registerIfNotFound)
		if nelem != elem {
			n = SliceType(nelem)
		}
	case KindArray:
		elem := n.Elem()
		nelem := i.qualifyType(elem, registerIfNotFound)
		if nelem != elem {
			n = ArrayType(nelem, n.Len())
		}
	case KindMap:
		key := n.Key()
		elem := n.Elem()
		nkey := i.qualifyType(key, registerIfNotFound)
		nelem := i.qualifyType(elem, registerIfNotFound)
		if nelem != elem || nkey != key {
			n = MapType(nkey, nelem)
		}
	case KindChan:
		elem := n.Elem()
		nelem := i.qualifyType(n.Elem(), registerIfNotFound)
		if nelem != elem {
			n = ChannelType(nelem, n.Dir())
		}
	case KindFunc:
		sig := n.Signature()
		nsig := i.qualifySignature(sig, registerIfNotFound)
		if nsig != sig {
			n = FuncTypeFromSig(nsig)
		}
	case KindStruct:
		fields := n.Fields()
		nfields := i.qualifyFields(fields, registerIfNotFound)
		if !sameSlice(nfields, fields) {
			n = StructType(nfields...)
		}
	case KindInterface:
		embeds := n.Embeds()
		nembeds := i.qualifySymbols(embeds, registerIfNotFound)
		methods := n.Methods()
		nmethods := i.qualifyMethods(methods, registerIfNotFound)
		if !sameSlice(nembeds, embeds) || !sameSlice(nmethods, methods) {
			n = InterfaceType(nembeds, nmethods...)
		}
	}
	return n
}

// EnsureAllTypesImported ensures that all argument and result value types in
// the given signature are imported and returns a new signature where all types
// contain the correct package prefixes. See EnsureTypeImported for more details.
func (i *Imports) EnsureAllTypesImported(s *Signature) *Signature {
	return i.qualifySignature(s, true)
}

// QualifySignature returns a new signature where all types contain the correct
// package prefixes, based on how the referenced packages were imported/aliased.
// This method panics if any of the referenced packages were never registered.
func (i *Imports) QualifySignature(s *Signature) *Signature {
	return i.qualifySignature(s, false)
}

func (i *Imports) qualifySignature(s *Signature, registerIfNotFound bool) *Signature {
	args := i.qualifyArgs(s.Args, registerIfNotFound)
	results := i.qualifyArgs(s.Results, registerIfNotFound)
	if !sameSlice(args, s.Args) || !sameSlice(results, s.Results) {
		s = &Signature{Args: args, Results: results, IsVariadic: s.IsVariadic}
	}
	return s
}

func sameSlice(s1 interface{}, s2 interface{}) bool {
	r1 := reflect.ValueOf(s1)
	r2 := reflect.ValueOf(s2)
	return r1.Pointer() == r2.Pointer() && r1.Len() == r2.Len()
}

func (i *Imports) qualifySymbols(syms []Symbol, registerIfNotFound bool) []Symbol {
	var ret []Symbol
	for idx, s := range syms {
		nt := i.qualify(s, registerIfNotFound)
		if nt != s {
			if ret == nil {
				ret = make([]Symbol, len(syms))
				copy(ret, syms)
			}
			ret[idx] = nt
		}
	}
	if ret == nil {
		return syms
	}
	return ret
}

func (i *Imports) qualifyArgs(args []ArgType, registerIfNotFound bool) []ArgType {
	var ret []ArgType
	for idx, a := range args {
		nt := i.qualifyType(a.Type, registerIfNotFound)
		if nt != a.Type {
			if ret == nil {
				ret = make([]ArgType, len(args))
				copy(ret, args)
			}
			ret[idx] = ArgType{Name: a.Name, Type: nt}
		}
	}
	if ret == nil {
		return args
	}
	return ret
}

func (i *Imports) qualifyFields(fields []FieldType, registerIfNotFound bool) []FieldType {
	var ret []FieldType
	for idx, f := range fields {
		nt := i.qualifyType(f.Type, registerIfNotFound)
		if nt != f.Type {
			if ret == nil {
				ret = make([]FieldType, len(fields))
				copy(ret, fields)
			}
			ret[idx] = FieldType{Name: f.Name, Type: nt, Tag: f.Tag}
		}
	}
	if ret == nil {
		return fields
	}
	return ret
}

func (i *Imports) qualifyMethods(methods []MethodType, registerIfNotFound bool) []MethodType {
	var ret []MethodType
	for idx, m := range methods {
		ns := i.qualifySignature(&m.Signature, registerIfNotFound)
		if ns != &m.Signature {
			if ret == nil {
				ret = make([]MethodType, len(methods))
				copy(ret, methods)
			}
			ret[idx] = MethodType{Name: m.Name, Signature: *ns}
		}
	}
	if ret == nil {
		return methods
	}
	return ret
}

// QualifyTemplateData will re-create the given template data value so that any
// references to packages (including elements/fields/etc whose type is
// gopoet.Package, gopoet.TypeName, gopoet.Signature, or any of the various
// gopoet.FileElement concrete types) indicate the correct package prefixes
// based on how the packages were actually imported/aliased.
//
// If any references are found to packages that have not been imported, they are
// added to the imports (e.g. i.RegisterImportForPackage) and the resulting
// package name or alias is used to re-create the reference.
func (i *Imports) QualifyTemplateData(data interface{}) interface{} {
	if data == nil {
		return nil
	}
	// we want to make sure the entry point value has a type of interface{}
	// (not data's concrete type) so we know we can safely re-write it if it
	// implements TypeName
	rv := reflect.ValueOf([]interface{}{data}).Index(0)
	newData, _ := qualifyTemplateData(i, rv)
	return newData.Interface()
}

// ImportSpecs returns the list of imports that have been accumulated so far,
// sorted lexically by import path.
func (i *Imports) ImportSpecs() []ImportSpec {
	specs := make([]ImportSpec, len(i.importsByPath))
	idx := 0
	for importPath, def := range i.importsByPath {
		specs[idx].ImportPath = importPath
		if def.isAlias {
			specs[idx].PackageAlias = def.packageName
		}
		idx++
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].ImportPath < specs[j].ImportPath
	})
	return specs
}

// ImportSpec describes an import statement in Go source. The spec's
// PackageAlias will be empty if the import statement needs no alias.
type ImportSpec struct {
	PackageAlias string
	ImportPath   string
}

// String returns a string representation of the import. It will be the import
// path in double-quotes. Optionally, if the package alias is not empty, it will
// have a prefix that indicates the alias. For example:
//
//    "some.domain.com/foo/bar"
//    bar2 "some.domain.com/foo/bar"
//
// The first line shows the string representation without an alias, the second
// line with.
func (i ImportSpec) String() string {
	if i.PackageAlias == "" {
		return strconv.Quote(i.ImportPath)
	}
	return i.PackageAlias + " " + strconv.Quote(i.ImportPath)
}
