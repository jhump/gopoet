package gopoet

import (
	"bytes"
	"fmt"
	"path"

	"go/types"
)

// Package is a simple representation of a Go package. The name may actually be
// an effective alias (when the package is imported using an alias). A symbol
// whose package has an empty Name is a local symbol: in the same package as the
// referencing context (and thus needs no package prefix to reference that
// symbol).
type Package struct {
	ImportPath, Name string
}

func (p Package) Symbol(name string) Symbol {
	return Symbol{Name: name, Package: p}
}

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

// String prints the symbol as it should appear in Go source: pkg.Name. The
// "pkg." prefix will be omitted if the symbol's Package has an empty Name.
func (s Symbol) String() string {
	if s.Package.Name != "" {
		return s.Package.Name + "." + s.Name
	}
	return s.Name
}

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
	Name                string
	// Doc comments that will appear before the package declaration.
	PackageComment      string
	// The package name that will be used in the package declaration.
	PackageName         string
	// The canonical import path which, if non-empty, will be used in an
	// annotation comment immediately following the package declaration.
	CanonicalImportPath string
	// The actual elements declared in the source file.
	elements            []FileElement
}

func NewGoFile(fileName, packagePath, packageName string) *GoFile {
	if path.Base(fileName) != fileName {
		panic("Go file name must be a base name with no path")
	}
	if path.Ext(fileName) != ".go" {
		panic("Go file name must have a '.go' extension")
	}

	ret := GoFile{Name: fileName, PackageName: packageName}
	ret.Imports = *NewImportsFor(packagePath)
	return &ret
}

func (f *GoFile) AddElement(e FileElement) *GoFile {
	e.setParent(f)
	f.elements = append(f.elements, e)
	return f
}

func (f *GoFile) AddConst(c *ConstSpec) *GoFile {
	return f.AddElement(&ConstDecl{Consts: []*ConstSpec{c}})
}

func (f *GoFile) AddVar(v *VarSpec) *GoFile {
	return f.AddElement(&VarDecl{Vars: []*VarSpec{v}})
}

func (f *GoFile) AddType(t *TypeSpec) *GoFile {
	return f.AddElement(&TypeDecl{Types: []*TypeSpec{t}})
}

func (f *GoFile) NumElements() int {
	return len(f.elements)
}

func (f *GoFile) ElementAt(i int) FileElement {
	return f.elements[i]
}

func (f *GoFile) Package() Package {
	return Package{
		Name:       f.PackageName,
		ImportPath: f.Imports.pkgPath,
	}
}

type writerToBuffer interface {
	writeTo(b *bytes.Buffer, indent string)
}

type FileElement interface {
	writerToBuffer
	setParent(*GoFile)
}

type ConstDecl struct {
	Comment string
	Consts  []*ConstSpec
}

func (c *ConstDecl) writeTo(b *bytes.Buffer, indent string) {
	// TODO
}

func (c *ConstDecl) setParent(f *GoFile) {
	for _, cnst := range c.Consts {
		cnst.parent = f
	}
}

type ConstSpec struct {
	Comment, Name string
	Type          TypeName
	Initializer   *CodeBlock
	parent        *GoFile
}

func (c *ConstSpec) Symbol() *Symbol {
	if c.parent == nil {
		panic(fmt.Sprintf("cannot use const %s %v as symbol because it has not been associated with a file", c.Name, c.Type))
	}
	return &Symbol{
		Name:    c.Name,
		Package: c.parent.Package(),
	}
}

type VarDecl struct {
	Comment string
	Vars    []*VarSpec
}

func (*VarDecl) writeTo(b *bytes.Buffer, indent string) {
	// TODO
}

func (v *VarDecl) setParent(f *GoFile) {
	for _, vr := range v.Vars {
		vr.parent = f
	}
}

type VarSpec struct {
	Comment     string
	Names       []string
	Type        TypeName
	Initializer *CodeBlock
	parent      *GoFile
}

func (v *VarSpec) Symbol() *Symbol {
	if len(v.Names) != 1 {
		panic("must be exactly one var to use Symbol(); use SymbolAt(int) instead")
	}
	return v.SymbolAt(0)
}

func (v *VarSpec) SymbolAt(i int) *Symbol {
	if v.parent == nil {
		panic(fmt.Sprintf("cannot use var %s %v as symbol because it has not been associated with a file", v.Names[i], v.Type))
	}
	return &Symbol{
		Name:    v.Names[i],
		Package: v.parent.Package(),
	}
}

type TypeDecl struct {
	Comment string
	Types   []*TypeSpec
}

func (*TypeDecl) writeTo(b *bytes.Buffer, indent string) {
	// TODO
}

func (t *TypeDecl) setParent(f *GoFile) {
	for _, ts := range t.Types {
		ts.setParent(f)
	}
}

type TypeSpec struct {
	Comment, Name string
	isAlias       bool
	underlying    TypeName
	structBody    []*FieldSpec
	interfaceBody []InterfaceElement
	methods       []*MethodSpec
	parent        *GoFile
}

func (t *TypeSpec) Symbol() *Symbol {
	if t.parent == nil {
		panic(fmt.Sprintf("cannot use type %s as symbol because it has not been associated with a file", t.Name))
	}
	return &Symbol{
		Name:    t.Name,
		Package: t.parent.Package(),
	}
}

func (t *TypeSpec) AddMethod(m *MethodSpec) *TypeSpec {
	m.parent = t
	t.methods = append(t.methods, m)
	return t
}

func (t *TypeSpec) setParent(f *GoFile) {
	t.parent = f
}

func (t *TypeSpec) TypeName() TypeName {
	return NamedType(t.Symbol())
}

func (t *TypeSpec) Underlying() TypeName {
	return t.underlying
}

func NewTypeSpec(typeName string, t TypeName) *TypeSpec {
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
				parent:    ret,
				FieldType: ft,
			}
		}
		ret.structBody = fields
	case KindInterface:
		embeds := t.Embeds()
		methods := t.Methods()
		elements := make([]InterfaceElement, len(embeds) + len(methods))
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
}

func NewStructTypeSpec(typeName string, fields ...*FieldSpec) *TypeSpec {
	ret := &TypeSpec{
		Name:       typeName,
		structBody: fields,
	}
	fieldTypes := make([]FieldType, len(fields))
	for i, f := range fields {
		f.parent = ret
		fieldTypes[i] = f.FieldType
	}
	ret.underlying = StructType(fieldTypes...)
	return ret
}

func NewInterfaceTypeSpec(typeName string, elements ...InterfaceElement) *TypeSpec {
	ret := &TypeSpec{
		Name:          typeName,
		interfaceBody: elements,
	}
	var embeds []TypeName
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

type FieldSpec struct {
	Comment string
	FieldType
	parent  *TypeSpec
}

type InterfaceElement interface {
	writerToBuffer
	setParent(*TypeSpec)
}

type InterfaceEmbed struct {
	Comment string
	Type    TypeName
	parent  *TypeSpec
}

func NewInterfaceEmbed(t TypeName) InterfaceEmbed {
	return InterfaceEmbed{Type: t}
}

func (e *InterfaceEmbed) writeTo(b *bytes.Buffer, indent string) {
	// TODO
}

func (e *InterfaceEmbed) setParent(t *TypeSpec) {
	e.parent = t
}

type InterfaceMethod struct {
	Comment, Name string
	Signature
	parent        *TypeSpec
}

func NewInterfaceMethod(name string) *InterfaceMethod {
	return &InterfaceMethod{Name: name}
}

func (m *InterfaceMethod) AddArg(name string, t TypeName) *InterfaceMethod {
	m.Signature.AddArg(name, t)
	return m
}

func (m *InterfaceMethod) AddResult(name string, t TypeName) *InterfaceMethod {
	m.Signature.AddResult(name, t)
	return m
}

func (m *InterfaceMethod) SetVariadic(isVariadic bool) *InterfaceMethod {
	m.Signature.SetVariadic(isVariadic)
	return m
}

func (m *InterfaceMethod) writeTo(b *bytes.Buffer, indent string) {
	// TODO
}

func (m *InterfaceMethod) setParent(t *TypeSpec) {
	m.parent = t
}

type FuncSpec struct {
	Comment, Name string
	Signature
	CodeBlock
	parent        *GoFile
}

func (f *FuncSpec) Symbol() *Symbol {
	if f.parent == nil {
		panic(fmt.Sprintf("cannot use func %s as symbol because it has not been associated with a file", f.Name))
	}
	return &Symbol{
		Name:    f.Name,
		Package: f.parent.Package(),
	}
}

func (f *FuncSpec) AddArg(name string, t TypeName) *FuncSpec {
	f.Signature.AddArg(name, t)
	return f
}

func (f *FuncSpec) AddResult(name string, t TypeName) *FuncSpec {
	f.Signature.AddResult(name, t)
	return f
}

func (f *FuncSpec) SetVariadic(isVariadic bool) *FuncSpec {
	f.Signature.SetVariadic(isVariadic)
	return f
}

func (f *FuncSpec) writeTo(b *bytes.Buffer, indent string) {
	// TODO
}

func (*FuncSpec) isFileElement() {}

type MethodSpec struct {
	FuncSpec
	ReceiverName      string
	ReceiverIsPointer bool
	parent            *TypeSpec
}

func (m *MethodSpec) AddArg(name string, t TypeName) *MethodSpec {
	m.Signature.AddArg(name, t)
	return m
}

func (m *MethodSpec) AddResult(name string, t TypeName) *MethodSpec {
	m.Signature.AddResult(name, t)
	return m
}

func (m *MethodSpec) SetVariadic(isVariadic bool) *MethodSpec {
	m.Signature.SetVariadic(isVariadic)
	return m
}

func (m *MethodSpec) writeTo(b *bytes.Buffer, indent string) {
	// TODO
}
