package gopoet

import (
	"bytes"
	"fmt"
	"go/types"
	"reflect"
	"text/template"
)

// CodeBlock represents one or more Go statements or expressions, such as for
// initializing a value (e.g const or var) or for the body of a function or
// method.
//
// Code block contents can be constructed from fmt-like methods on the CodeBlock
// or by rendering a template via the RenderCode method. The format arguments
// supplied to Printf or Printlnf methods can be references to other Go Poet
// types (Package, Symbol, MethodReference, TypeName) including file elements
// (*ConstSpec, *VarSpec, *TypeSpec, *FuncSpec, etc). Such references will be
// properly qualified when the CodeBlock is rendered into generated Go code, so
// that references use the correct package names and aliases.
type CodeBlock struct {
	elements []codeElement
}

// codeElement represents code to render. It will either be a message and format
// arguments, rendered using the "fmt" package, or a template and value,
// rendered using the "text/template" package.
type codeElement interface {
	isCodeElement()
	writeTo(b *bytes.Buffer)
	qualify(imports *Imports)
}

// Print adds the given text to the code block. It returns the code block, for
// method chaining.
func (cb *CodeBlock) Print(text string) *CodeBlock {
	return cb.Printf("%s", text)
}

// Println adds the given text to the code block, along with a trailing newline.
// It returns the code block, for method chaining.
func (cb *CodeBlock) Println(text string) *CodeBlock {
	return cb.Printlnf("%s", text)
}

// Printf adds the given text to the code block and will properly qualify any
// supported types in the argument list when rendered. It returns the code
// block, for method chaining.
func (cb *CodeBlock) Printf(fmt string, args ...interface{}) *CodeBlock {
	cb.elements = append(cb.elements, line{format: fmt, args: args})
	return cb
}

// Printlnf adds the given text to the code block and will properly qualify any
// supported types in the argument list when rendered. Similar to Println, it
// adds a trailing newline. It returns the code block, for method chaining.
func (cb *CodeBlock) Printlnf(fmt string, args ...interface{}) *CodeBlock {
	return cb.Printf(fmt+"\n", args...)
}

// RenderCode uses the given template and data object to render contents. The
// given data object will be re-created during rendering, if necessary, so that
// any supported types therein are updated to use properly qualified references.
// For example, a struct that includes a *TypeSpec will be re-created so that
// the *TypeSpec field is modified to include a package name that matches the
// proper import name/alias for the render context.
//
// This method returns the code block, for method chaining.
func (cb *CodeBlock) RenderCode(template *template.Template, data interface{}) *CodeBlock {
	cb.elements = append(cb.elements, &templ{templ: template, data: data})
	return cb
}

// AddCode adds the contents of the given CodeBlock to this one. It returns the
// code block, for method chaining.
func (cb *CodeBlock) AddCode(code *CodeBlock) *CodeBlock {
	cb.elements = append(cb.elements, code.elements...)
	return cb
}

// Print is a convenience function that allocates a new CodeBlock and calls its
// Print method in one step.
func Print(text string) *CodeBlock {
	return (&CodeBlock{}).Print(text)
}

// Println is a convenience function that allocates a new CodeBlock and calls
// its Println method in one step.
func Println(text string) *CodeBlock {
	return (&CodeBlock{}).Println(text)
}

// Printf is a convenience function that allocates a new CodeBlock and calls its
// Printf method in one step.
func Printf(fmt string, args ...interface{}) *CodeBlock {
	return (&CodeBlock{}).Printf(fmt, args...)
}

// Printlnf is a convenience function that allocates a new CodeBlock and calls
// its Printlnf method in one step.
func Printlnf(fmt string, args ...interface{}) *CodeBlock {
	return (&CodeBlock{}).Printlnf(fmt, args...)
}

// RenderCode is a convenience function that allocates a new CodeBlock and calls
// its RenderCode method in one step.
func RenderCode(template *template.Template, data interface{}) *CodeBlock {
	return (&CodeBlock{}).RenderCode(template, data)
}

func (cb *CodeBlock) qualify(imports *Imports) {
	for _, e := range cb.elements {
		e.qualify(imports)
	}
}

func (cb *CodeBlock) writeTo(b *bytes.Buffer) {
	for _, e := range cb.elements {
		e.writeTo(b)
	}
}

// line represents a message and format arguments, which will be rendered to a
// code block using the "fmt" package.
type line struct {
	format string
	args   []interface{}
}

func (l line) isCodeElement() {}

func (l line) writeTo(b *bytes.Buffer) {
	fmt.Fprintf(b, l.format, l.args...)
}

func (l line) qualify(imports *Imports) {
	for i := range l.args {
		switch a := l.args[i].(type) {
		case Package:
			p := imports.RegisterImportForPackage(a)
			if p != a.Name {
				l.args[i] = Package{Name: p, ImportPath: a.ImportPath}
			}
		case *Package:
			p := imports.RegisterImportForPackage(*a)
			if p != a.Name {
				l.args[i] = Package{Name: p, ImportPath: a.ImportPath}
			}
		case Symbol:
			l.args[i] = imports.EnsureImported(a)
		case *Symbol:
			l.args[i] = imports.EnsureImported(*a)
		case MethodReference:
			sym := imports.EnsureImported(a.Type)
			if sym != a.Type {
				l.args[i] = &MethodReference{Type: sym, Method: a.Method}
			}
		case *MethodReference:
			sym := imports.EnsureImported(a.Type)
			if sym != a.Type {
				l.args[i] = &MethodReference{Type: sym, Method: a.Method}
			}
		case Signature:
			l.args[i] = imports.EnsureAllTypesImported(&a)
		case *Signature:
			l.args[i] = imports.EnsureAllTypesImported(a)
		case TypeName:
			l.args[i] = imports.EnsureTypeImported(a)
		case *ConstSpec:
			l.args[i] = imports.EnsureImported(a.ToSymbol())
		case *VarSpec:
			l.args[i] = imports.EnsureImported(a.ToSymbol())
		case *TypeSpec:
			l.args[i] = imports.EnsureImported(a.ToSymbol())
		case *InterfaceMethod:
			mr := a.ToMethodRef()
			sym := imports.EnsureImported(mr.Type)
			if sym != mr.Type {
				l.args[i] = &MethodReference{Type: sym, Method: mr.Method}
			}
		case *FuncSpec:
			symOrMr := a.ToSymbol()
			switch sm := symOrMr.(type) {
			case Symbol:
				l.args[i] = imports.EnsureImported(sm)
			case MethodReference:
				sym := imports.EnsureImported(sm.Type)
				if sym != sm.Type {
					l.args[i] = &MethodReference{Type: sym, Method: sm.Method}
				}
			}
		case reflect.Type:
			t := TypeNameForReflectType(a)
			l.args[i] = imports.EnsureTypeImported(t)
		case types.Type:
			t := TypeNameForGoType(a)
			l.args[i] = imports.EnsureTypeImported(t)
		case types.Object:
			symOrMr := SymbolForGoObject(a)
			switch sm := symOrMr.(type) {
			case Symbol:
				l.args[i] = imports.EnsureImported(sm)
			case MethodReference:
				sym := imports.EnsureImported(sm.Type)
				if sym != sm.Type {
					l.args[i] = &MethodReference{Type: sym, Method: sm.Method}
				}
			}
		case *types.Package:
			p := PackageForGoType(a)
			l.args[i] = imports.RegisterImportForPackage(p)
		}
	}
}

// templ represents a template and data value, which will be rendered to a code
// block using the "text/template" package.
type templ struct {
	templ *template.Template
	data  interface{}
}

func (t *templ) isCodeElement() {}

func (t *templ) writeTo(b *bytes.Buffer) {
	err := t.templ.Execute(b, t.data)
	if err != nil {
		panic(err.Error())
	}
}

func (t *templ) qualify(imports *Imports) {
	t.data = imports.QualifyTemplateData(t.data)
}
