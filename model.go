package gopoet

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"go/format"
	"go/types"
	"path"
	"strconv"
	"text/template"
)

// Package is a simple representation of a Go package. The name may actually be
// an effective alias (when the package is imported using an alias). A symbol
// whose package has an empty Name is a local symbol: in the same package as the
// referencing context (and thus needs no package prefix to reference that
// symbol).
type Package struct {
	ImportPath, Name string
}

// NewPackage is a simple factory method for a package whose name is the same as
// the base name (e.g. last element) of its import path.
//
// If the package name varies from its import path, use a struct initializer
// instead:
//
//    Package{Name: "foo", ImportPath: "some.domain.com/foo/v1"}
//
// If you do not know the package name, only the import path, then use
// a struct initializer and leave the Name field empty. An empty Name field will
// cause an alias to be used when the package is registered with an *Imports,
// to ensure that the resulting code will compile and uses a correct package
// prefix.
func NewPackage(importPath string) Package {
	return Package{
		Name:       path.Base(importPath),
		ImportPath: importPath,
	}
}

// Symbol returns a symbol with the given name in this package.
func (p Package) Symbol(name string) Symbol {
	return Symbol{Name: name, Package: p}
}

// String returns the package's name. While this is not always useful (the
// import path is generally more useful), it is the least surprising behavior
// if you reference the package instance from a code block:
//
//    gopoet.Printf("%s.Foobar", pkg)
//
// In this example, the result is a qualified reference, such as "pkg.Foobar"
// if the given package's name were "pkg".
func (p Package) String() string {
	return p.Name
}

// PackageForGoType returns a Package for the given Go types package. This is
// useful for converting between package representations.
func PackageForGoType(pkg *types.Package) Package {
	return Package{
		Name:       pkg.Name(),
		ImportPath: pkg.Path(),
	}
}

// Symbol references an element in Go source. It is a const, var, function, or
// type.
type Symbol struct {
	Package Package
	Name    string
}

// SymbolOrMethodRef is either a Symbol or a MethodReference (either of which
// can refer to an element in a Go source file).
type SymbolOrMethodRef interface {
	fmt.Stringer
	isSymbolOrMethodRef()
}

// NewSymbol creates a symbol for the following package and name. The given
// package path is assumed to have a name that matches its base name (e.g. last
// path element). This function is a convenient shorthand for this:
//
//    NewPackage(importPath).Symbol(symName)
func NewSymbol(importPath, symName string) Symbol {
	return NewPackage(importPath).Symbol(symName)
}

// SymbolForGoObject returns the given Go types object as a gopoet.Symbol or as
// a gopoet.MethodReference. If the given object is a method, it returns a
// method reference. Otherwise (const, var, type, non-method func), the returned
// value is a symbol.
func SymbolForGoObject(obj types.Object) SymbolOrMethodRef {
	if fn, ok := obj.(*types.Func); ok {
		sig := fn.Type().(*types.Signature)
		if sig.Recv() != nil {
			// it's a method
			recvType := TypeNameForGoType(sig.Recv().Type())
			if recvType.Kind() == KindPtr {
				recvType = recvType.Elem()
			}
			return MethodReference{
				Type:   recvType.Symbol(),
				Method: obj.Name(),
			}
		}
	}
	return PackageForGoType(obj.Pkg()).Symbol(obj.Name())
}

// String prints the symbol as it should appear in Go source: pkg.Name. The
// "pkg." prefix will be omitted if the symbol's Package has an empty Name.
func (s Symbol) String() string {
	if s.Package.Name != "" {
		return s.Package.Name + "." + s.Name
	}
	return s.Name
}

func (s Symbol) isSymbolOrMethodRef() {}

// GoFile represents a Go source file. It has methods for adding Go source
// declarations such as consts, vars, types, and functions. When building
// the declarations, particularly implementation code that references other
// types and packages, if all references are done using instances of
// gopoet.Package, gopoet.Symbol, and gopoet.TypeName, then the file's
// imports will be managed automatically. Code that constructs a GoFile can
// also use the Imports methods to manually register imports where necessary.
//
// To construct a GoFile, use the NewGoFile factory function.
type GoFile struct {
	Imports

	// The name of the file. This should not include a path, only a file
	// name with a ".go" extension.
	Name string
	// Doc comments that will appear before the package declaration.
	PackageComment string
	// The package name that will be used in the package declaration.
	PackageName string
	// The canonical import path which, if non-empty, will be used in an
	// annotation comment immediately following the package declaration.
	CanonicalImportPath string
	// The actual elements declared in the source file.
	elements []FileElement
}

// FormatError is the kind of error returned from the WriteGoFile method (and
// its variations) when the resulting rendered code has a syntax error that
// prevents it from being formatted by the "go/format" package. Code that gets
// an instance of this kind of error can examine the unformatted code to
// associate formatting error messages (which usually contain line and column
// information) with the text that induced them.
type FormatError struct {
	// The unformatted Go code.
	Unformatted []byte
	// The underlying error returned from the "go/format" package.
	Err error
}

// Error implements the error interface, delegating to the underlying error
// returned from the "go/format" package.
func (e *FormatError) Error() string {
	return e.Err.Error()
}

// WriteGoFile renders the given Go file to the given writer.
//
// This process first qualifies all elements and code in the given file, to
// ensure that all referenced packages will be correctly represented with
// import statements and that all referenced packages will be rendered with
// the correct package name or alias.
//
// Then this serializes the file elements and contained code into a buffer,
// but with no attention paid to formatting.
//
// Finally, the "go/format" package is used to convert the generated code into
// the Go canonical format, which is primarily needed to clean up whitespace and
// indentation in the output.
//
// The formatted code is then written to the given io.Writer.
func WriteGoFile(w io.Writer, f *GoFile) error {
	// make sure all package references are properly qualified for
	for i := range f.elements {
		f.elements[i].qualify(&f.Imports)
	}

	// now we generate the code
	var buf bytes.Buffer
	if err := f.writeTo(&buf); err != nil {
		return err
	}
	// format it
	unformatted := buf.Bytes()
	formatted, err := format.Source(unformatted)
	if err != nil {
		return &FormatError{Unformatted: unformatted, Err: err}
	}
	// and finally emit it
	_, err = w.Write(formatted)
	return err
}

// WriteGoFiles renders all of the given Go files, using the given function to
// open io.Writer instances, to which the rendered and formatted Go source is
// written. The function is given a Go file's path, which includes both the
// package path and the file's name and is expected to return a writer just for
// that file.
//
// If the function returns an error for any file, this function aborts and
// returns the error, but it makes no effort to clean up any previously written
// files that may have already been written without error.
func WriteGoFiles(outFn func(path string) (io.WriteCloser, error), files ...*GoFile) error {
	for _, file := range files {
		p := filepath.Join(file.Package().ImportPath, file.Name)
		w, err := outFn(p)
		if err != nil {
			return err
		}
		err = func() error {
			defer w.Close()
			return WriteGoFile(w, file)
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteGoFilesToFileSystem is a convenience wrapper for WriteGoFiles that uses
// os.Open as the function to open a writer for each file. A full path is first
// computed by joining the given root directory with the file's path.
func WriteGoFilesToFileSystem(rootDir string, files ...*GoFile) error {
	return WriteGoFiles(func(path string) (io.WriteCloser, error) {
		fullPath := filepath.Join(rootDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, err
		}
		return os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	}, files...)
}

// WriteGoFilesToGoPath is a convenience wrapper for WriteGoFilesToFileSystem
// that inspects the GOPATH environment variable and uses the first entry
// therein as the root directory for where files are to be written. If the
// GOPATH environment variable is empty or unset, an error is returned.
func WriteGoFilesToGoPath(files ...*GoFile) error {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return errors.New("GOPATH is empty")
	}
	paths := strings.Split(gopath, string(filepath.ListSeparator))
	if len(paths) != 0 {
		return errors.New("GOPATH is empty")
	}
	return WriteGoFilesToFileSystem(paths[0], files...)
}

// NewGoFile creates a new Go file with the given name and package information.
// The given package path is used to ensure that all referenced elements are
// correctly qualified: e.g. if other elements are in the same package they do
// not need a qualifier but other elements do need a qualifier.
//
// The given package name will be referenced in the package statement at the top
// of the rendered Go source file.
func NewGoFile(fileName, packagePath, packageName string) *GoFile {
	if filepath.Base(fileName) != fileName {
		panic("Go file name must be a base name with no path")
	}
	if filepath.Ext(fileName) != ".go" {
		panic("Go file name must have a '.go' extension")
	}

	ret := GoFile{Name: fileName, PackageName: packageName}
	ret.Imports = *NewImportsFor(packagePath)
	return &ret
}

// AddElement adds the given element to this file. This method returns the file,
// for method chaining.
func (f *GoFile) AddElement(e FileElement) *GoFile {
	e.setParent(f)
	f.elements = append(f.elements, e)
	return f
}

// AddConst adds the given const to this file. This is shorthand for wrapping
// the given const up into a *gopoet.ConstDecl and passing that to f.AddElement.
// This method returns the file, for method chaining.
func (f *GoFile) AddConst(c *ConstSpec) *GoFile {
	return f.AddElement(&ConstDecl{Consts: []*ConstSpec{c}})
}

// AddVar adds the given var to this file. This is shorthand for wrapping the
// given var up into a *gopoet.VarDecl and passing that to f.AddElement. This
// method returns the file, for method chaining.
func (f *GoFile) AddVar(v *VarSpec) *GoFile {
	return f.AddElement(&VarDecl{Vars: []*VarSpec{v}})
}

// AddType adds the given type to this file. This is shorthand for wrapping
// the given type up into a *gopoet.TypeDecl and passing that to f.AddElement.
// This method returns the file, for method chaining.
func (f *GoFile) AddType(t *TypeSpec) *GoFile {
	return f.AddElement(&TypeDecl{Types: []*TypeSpec{t}})
}

// NumElements returns the number of top-level elements added to this Go file.
func (f *GoFile) NumElements() int {
	return len(f.elements)
}

// ElementAt retrieves the top-level element at the given index. The index must
// be between zero (inclusive) and f.NumElements() (exclusive).
func (f *GoFile) ElementAt(i int) FileElement {
	return f.elements[i]
}

// Package returns the package in which the file is declared.
func (f *GoFile) Package() Package {
	return Package{
		Name:       f.PackageName,
		ImportPath: f.Imports.pkgPath,
	}
}

func (f *GoFile) writeTo(w *bytes.Buffer) error {
	// package and imports pre-amble
	writeComment(f.PackageComment, w)
	if f.CanonicalImportPath != "" {
		fmt.Fprintf(w, "package %s // import %q\n", f.PackageName, f.CanonicalImportPath)
	} else {
		fmt.Fprintf(w, "package %s\n", f.PackageName)
	}

	w.WriteRune('\n')

	for _, i := range f.ImportSpecs() {
		if i.PackageAlias != "" {
			fmt.Fprintf(w, "import %s %q\n", i.PackageAlias, i.ImportPath)
		} else {
			fmt.Fprintf(w, "import %q\n", i.ImportPath)
		}
	}

	// now we print all of the file elements
	for _, el := range f.elements {
		w.WriteRune('\n')
		el.writeTo(w)
	}

	return nil
}

func writeComment(comment string, w *bytes.Buffer) {
	if comment != "" {
		lines := strings.Split(comment, "\n")
		for _, line := range lines {
			fmt.Fprintf(w, "// %s\n", line)
		}
	}
}

// FileElement is a top-level element in a Go source file. It will be a const,
// var, type, or func declaration. The concrete types that implement this
// interface are *gopoet.ConstDecl, *gopoet.VarDecl, *gopoet.TypeDecl, and
// *gopoet.FuncSpec.
type FileElement interface {
	isFileElement()
	writeTo(b *bytes.Buffer)
	setParent(*GoFile)
	qualify(imports *Imports)
}

// ConstDecl is a FileElement representing a declaration of one or more consts.
// When it has exactly one const, it is rendered like so:
//
//    const name Type = initialValue
//
// When there are multiple consts, it is rendered differently:
//
//    const (
//        name1 Type = initialValue
//        name2 Type = initialValue
//    )
type ConstDecl struct {
	Comment string
	Consts  []*ConstSpec
}

// NewConstDecl creates a new declaration for the given consts.
func NewConstDecl(cs ...*ConstSpec) *ConstDecl {
	return &ConstDecl{Consts: cs}
}

// AddConst adds the given const to this declaration. This method returns the
// const declaration, for method chaining.
func (c *ConstDecl) AddConst(cs *ConstSpec) *ConstDecl {
	c.Consts = append(c.Consts, cs)
	return c
}

// SetComment sets the declaration comment. For declarations with multiple
// consts, this comment is rendered above the "const" keyword, and the comments
// on each ConstSpec are rendered just above each individual const. For example:
//
//    // Comment from the ConstDecl
//    const (
//        // Comment from the first ConstSpec
//        c1 = value
//        // Comment from the second ConstSpec
//        c2 = otherValue
//    )
//
// If the declaration has only a single const, then leave this comment empty and
// instead use the Comment field on the ConstSpec only. That will then render
// like so:
//
//    // Comment from the ConstSpec
//    const c1 = value
//
// This method returns the const declaration, for method chaining.
func (c *ConstDecl) SetComment(comment string) *ConstDecl {
	c.Comment = comment
	return c
}

func (c *ConstDecl) isFileElement() {}

func (c *ConstDecl) writeTo(b *bytes.Buffer) {
	if len(c.Consts) > 1 || c.Comment != "" {
		writeComment(c.Comment, b)
		b.WriteString("const (\n")
		for _, cs := range c.Consts {
			cs.writeTo(b)
		}
		b.WriteString(")\n")
		return
	}

	b.WriteString("const ")
	c.Consts[0].writeTo(b)
}

func (c *ConstDecl) setParent(f *GoFile) {
	for _, cnst := range c.Consts {
		cnst.parent = f
	}
}

func (c *ConstDecl) qualify(imports *Imports) {
	for _, cs := range c.Consts {
		cs.Type = qualifyType(imports, cs.Type)
		if cs.Initializer != nil {
			cs.Initializer.qualify(imports)
		}
	}
}

func qualifyType(imports *Imports, t TypeNameOrSpec) TypeNameOrSpec {
	switch t := t.(type) {
	case nil:
		return nil
	case TypeName:
		return imports.EnsureTypeImported(t)
	case *TypeSpec:
		t.qualify(imports)
		return t
	default:
		panic(fmt.Sprintf("unrecognized type %T", t))
	}
}

// ConstSpec describes a single const expression (which can actually identify
// more than one const). The Names field is not optional and must have at least
// one value. Just like in Go source, the Type field is optional but the
// Initializer is not.
//
// However, there is an exception to this rule: a const is allowed to have
// neither type nor initializer if it follows a const and is intended to re-use
// the prior const's type and initializer. This is typically used with iota
// expressions, like so:
//
//    const (
//        // This const has both type and initializer
//        a int64 = iota+1
//        // These consts have neither and will re-use the above type and
//        // initializer (relying on the fact that iota is auto-incremented
//        // after every reference inside of a const declaration)
//        b
//        c
//        d
//    )
type ConstSpec struct {
	Comment     string
	Names       []string
	Type        TypeNameOrSpec
	Initializer *CodeBlock
	parent      *GoFile
}

// NewConst returns a new const expression with the given name(s).
func NewConst(names ...string) *ConstSpec {
	return &ConstSpec{Names: names}
}

// SetComment sets the comment for this const expression. See
// ConstDecl.SetComment for more details on how comments are rendered.
//
// This method returns the const, for method chaining.
func (c *ConstSpec) SetComment(comment string) *ConstSpec {
	c.Comment = comment
	return c
}

// SetType sets the type of this const expression. This method returns the
// const, for method chaining.
func (c *ConstSpec) SetType(t TypeNameOrSpec) *ConstSpec {
	c.Type = t
	return c
}

// Initialize sets the initializer for this const to a CodeBlock with a single
// statement defined by the given format message and arguments. It is a
// convenient shorthand for this:
//    c.Initializer = gopoet.Printf(fmt, args...)
// This method returns the const, for method chaining.
func (c *ConstSpec) Initialize(fmt string, args ...interface{}) *ConstSpec {
	c.Initializer = Printf(fmt, args...)
	return c
}

// SetInitializer sets the initializer to the given code block. This method
// returns the const, for method chaining.
func (c *ConstSpec) SetInitializer(cb *CodeBlock) *ConstSpec {
	c.Initializer = cb
	return c
}

// ToSymbol returns a symbol that refers to this const. This method panics if
// the const expression indicates more than one const name, in which case the
// calling code should instead use SymbolAt. This method also panics if this
// const has never been added to a file (see SymbolAt for more information about
// this).
func (c *ConstSpec) ToSymbol() Symbol {
	if len(c.Names) != 1 {
		panic(fmt.Sprintf("must be exactly one const to use ToSymbol(); use SymbolAt(int) instead: %v", c.Names))
	}
	return c.SymbolAt(0)
}

// String returns a simple string representation of the const. The string
// representation is the same as that of the symbol returned by ToSymbol,
// however this method will not panic: if the var has never been associated
// with a file, it will return just the const's name, unqualified. Also, if this
// const indicates more than one const name, the returned string will be a
// comma-separated list of all of the symbols.
func (c *ConstSpec) String() string {
	if len(c.Names) != 1 {
		var syms []string
		if c.parent == nil {
			syms = c.Names
		} else {
			syms = make([]string, len(c.Names))
			for i := range c.Names {
				syms[i] = c.SymbolAt(i).String()
			}
		}
		return strings.Join(syms, ", ")
	}

	if c.parent == nil {
		return c.Names[0]
	}
	return c.ToSymbol().String()
}

// SymbolAt returns a symbol that refers to the const name at the given index.
// This method panics if the given index is not valid (e.g. between zero,
// inclusive, and len(c.Names), exclusive). This method will also panic if this
// const has never been added to a file since, without an associated file, the
// package of the symbol is unknown. It can be added to a file directly via
// GoFile.AddConst or indirectly by first adding to a ConstDecl which is then
// added to a file.
func (c *ConstSpec) SymbolAt(i int) Symbol {
	if c.parent == nil {
		panic(fmt.Sprintf("cannot use const %q as symbol because it has not been associated with a file", c.Names[i]))
	}
	return Symbol{
		Name:    c.Names[i],
		Package: c.parent.Package(),
	}
}

func (c *ConstSpec) writeTo(b *bytes.Buffer) {
	writeComment(c.Comment, b)
	fmt.Fprintf(b, "%s", strings.Join(c.Names, ", "))
	if c.Type != nil {
		fmt.Fprintf(b, " %v", c.Type)
	}
	if c.Initializer != nil {
		b.WriteString(" = ")
		c.Initializer.writeTo(b)
	}
	b.WriteRune('\n')
}

// VarDecl is a FileElement representing a declaration of one or more vars. When
// it has exactly one var, it is rendered like so:
//
//    var name Type = initialValue
//
// When there are multiple vars, it is rendered differently:
//
//    var (
//        name1 Type = initialValue
//        name2 Type = initialValue
//    )
type VarDecl struct {
	Comment string
	Vars    []*VarSpec
}

// NewVarDecl creates a new declaration for the given vars.
func NewVarDecl(vs ...*VarSpec) *VarDecl {
	return &VarDecl{Vars: vs}
}

// AddVar adds the given var to this declaration. This method returns the var
// declaration, for method chaining.
func (v *VarDecl) AddVar(vs *VarSpec) *VarDecl {
	v.Vars = append(v.Vars, vs)
	return v
}

// SetComment sets the declaration comment. For declarations with multiple
// vars, this comment is rendered above the "var" keyword, and the comments on
// each VarSpec are rendered just above each individual var. For example:
//
//    // Comment from the VarDecl
//    var (
//        // Comment from the first VarSpec
//        v1 = value
//        // Comment from the second VarSpec
//        v2 = otherValue
//    )
//
// If the declaration has only a single var, then leave this comment empty and
// instead use the Comment field on the VarSpec only. That will then render like
// so:
//
//    // Comment from the VarSpec
//    var v1 = value
//
// This method returns the var declaration, for method chaining.
func (v *VarDecl) SetComment(comment string) *VarDecl {
	v.Comment = comment
	return v
}

func (v *VarDecl) isFileElement() {}

func (v *VarDecl) writeTo(b *bytes.Buffer) {
	if len(v.Vars) > 1 || v.Comment != "" {
		writeComment(v.Comment, b)
		b.WriteString("var (\n")
		for _, vs := range v.Vars {
			vs.writeTo(b)
		}
		b.WriteString(")\n")
		return
	}

	b.WriteString("var ")
	v.Vars[0].writeTo(b)
}

func (v *VarDecl) setParent(f *GoFile) {
	for _, vr := range v.Vars {
		vr.parent = f
	}
}

func (v *VarDecl) qualify(imports *Imports) {
	for _, vs := range v.Vars {
		vs.Type = qualifyType(imports, vs.Type)
		if vs.Initializer != nil {
			vs.Initializer.qualify(imports)
		}
	}
}

// VarSpec describes a single var expression (which can actually identify more
// than one var). The Names field is not optional and must have at least one
// value. Just like in Go source, the Type field is optional only if the
// Initializer is set (in which case, the type is inferred from the type of
// the initializer expression). One or the other, or both, must be present.
type VarSpec struct {
	Comment     string
	Names       []string
	Type        TypeNameOrSpec
	Initializer *CodeBlock
	parent      *GoFile
}

// NewVar returns a new var expression that defines the given name(s).
func NewVar(names ...string) *VarSpec {
	return &VarSpec{Names: names}
}

// SetComment sets the comment for this var expression. See VarDecl.SetComment
// for more details on how comments are rendered.
//
// This method returns the var, for method chaining.
func (v *VarSpec) SetComment(comment string) *VarSpec {
	v.Comment = comment
	return v
}

// SetType sets the type of this var. This method returns the var, for method
// chaining.
func (v *VarSpec) SetType(t TypeNameOrSpec) *VarSpec {
	v.Type = t
	return v
}

// Initialize sets the initializer for this var to a CodeBlock with a single
// statement defined by the given format message and arguments. It is a
// convenient shorthand for this:
//    v.Initializer = gopoet.Printf(fmt, args...)
// This method returns the var, for method chaining.
func (v *VarSpec) Initialize(fmt string, args ...interface{}) *VarSpec {
	v.Initializer = Printf(fmt, args...)
	return v
}

// SetInitializer sets the initializer to the given code block. This method
// returns the var, for method chaining.
func (v *VarSpec) SetInitializer(cb *CodeBlock) *VarSpec {
	v.Initializer = cb
	return v
}

// ToSymbol returns a symbol that refers to this var. This method panics if the
// var expression indicates more than one var name, in which case calling code
// should instead use SymbolAt. This method also panics if this var has never
// been added to a file (see SymbolAt for more information about this).
func (v *VarSpec) ToSymbol() Symbol {
	if len(v.Names) != 1 {
		panic(fmt.Sprintf("must be exactly one var to use ToSymbol(); use SymbolAt(int) instead: %v", v.Names))
	}
	return v.SymbolAt(0)
}

// String returns a simple string representation of the var. The string
// representation is the same as that of the symbol returned by ToSymbol,
// however this method will not panic: if the var has never been associated
// with a file, it will return just the var's name, unqualified. Also, if this
// var indicates more than one var name, the returned string will be a
// comma-separated list of all of the symbols.
func (v *VarSpec) String() string {
	if len(v.Names) != 1 {
		var syms []string
		if v.parent == nil {
			syms = v.Names
		} else {
			syms = make([]string, len(v.Names))
			for i := range v.Names {
				syms[i] = v.SymbolAt(i).String()
			}
		}
		return strings.Join(syms, ", ")
	}

	if v.parent == nil {
		return v.Names[0]
	}
	return v.ToSymbol().String()
}

// SymbolAt returns a symbol that refers to the var name at the given index.
// This method panics if the given index is not valid (e.g. between zero,
// inclusive, and len(v.Names), exclusive). This method will also panic if this
// var has never been added to a file since, without an associated file, the
// package of the symbol is unknown. It can be added to a file directly via
// GoFile.AddVar or indirectly by first adding to a VarDecl which is then added
// to a file.
func (v *VarSpec) SymbolAt(i int) Symbol {
	if v.parent == nil {
		panic(fmt.Sprintf("cannot use var %s %v as symbol because it has not been associated with a file", v.Names[i], v.Type))
	}
	return Symbol{
		Name:    v.Names[i],
		Package: v.parent.Package(),
	}
}

func (v *VarSpec) writeTo(b *bytes.Buffer) {
	writeComment(v.Comment, b)
	fmt.Fprintf(b, "%s", strings.Join(v.Names, ", "))
	if v.Type != nil {
		fmt.Fprintf(b, " %v", v.Type)
	}
	if v.Initializer != nil {
		b.WriteString(" = ")
		v.Initializer.writeTo(b)
	}
	b.WriteRune('\n')
}

// TypeDecl is a FileElement representing a declaration of one or more types.
// When it has exactly one type, it is rendered like so:
//
//    type Name underlyingType
//
// When there are multiple types, it is rendered differently:
//
//    type (
//        Name1 underlyingType1
//        name2 underlyingType2
//    )
type TypeDecl struct {
	Comment string
	Types   []*TypeSpec
}

// NewTypeDecl creates a new declaration for the given types.
func NewTypeDecl(ts ...*TypeSpec) *TypeDecl {
	return &TypeDecl{Types: ts}
}

// AddType adds the given type to this declaration. This method returns the type
// declaration, for method chaining.
func (t *TypeDecl) AddType(ts *TypeSpec) *TypeDecl {
	t.Types = append(t.Types, ts)
	return t
}

// SetComment sets the declaration comment. For declarations with multiple
// types, this comment is rendered above the "type" keyword, and the comments on
// each TypeSpec are rendered just above each individual type. For example:
//
//    // Comment from the TypeDecl
//    type (
//        // Comment from the first TypeSpec
//        T1 int
//        // Comment from the second TypeSpec
//        T2 string
//    )
//
// If the declaration has only a single type, then leave this comment empty and
// instead use the Comment field on the TypeSpec only. That will then render
// like so:
//
//    // Comment from the TypeSpec
//    type T1 int
//
// This method returns the type declaration, for method chaining.
func (t *TypeDecl) SetComment(comment string) *TypeDecl {
	t.Comment = comment
	return t
}

func (t *TypeDecl) isFileElement() {}

func (t *TypeDecl) writeTo(b *bytes.Buffer) {
	if len(t.Types) > 1 || t.Comment != "" {
		writeComment(t.Comment, b)
		b.WriteString("type (\n")
		for _, ts := range t.Types {
			ts.writeTo(b)
		}
		b.WriteString(")\n")
		return
	}

	b.WriteString("type ")
	t.Types[0].writeTo(b)
}

func (t *TypeDecl) setParent(f *GoFile) {
	for _, ts := range t.Types {
		ts.setParent(f)
	}
}

func (t *TypeDecl) qualify(imports *Imports) {
	for _, ts := range t.Types {
		ts.qualify(imports)
	}
}

// TypeSpec describes a single type. The underlying type is not optional and not
// exported, so the only way to create a valid *TypeSpec is using the various
// factory methods (NewTypeSpec, NewTypeAlias, NewStructTypeSpec, and
// NewInterfaceTypeSpec).
type TypeSpec struct {
	Comment, Name string
	isAlias       bool
	underlying    TypeName
	structBody    []*FieldSpec
	interfaceBody []InterfaceElement
	parent        *GoFile
}

// NewTypeAlias creates a new type spec that aliases the given type to the given
// name.
func NewTypeAlias(typeName string, t TypeNameOrSpec) *TypeSpec {
	ts := NewTypeSpec(typeName, t)
	ts.isAlias = true
	return ts
}

// NewTypeSpec creates a new type spec that declares the given name to have the
// given underlying type.
func NewTypeSpec(typeName string, t TypeNameOrSpec) *TypeSpec {
	switch t := t.(type) {
	case *TypeSpec:
		ret := &TypeSpec{
			Name:       typeName,
			underlying: t.ToTypeName(),
		}
		if t.Name == "" {
			ret.structBody = append([]*FieldSpec(nil), t.structBody...)
			ret.interfaceBody = append([]InterfaceElement(nil), t.interfaceBody...)
		}
		return ret

	case TypeName:
		ret := &TypeSpec{
			Name:       typeName,
			underlying: t,
		}
		switch t.Kind() {
		case KindStruct:
			fieldTypes := t.Fields()
			fields := make([]*FieldSpec, len(fieldTypes))
			for i, ft := range fieldTypes {
				fields[i] = &FieldSpec{
					parent: ret,
					Name:   ft.Name,
					Type:   ft.Type,
					Tag:    ft.Tag,
				}
			}
			ret.structBody = fields
		case KindInterface:
			embeds := t.Embeds()
			methods := t.Methods()
			elements := make([]InterfaceElement, len(embeds)+len(methods))
			for i, e := range embeds {
				elements[i] = &InterfaceEmbed{
					parent: ret,
					Type:   e,
				}
			}
			offs := len(embeds)
			for i, m := range methods {
				elements[i+offs] = &InterfaceMethod{
					parent:    ret,
					Name:      m.Name,
					Signature: m.Signature,
				}
			}
			ret.interfaceBody = elements
		}
		return ret

	default:
		panic(fmt.Sprintf("unknown type representation %T", t))
	}
}

// NewStructTypeSpec creates a new type spec for a struct with the given name
// and given fields.
func NewStructTypeSpec(typeName string, fields ...*FieldSpec) *TypeSpec {
	ret := &TypeSpec{
		Name:       typeName,
		structBody: fields,
	}
	fieldTypes := make([]FieldType, len(fields))
	for i, f := range fields {
		f.parent = ret
		var fieldType TypeName
		switch t := f.Type.(type) {
		case TypeName:
			fieldType = t
		case *TypeSpec:
			fieldType = t.ToTypeName()
		}
		fieldTypes[i] = FieldType{
			Name: f.Name,
			Type: fieldType,
			Tag:  f.Tag,
		}
	}
	ret.underlying = StructType(fieldTypes...)
	return ret
}

// NewInterfaceTypeSpec creates a new type spec for an interface with the given
// name and given elements (explicit methods and embedded interfaces).
func NewInterfaceTypeSpec(typeName string, elements ...InterfaceElement) *TypeSpec {
	ret := &TypeSpec{
		Name:          typeName,
		interfaceBody: elements,
	}
	var embeds []Symbol
	var methods []MethodType
	for _, e := range elements {
		e.setParent(ret)
		switch e := e.(type) {
		case *InterfaceEmbed:
			embeds = append(embeds, e.Type)
		case *InterfaceMethod:
			methods = append(methods, MethodType{
				Name:      e.Name,
				Signature: e.Signature,
			})
		}
	}
	ret.underlying = InterfaceType(embeds, methods...)
	return ret
}

// SetComment sets the comment for this type. See TypeDecl.SetComment for more
// details on how comments are rendered.
//
// This method returns the type, for method chaining.
func (t *TypeSpec) SetComment(comment string) *TypeSpec {
	t.Comment = comment
	return t
}

// ToSymbol returns a symbol that refers to this type. This method panics if
// this is an unnamed type. This method also panics if this type has never been
// added to a file since, without an associated file, the package of the symbol
// is unknown. It can be added to a file directly via GoFile.AddType or
// indirectly by first adding to a TypeDecl which is then added to a file.
func (t *TypeSpec) ToSymbol() Symbol {
	if t.Name == "" {
		panic("cannot use unnamed type as symbol")
	}
	if t.parent == nil {
		panic(fmt.Sprintf("cannot use type %s as symbol because it has not been associated with a file", t.Name))
	}
	return Symbol{
		Name:    t.Name,
		Package: t.parent.Package(),
	}
}

// String returns a simple string representation of the type. The string
// representation is the same as that of the type name returned by ToTypeName.
//
// However this method will not panic: if this is a named type that has never
// been associated with a file, it will return just the type's name,
// unqualified.
func (t *TypeSpec) String() string {
	if t.Name == "" {
		return t.underlying.String()
	}
	if t.parent == nil {
		return t.Name
	}
	return t.ToSymbol().String()
}

// ToTypeName returns a TypeName that represents this type. If this is a named
// type, the returned TypeName will have a kind of KindNamed. If this is not a
// named type then this is the same as calling t.Underlying().
//
// If this is a named type, it will panic if the type has not been added to a
// file. (See ToSymbol for more details on this constraint.)
func (t *TypeSpec) ToTypeName() TypeName {
	if t.Name == "" {
		return t.underlying
	}
	return NamedType(t.ToSymbol())
}

// Underlying returns the underlying type for this named type. If this type is
// unnamed, this is the same as calling t.ToTypeName().
func (t *TypeSpec) Underlying() TypeName {
	return t.underlying
}

// IsAlias returns true if this is a type alias, vs. a regular type definition.
func (t *TypeSpec) IsAlias() bool {
	return t.isAlias
}

func (t *TypeSpec) setParent(f *GoFile) {
	t.parent = f
}

func (t *TypeSpec) isTypeNameOrSpec() {}

func (t *TypeSpec) qualify(imports *Imports) {
	t.underlying = imports.EnsureTypeImported(t.underlying)
	for _, fs := range t.structBody {
		switch t := fs.Type.(type) {
		case TypeName:
			fs.Type = imports.EnsureTypeImported(t)
		case *TypeSpec:
			t.qualify(imports)
		}
	}
	for _, ie := range t.interfaceBody {
		ie.qualify(imports)
	}
}

func (t *TypeSpec) writeTo(b *bytes.Buffer) {
	writeComment(t.Comment, b)
	b.WriteString(t.Name)
	b.WriteRune(' ')
	if t.isAlias {
		b.WriteString("= ")
	}
	t.writeUnderlying(b)
}

func (t *TypeSpec) writeUnderlying(b *bytes.Buffer) {
	switch t.underlying.Kind() {
	case KindStruct:
		b.WriteString(" struct {\n")
		for _, f := range t.structBody {
			f.writeTo(b)
		}
		b.WriteString("}\n")
	case KindInterface:
		b.WriteString(" interface {\n")
		for _, el := range t.interfaceBody {
			el.writeTo(b)
		}
		b.WriteString("}\n")
	default:
		b.WriteRune(' ')
		typeNameToBuffer(t.underlying, b)
		b.WriteRune('\n')
	}
}

// TypeNameOrSpec represents a type and can either be a TypeName instance or a
// *TypeSpec. The former allows creating arbitrary type references whereas the
// latter allows referencing type elements whose definitions are also being
// generated.
//
// A *TypeSpec also allows for defining comments on struct fields and interface
// methods, for unnamed structs and interfaces, whereas a TypeName does not (a
// TypeName has only type information, no other source-level information).
type TypeNameOrSpec interface {
	isTypeNameOrSpec()
}

// FieldSpec describes a field in a struct type.
type FieldSpec struct {
	Comment, Name string
	Tag           reflect.StructTag
	Type          TypeNameOrSpec
	parent        *TypeSpec
}

// NewField creates a new field with the given name and type. An empty name
// indicates an anonymous field, also known as an embedded type.
func NewField(name string, t TypeNameOrSpec) *FieldSpec {
	return &FieldSpec{Name: name, Type: t}
}

// SetComment sets the comment on the field that will be rendered in the struct
// definition. This method returns the field, for method chaining.
func (f *FieldSpec) SetComment(comment string) *FieldSpec {
	f.Comment = comment
	return f
}

// SetTag sets the struct tag for the field. This method returns the field, for
// method chaining.
func (f *FieldSpec) SetTag(tag string) *FieldSpec {
	f.Tag = reflect.StructTag(tag)
	return f
}

// String returns the name of the field. This allows the field to be referenced
// as a format argument in a code block and result in a valid field name being
// interpolated into the code.
func (f *FieldSpec) String() string {
	return f.Name
}

func (f *FieldSpec) writeTo(b *bytes.Buffer) {
	writeComment(f.Comment, b)
	if f.Name != "" {
		b.WriteString(f.Name)
		b.WriteRune(' ')
	}

	switch t := f.Type.(type) {
	case TypeName:
		if t.Kind() == KindStruct || t.Kind() == KindInterface {
			ts := NewTypeSpec("", t)
			ts.writeUnderlying(b)
		} else {
			fmt.Fprintf(b, "%v", t)
		}
	case *TypeSpec:
		if t.Name == "" {
			t.writeUnderlying(b)
		} else {
			fmt.Fprintf(b, "%v", t.ToSymbol())
		}
	}

	if f.Tag != "" {
		tag := string(f.Tag)
		if strconv.CanBackquote(tag) {
			fmt.Fprintf(b, " `%s`", tag)
		} else {
			fmt.Fprintf(b, " %q", tag)
		}
	}

	b.WriteRune('\n')
}

// InterfaceElement is an element inside of an interface definition. This can be
// either an embedded interface or a method, represented by the InterfaceEmbed
// and InterfaceMethod types, respectively.
type InterfaceElement interface {
	isInterfaceElement()
	writeTo(b *bytes.Buffer)
	setParent(*TypeSpec)
	qualify(*Imports)
}

// InterfaceEmbed is an embedded interface inside of an interface definition.
type InterfaceEmbed struct {
	Comment string
	Type    Symbol
	parent  *TypeSpec
}

// NewInterfaceEmbed returns an embedded interface for the interface type named
// by the given symbol.
func NewInterfaceEmbed(s Symbol) *InterfaceEmbed {
	return &InterfaceEmbed{Type: s}
}

// SetComment sets the comment on this embedded interface that will be rendered
// in the interface definition. This method returns the embed, for method
// chaining.
func (e *InterfaceEmbed) SetComment(comment string) *InterfaceEmbed {
	e.Comment = comment
	return e
}

// String returns a string representation of this embedded interface, which is
// the same as the string representation of the referenced symbol.
func (e *InterfaceEmbed) String() string {
	return e.Type.String()
}

func (e *InterfaceEmbed) isInterfaceElement() {}

func (e *InterfaceEmbed) writeTo(b *bytes.Buffer) {
	writeComment(e.Comment, b)
	fmt.Fprintf(b, "%v\n", e.Type)
}

func (e *InterfaceEmbed) setParent(t *TypeSpec) {
	e.parent = t
}

func (e *InterfaceEmbed) qualify(imports *Imports) {
	e.Type = imports.EnsureImported(e.Type)
}

// InterfaceMethod is a method defined inside of an interface. This represents
// an explicit method, as opposed to an interface method that comes from an
// embedded interface.
type InterfaceMethod struct {
	Comment, Name string
	Signature
	parent *TypeSpec
}

// NewInterfaceMethod returns a method with the given name.
func NewInterfaceMethod(name string) *InterfaceMethod {
	return &InterfaceMethod{Name: name}
}

// AddArg adds an argument to the signature of this method. The given name can
// be blank for signatures with unnamed arguments. If the signature has a mix of
// named and unnamed args, they will all be rendered as named, but "_" will be
// used for unnamed ones. This method returns the interface method, for method
// chaining.
func (m *InterfaceMethod) AddArg(name string, t TypeName) *InterfaceMethod {
	m.Signature.AddArg(name, t)
	return m
}

// AddResult adds a result value to the signature of this method. The given name
// can be blank for signatures with unnamed results. If the signature has a mix
// of named and unnamed results, they will all be rendered as named, but "_"
// will be used for unnamed ones. This method returns the interface method, for
// method chaining.
func (m *InterfaceMethod) AddResult(name string, t TypeName) *InterfaceMethod {
	m.Signature.AddResult(name, t)
	return m
}

// SetVariadic sets whether this method's signature is variadic or not. The
// resulting interface method will be invalid if variadic is set to true but
// the last argument is not a slice. This method returns the interface method,
// for method chaining.
func (m *InterfaceMethod) SetVariadic(isVariadic bool) *InterfaceMethod {
	m.Signature.SetVariadic(isVariadic)
	return m
}

// ToTypeName returns a type name with the same type as this method. The
// signature of the returned type varies from this interface method in that it
// will have an extra argument at the beginning of the argument list: the
// method's receiver type.
func (m *InterfaceMethod) ToTypeName() TypeName {
	if m.parent == nil {
		panic(fmt.Sprintf("cannot construct TypeName for method %s because it has not been associated with a type (no known receiver type)", m.Name))
	}
	var sig Signature
	rcvr := ArgType{Type: m.parent.ToTypeName()}
	sig.Args = append([]ArgType{rcvr}, m.Args...)
	sig.Results = append([]ArgType{}, m.Results...)
	sig.IsVariadic = m.IsVariadic
	return FuncTypeFromSig(&sig)
}

// ToMethodRef returns a method reference that can be interpolated into a code
// block to form a method expression in code.
func (m *InterfaceMethod) ToMethodRef() MethodReference {
	if m.parent == nil {
		panic(fmt.Sprintf("cannot construct MethodReference for method %s because it has not been associated with a type", m.Name))
	}
	return MethodReference{
		Type:   m.parent.ToSymbol(),
		Method: m.Name,
	}
}

// String returns the method's name, so if the interface method is used as a
// format argument in a code block it will result in a reference to the method,
// like for a method invocation.
func (m *InterfaceMethod) String() string {
	return m.Name
}

func (m *InterfaceMethod) isInterfaceElement() {}

func (m *InterfaceMethod) writeTo(b *bytes.Buffer) {
	writeComment(m.Comment, b)
	fmt.Fprintf(b, "%s", m.Name)
	signatureToBuffer(&m.Signature, b)
	b.WriteRune('\n')
}

func (m *InterfaceMethod) setParent(t *TypeSpec) {
	m.parent = t
}

func (m *InterfaceMethod) qualify(imports *Imports) {
	newSig := imports.EnsureAllTypesImported(&m.Signature)
	if newSig != &m.Signature {
		m.Signature = *newSig
	}
}

// FuncSpec is a FileElement representing a func or method definition. The Name
// field is not optional. If the function has one or more result values in its
// signature, the code block body should not be empty (or the result will be Go
// code that doesn't compile).
//
// If the FuncSpec has a receiver, then it is a method instead of an ordinary
// function.
type FuncSpec struct {
	Comment, Name string
	Receiver      *ReceiverSpec
	Signature
	CodeBlock
	parent *GoFile
}

// NewFunc creates a new function with the given name.
func NewFunc(name string) *FuncSpec {
	return &FuncSpec{Name: name}
}

// NewMethod creates a new method for the given receiver and with the given
// name.
func NewMethod(rcvr *ReceiverSpec, name string) *FuncSpec {
	return &FuncSpec{Name: name, Receiver: rcvr}
}

// SetComment sets the comment on the function. This method returns the func,
// for method chaining.
func (f *FuncSpec) SetComment(comment string) *FuncSpec {
	f.Comment = comment
	return f
}

// ToSymbol returns a symbol or a method reference that refers to this func. If
// this is a normal func (no receiver) the returned value is a gopoet.Symbol.
// Otherwise, if it is a method, the returned value is a gopoet.MethodReference.
// This method panics if this type has never been added to a file since, without
// an associated file, the package of the symbol is unknown. It can be added to
// a file via GoFile.AddElement.
func (f *FuncSpec) ToSymbol() SymbolOrMethodRef {
	if f.parent == nil {
		panic(fmt.Sprintf("cannot use func %s as symbol because it has not been associated with a file", f.Name))
	}
	if f.Receiver != nil {
		rcvrType := Symbol{
			Name:    f.Receiver.Type,
			Package: f.parent.Package(),
		}
		return MethodReference{
			Type:   rcvrType,
			Method: f.Name,
		}
	}
	return Symbol{
		Name:    f.Name,
		Package: f.parent.Package(),
	}
}

// String returns a string representation of this func that can be interpolated
// into a code block and be a valid reference. To that end, it is the same as
// string representation of the symbol returned from f.ToSymbol() if it is a
// non-method func (e.g. no receiver). If it is a method name, it is just the
// name of the method. This method will not panic: if the func has never been
// added to a file and this is a non-method func, the func's name will be
// returned, without a package qualifier.
func (f *FuncSpec) String() string {
	if f.parent == nil || f.Receiver != nil {
		return f.Name
	}
	return f.ToSymbol().String()
}

// AddArg adds an argument to the signature of this func. The given name can be
// blank for signatures with unnamed arguments. If the signature has a mix of
// named and unnamed args, they will all be rendered as named, but "_" will be
// used for unnamed ones. This method returns the func, for method chaining.
func (f *FuncSpec) AddArg(name string, t TypeName) *FuncSpec {
	f.Signature.AddArg(name, t)
	return f
}

// AddResult adds a result value to the signature of this func. The given name
// can be blank for signatures with unnamed results. If the signature has a
// mix of named and unnamed results, they will all be rendered as named, but "_"
// will be used for unnamed ones. This method returns the func, for method
// chaining.
func (f *FuncSpec) AddResult(name string, t TypeName) *FuncSpec {
	f.Signature.AddResult(name, t)
	return f
}

// SetVariadic sets whether this func's signature is variadic or not. The
// resulting func will be invalid if variadic is set to true but the last
// argument is not a slice. This method returns the interface method, for method
// chaining.
func (f *FuncSpec) SetVariadic(isVariadic bool) *FuncSpec {
	f.Signature.SetVariadic(isVariadic)
	return f
}

// Print adds the given text to the function's body. It returns the func, for
// method chaining.
func (f *FuncSpec) Print(text string) *FuncSpec {
	f.CodeBlock.Print(text)
	return f
}

// Println adds the given text to the function's body, along with a trailing
// newline. It returns the func, for method chaining.
func (f *FuncSpec) Println(text string) *FuncSpec {
	f.CodeBlock.Println(text)
	return f
}

// Printf adds the given text to the function body and will properly qualify any
// supported types in the argument list when rendered. It returns the func, for
// method chaining.
func (f *FuncSpec) Printf(fmt string, args ...interface{}) *FuncSpec {
	f.CodeBlock.Printf(fmt, args...)
	return f
}

// Printlnf adds the given text to the function body and will properly qualify
// any supported types in the argument list when rendered. Similar to Println,
// it adds a trailing newline. It returns the func, for method chaining.
func (f *FuncSpec) Printlnf(fmt string, args ...interface{}) *FuncSpec {
	f.CodeBlock.Printlnf(fmt, args...)
	return f
}

// RenderCode uses the given template and data object to render contents into
// this function's body. See CodeBlock.RenderCode for more details. This method
// returns the func, for method chaining.
func (f *FuncSpec) RenderCode(template *template.Template, data interface{}) *FuncSpec {
	f.CodeBlock.RenderCode(template, data)
	return f
}

// AddCode adds the contents of the given CodeBlock to this function's body. It
// returns the func, for method chaining.
func (f *FuncSpec) AddCode(cb *CodeBlock) *FuncSpec {
	f.CodeBlock.AddCode(cb)
	return f
}

// ToTypeName returns a type name that represents the type of this function. For
// normal (non-method) functions, the returned type has the same signature as
// the function. For methods, the returned type's signature includes an extra
// argument before any others that represents the method's receiver. This
// method panics if this is a method but it has not been added to a file since,
// without an associated file, the package of the receiver type is unknown. It
// can be added to a file via GoFile.AddElement.
func (f *FuncSpec) ToTypeName() TypeName {
	if f.Receiver != nil {
		if f.parent == nil {
			panic(fmt.Sprintf("cannot determine TypeName for method %s because it has not been associated with a file", f.Name))
		}
		rcvrType := NamedType(Symbol{
			Name:    f.Receiver.Type,
			Package: f.parent.Package(),
		})
		if f.Receiver.IsPointer {
			rcvrType = PointerType(rcvrType)
		}
		rcvr := ArgType{Name: f.Receiver.Name, Type: rcvrType}
		sig := f.Signature
		args := make([]ArgType, len(sig.Args)+1)
		args[0] = rcvr
		copy(args[1:], sig.Args)
		sig.Args = args
		sig.Results = f.Results
		sig.IsVariadic = f.IsVariadic
		return FuncTypeFromSig(&sig)
	}

	return FuncTypeFromSig(&f.Signature)
}

func (f *FuncSpec) isFileElement() {}

func (f *FuncSpec) writeTo(b *bytes.Buffer) {
	writeComment(f.Comment, b)
	b.WriteString("func ")
	if f.Receiver != nil {
		r := f.Receiver
		b.WriteRune('(')
		if r.Name != "" {
			b.WriteString(r.Name)
			b.WriteRune(' ')
		}
		if r.IsPointer {
			b.WriteRune('*')
		}
		b.WriteString(r.Type)
		b.WriteString(") ")
	}
	b.WriteString(f.Name)
	signatureToBuffer(&f.Signature, b)
	b.WriteString(" {\n")
	f.CodeBlock.writeTo(b)
	b.WriteString("}\n")
}

func (f *FuncSpec) setParent(file *GoFile) {
	f.parent = file
}

func (f *FuncSpec) qualify(imports *Imports) {
	newSig := imports.EnsureAllTypesImported(&f.Signature)
	if newSig != &f.Signature {
		f.Signature = *newSig
	}
	f.CodeBlock.qualify(imports)
}

// ReceiverSpec describes the receiver for a method definition. Unlike an
// InterfaceMethod, a FuncSpec that represents a method must have explicit
// receiver information. The receiver must reference a named type and can
// be a pointer or not. Since the method must be defined in the same package as
// the type, the receiver need not be a fully-qualified symbol, so a string
// suffices (vs. a gopoet.Symbol, *gopoet.TypeSpec, or gopoet.TypeName).
type ReceiverSpec struct {
	Name      string
	Type      string
	IsPointer bool
}

// NewReceiver returns a new receiver definition with the given name and the
// given type. The type is an unqualified reference to a named type in the same
// package.
func NewReceiver(name, rcvrType string) *ReceiverSpec {
	if rcvrType == "" {
		panic("cannot create a ReceiverSpec with an empty receiver type name")
	}
	return &ReceiverSpec{Name: name, Type: rcvrType}
}

// NewReceiverForType returns a new receiver definition with the given name and
// the given receiver type. If the given receiver type is not a named type or a
// pointer to a named type, this method will panic.
func NewReceiverForType(name string, rcvrType TypeNameOrSpec) *ReceiverSpec {
	var rcvrTypeName string
	isPointer := false
	switch t := rcvrType.(type) {
	case TypeName:
		if t.Kind() == KindNamed {
			rcvrTypeName = t.Symbol().Name
		} else if t.Kind() == KindPtr && t.Elem().Kind() == KindNamed {
			isPointer = true
			rcvrTypeName = t.Elem().Symbol().Name
		} else {
			panic(fmt.Sprintf("cannot create a ReceiverSpec for an unnamed type: %s", t))
		}
	case *TypeSpec:
		if t.Name == "" {
			panic("cannot create a pointer ReceiverSpec for an unnamed type")
		}
		rcvrTypeName = t.Name
	}
	return &ReceiverSpec{Name: name, Type: rcvrTypeName, IsPointer: isPointer}
}

// NewPointerReceiver returns a new receiver definition with the given name
// whose receiver type is a pointer to the given type. The type is an
// unqualified reference to a named type in the same package.
func NewPointerReceiver(name, rcvrType string) *ReceiverSpec {
	return &ReceiverSpec{Name: name, Type: rcvrType, IsPointer: true}
}

// NewPointerReceiverForType returns a new receiver definition with the given
// name whose receiver type is a pointer to the given type. If the given
// receiver type is not a named type, this method will panic.
func NewPointerReceiverForType(name string, rcvrType TypeNameOrSpec) *ReceiverSpec {
	var rcvrTypeName string
	switch t := rcvrType.(type) {
	case TypeName:
		if t.Kind() != KindNamed {
			panic(fmt.Sprintf("cannot create a pointer ReceiverSpec for an unnamed type: %s", t))
		}
		rcvrTypeName = t.Symbol().Name
	case *TypeSpec:
		if t.Name == "" {
			panic("cannot create a pointer ReceiverSpec for an unnamed type")
		}
		rcvrTypeName = t.Name
	}
	return &ReceiverSpec{Name: name, Type: rcvrTypeName, IsPointer: true}
}

// MethodReference is a reference to a method that can be used as a method
// expression in a code block. A method expression is one that references a
// method without a receiver, and the expression's type is a function type where
// the first argument is the method's receiver. For example:
//
//    bytes.Buffer.Write
//
// This is a reference to the Write method for the Buffer type in the "bytes"
// standard package.
type MethodReference struct {
	Type   Symbol
	Method string
}

// String returns the method reference as could be used in Go code as a method
// expression. It is the type's string representation followed by a dot '.'
// and then followed by the method name.
func (r MethodReference) String() string {
	return fmt.Sprintf("%s.%s", r.Type, r.Method)
}

func (r MethodReference) isSymbolOrMethodRef() {}
