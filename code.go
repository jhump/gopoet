package gopoet

import (
	"bytes"
	"fmt"
)

type CodeBlock struct {
	lines      []line
}

func (cb *CodeBlock) Print(text string) *CodeBlock {
	return cb.Printf("%s", text)
}

func (cb *CodeBlock) Println(text string) *CodeBlock {
	return cb.Printlnf("%s", text)
}

func (cb *CodeBlock) Printf(fmt string, args ...interface{}) *CodeBlock {
	cb.lines = append(cb.lines, line{format: fmt, args: args})
	return cb
}

func (cb *CodeBlock) Printlnf(fmt string, args ...interface{}) *CodeBlock {
	return cb.Printf(fmt+"\n", args...)
}

func Print(text string) *CodeBlock {
	return (&CodeBlock{}).Print(text)
}

func Println(text string) *CodeBlock {
	return (&CodeBlock{}).Println(text)
}

func Printf(fmt string, args ...interface{}) *CodeBlock {
	return (&CodeBlock{}).Printf(fmt, args)
}

func Printlnf(fmt string, args ...interface{}) *CodeBlock {
	return (&CodeBlock{}).Printlnf(fmt, args)
}

func (cb *CodeBlock) qualify(imports *Imports) {
	for _, l := range cb.lines {
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
				l.args[i] = imports.EnsureImported(&a)
			case *Symbol:
				l.args[i] = imports.EnsureImported(a)
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
			case interface{ToSymbol() *Symbol}:
				l.args[i] = imports.EnsureImported(a.ToSymbol())
			case interface{ToMethodReference() *MethodReference}:
				mr := a.ToMethodReference()
				sym := imports.EnsureImported(mr.Type)
				l.args[i] = &MethodReference{Type: sym, Method: mr.Method}
			}
		}
	}
}

func (cb *CodeBlock) writeTo(b *bytes.Buffer) {
	for _, line := range cb.lines {
		fmt.Fprintf(b, line.format, line.args...)
	}
}

func writeIndent(b *bytes.Buffer, indent int) {
	for i := 0; i < indent; i++ {
		b.WriteRune('\t')
	}
}

type line struct {
	format string
	args   []interface{}
}
