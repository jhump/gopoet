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
	"strconv"
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
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	// and finally emit it
	_, err = w.Write(formatted)
	return err
}

func WriteGoFiles(outFn func(path string) (io.Writer, error), files ...*GoFile) error {
	for _, file := range files {
		p := filepath.Join(file.Package().ImportPath, file.Name)
		w, err := outFn(p)
		if err != nil {
			return err
		}
		if err := WriteGoFile(w, file); err != nil {
			return err
		}
	}
	return nil
}

func WriteGoFilesToFileSystem(rootDir string, files ...*GoFile) error {
	return WriteGoFiles(func(path string) (io.Writer, error) {
		fullPath := filepath.Join(rootDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, err
		}
		return os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)

	}, files...)
}

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

type FileElement interface {
	writeTo(b *bytes.Buffer)
	setParent(*GoFile)
	qualify(imports *Imports)
}

type ConstDecl struct {
	Comment string
	Consts  []*ConstSpec
}

func (c *ConstDecl) writeTo(b *bytes.Buffer) {
	if len(c.Consts) > 1 {
		writeComment(c.Comment, b)
		b.WriteString("const (\n")
		for _, cs := range c.Consts {
			cs.writeTo(b)
		}
		b.WriteString(")\n")
		return
	}

	cs := c.Consts[1]
	b.WriteString("const ")
	cs.writeTo(b)
}

func (c *ConstDecl) setParent(f *GoFile) {
	for _, cnst := range c.Consts {
		cnst.parent = f
	}
}

func (c *ConstDecl) qualify(imports *Imports) {
	for _, cs := range c.Consts {
		cs.Type = imports.EnsureTypeImported(cs.Type)
		if cs.Initializer != nil {
			cs.Initializer.qualify(imports)
		}
	}
}

type ConstSpec struct {
	Comment, Name string
	Type          TypeName
	Initializer   *CodeBlock
	parent        *GoFile
}

func (c *ConstSpec) ToSymbol() *Symbol {
	if c.parent == nil {
		panic(fmt.Sprintf("cannot use const %s %v as symbol because it has not been associated with a file", c.Name, c.Type))
	}
	return &Symbol{
		Name:    c.Name,
		Package: c.parent.Package(),
	}
}

func (c *ConstSpec) writeTo(b *bytes.Buffer) {
	writeComment(c.Comment, b)
	fmt.Fprintf(b, "%s", c.Name)
	if c.Type != nil {
		fmt.Fprintf(b, " %v", c.Type)
	}
	if c.Initializer != nil {
		b.WriteString(" = ")
		c.Initializer.writeTo(b)
	}
}

type VarDecl struct {
	Comment string
	Vars    []*VarSpec
}

func (v *VarDecl) writeTo(b *bytes.Buffer) {
	if len(v.Vars) > 1 {
		writeComment(v.Comment, b)
		b.WriteString("var (\n")
		for _, vs := range v.Vars {
			vs.writeTo(b)
		}
		b.WriteString(")\n")
		return
	}

	vs := v.Vars[1]
	b.WriteString("var ")
	vs.writeTo(b)
}

func (v *VarDecl) setParent(f *GoFile) {
	for _, vr := range v.Vars {
		vr.parent = f
	}
}

func (v *VarDecl) qualify(imports *Imports) {
	for _, vs := range v.Vars {
		vs.Type = imports.EnsureTypeImported(vs.Type)
		if vs.Initializer != nil {
			vs.Initializer.qualify(imports)
		}
	}
}

type VarSpec struct {
	Comment     string
	Names       []string
	Type        TypeName
	Initializer *CodeBlock
	parent      *GoFile
}

func (v *VarSpec) ToSymbol() *Symbol {
	if len(v.Names) != 1 {
		panic(fmt.Sprintf("must be exactly one var to use ToSymbol(); use SymbolAt(int) instead: %v", v.Names))
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
}

type TypeDecl struct {
	Comment string
	Types   []*TypeSpec
}

func (t *TypeDecl) writeTo(b *bytes.Buffer) {
	if len(t.Types) > 1 {
		writeComment(t.Comment, b)
		b.WriteString("type (\n")
		for _, ts := range t.Types {
			ts.writeTo(b)
		}
		b.WriteString(")\n")
		return
	}

	ts := t.Types[1]
	b.WriteString("type ")
	ts.writeTo(b)
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

type TypeSpec struct {
	Comment, Name string
	isAlias       bool
	underlying    TypeName
	structBody    []*FieldSpec
	interfaceBody []InterfaceElement
	methods       []*MethodSpec
	parent        *GoFile
}

func (t *TypeSpec) ToSymbol() *Symbol {
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
	for _, ms := range t.methods {
		ms.qualify(imports)
	}
}

func (t *TypeSpec) ToTypeName() TypeName {
	return NamedType(t.ToSymbol())
}

func (t *TypeSpec) Underlying() TypeName {
	return t.underlying
}

func (t *TypeSpec) IsAlias() bool {
	return t.isAlias
}

func (t *TypeSpec) writeTo(b *bytes.Buffer) {
	writeComment(t.Comment, b)
	fmt.Fprintf(b, "%s ", t.Name)
	t.writeUnderlying(b)

	// we only emit methods for top-level types
	for _, ms := range t.methods {
		b.WriteRune('\n')
		ms.writeTo(b)
	}
}

func (t *TypeSpec) writeUnderlying(b *bytes.Buffer) {
	switch t.underlying.Kind() {
	case KindStruct:
		b.WriteString(" struct {\n")
		for _, f := range t.structBody {
			f.writeTo(b)
		}
		fmt.Fprintf(b, "}\n")
	case KindInterface:
		b.WriteString(" interface {\n")
		for _, el := range t.interfaceBody {
			el.writeTo(b)
		}
		fmt.Fprintf(b, "}\n")
	default:
		fmt.Fprintf(b, " %v", t.underlying)
	}
}

func NewTypeAlias(typeName string, t TypeName) *TypeSpec {
	ts := NewTypeSpec(typeName, t)
	ts.isAlias = true
	return ts
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
}

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

func NewInterfaceTypeSpec(typeName string, elements ...InterfaceElement) *TypeSpec {
	ret := &TypeSpec{
		Name:          typeName,
		interfaceBody: elements,
	}
	var embeds []*Symbol
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

type TypeNameOrSpec interface {
	isTypeNameOrSpec()
}

type FieldSpec struct {
	Comment, Name string
	Tag           reflect.StructTag
	Type		  TypeNameOrSpec
	parent *TypeSpec
}

func (f *FieldSpec) writeTo(b *bytes.Buffer) {
	writeComment(f.Comment, b)
	fmt.Fprintf(b, "%s ", f.Name)

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
}

type InterfaceElement interface {
	writeTo(b *bytes.Buffer)
	setParent(*TypeSpec)
	qualify(*Imports)
}

type InterfaceEmbed struct {
	Comment string
	Type    *Symbol
	parent  *TypeSpec
}

func NewInterfaceEmbed(s *Symbol) InterfaceEmbed {
	return InterfaceEmbed{Type: s}
}

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

type InterfaceMethod struct {
	Comment, Name string
	Signature
	parent *TypeSpec
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

type FuncSpec struct {
	Comment, Name string
	Signature
	CodeBlock
	parent *GoFile
}

func (f *FuncSpec) ToSymbol() *Symbol {
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

func (f *FuncSpec) ToTypeName() TypeName {
	return FuncTypeFromSig(&f.Signature)
}

func (f *FuncSpec) writeTo(b *bytes.Buffer) {
	writeComment(f.Comment, b)
	fmt.Fprintf(b, "func %s", f.Name)
	signatureToBuffer(&f.Signature, b)
	b.WriteString(" {\n")
	f.CodeBlock.writeTo(b)
	b.WriteString("}\n")
}

func (f *FuncSpec) qualify(imports *Imports) {
	newSig := imports.EnsureAllTypesImported(&f.Signature)
	if newSig != &f.Signature {
		f.Signature = *newSig
	}
	f.CodeBlock.qualify(imports)
}

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

func (m *MethodSpec) ToTypeName() TypeName {
	if m.parent == nil {
		panic(fmt.Sprintf("cannot construct TypeName for method %s because it has not been associated with a type (no known receiver type)", m.Name))
	}
	var sig Signature
	rcvType := m.parent.ToTypeName()
	if m.ReceiverIsPointer {
		rcvType = PointerType(rcvType)
	}
	rcvr := ArgType{Name: m.ReceiverName, Type: rcvType}
	sig.Args = append([]ArgType{rcvr}, m.Args...)
	sig.Results = append([]ArgType{}, m.Results...)
	sig.IsVariadic = m.IsVariadic
	return FuncTypeFromSig(&sig)
}

func (m *MethodSpec) ToMethodRef() *MethodReference {
	if m.parent == nil {
		panic(fmt.Sprintf("cannot construct MethodReference for method %s because it has not been associated with a type", m.Name))
	}
	return &MethodReference{
		Type:   m.parent.ToSymbol(),
		Method: m.Name,
	}
}

func (m *MethodSpec) writeTo(b *bytes.Buffer) {
	writeComment(m.Comment, b)
	b.WriteString("func (")
	if m.ReceiverName != "" {
		fmt.Fprintf(b, "%s ", m.ReceiverName)
	}
	if m.ReceiverIsPointer {
		b.WriteRune('*')
	}
	b.WriteString(m.parent.Name)
	fmt.Fprintf(b, ") %s", m.Name)
	signatureToBuffer(&m.Signature, b)
	b.WriteString(" {\n")
	m.CodeBlock.writeTo(b)
	b.WriteString("}\n")
}

type MethodReference struct {
	Type   *Symbol
	Method string
}