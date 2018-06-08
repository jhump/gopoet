// Package gopoet is a library to assist with generating Go code. It includes a
// model of the Go language that is simpler, and thus easier to work with, than
// those provided by the "go/ast" and "go/types" packages. It also provides
// adapter methods to allow simple interoperability with elements from the
// "go/types" and "reflect" packages.
//
// The Go Poet API and functionality is strongly influenced by a similar library
// for Java (for generating Java code) named Java Poet
// (https://github.com/square/javapoet).
//
// Types
//
// TypeName is the way Go Poet represents Go types. There is API in this package
// for constructing TypeName instances and for converting type representations
// from the "go/types" and "reflect" packages to TypeName values. It includes
// related types for representing function signatures, struct fields, and
// interface methods (Signature, FieldSpec, and MethodSpec respectively).
//
// Elements
//
// GoFile is the root type in Go Poet for building a representation of Go
// language elements. The GoFile represents the file itself. The FileElement
// (and its various concrete implementations) represent top-level declarations
// in the file. And types like FieldSpec, InterfaceEmbed, and InterfaceMethod
// represent the elements that comprise struct and interface type definitions.
//
// Statements and expressions are not modeled by the Go Poet API, so function
// bodies and const and var initializers are represented with a type named
// CodeBlock.
//
// Usage of Go Poet involves constructing a GoFile, filling it with elements,
// and then using the various WriteGoFile* methods to then translate these
// models into Go source code.
//
// Imports
//
// Import statements need not be defined manually. GoFile embeds a type named
// Imports which assists with managing import statements. It tracks all packages
// that are referenced, generating import aliases as necessary in the event of
// conflicts. After all referenced packages have been resolved, gopoet.Imports
// can then generate the import statements necessary. It also provides API for
// re-writing various references, to adjust their package qualifier so that
// references to elements or types in other packages are interpolated into Go
// source code with the correct qualifiers.
//
// Packages, Symbols, and Method References
//
// The lowest level building blocks for the above API are representations of
// packages, symbols (references to named package-level elements, like consts,
// vars, types, and funcs), and method references (like a func symbol, but also
// includes a type qualifier, not just a package qualifier).
//
// Various parts of the API provide methods for accessing/converting to these
// types. Under the hood, it is packages and symbols that are re-written by an
// Imports instance to ensure all referenced elements are rendered with the
// package qualifier (e.g. the package name or associated import alias).
//
// Model Limitations
//
// Go Poet does not attempt to model Go statements and expressions or provide
// any way to create structured representations of function and method bodies.
// This is very similar to Java Poet *except* that Go Poet does not provide a
// custom mechanism for printing and formatting code. It instead relies on the
// existing facilities in the "fmt" and "text/template" packages. This package
// provides several types for modeling elements of the Go language that can then
// be referenced in code blocks (via "%s" or "%v" format specifiers or as
// elements of a data value rendered by a template).
//
// The CodeBlock type and related methods include API that resembles the various
// Print* functions in the "fmt" package. Before these are rendered to source
// code, references to Go elements and types are translated to account for the
// import statements (and any associated aliases) for the file context into
// which they are being rendered. Format arguments can also include instances of
// reflect.Type or even items from the "go/types" package: types.Type,
// types.Object, and *types.Package. These types of values will result in proper
// references to these elements when the code is actually rendered.
//
// Similarly, templates can be rendered, and the data value supplied to the
// template will be reconstructed, with any elements therein being first
// translated to have the right package qualifiers.
//
// Templates
//
// As described above, code blocks (which represent function and method bodies
// and initializer expressions) can be rendered from templates and provided data
// values that the template renders.
//
// It is also possible to completely eschew modeling generated code with various
// elements and to generate a file completely from a template. In this case, you
// can still get value from Go Poet by using a *gopoet.Imports type to track
// imported packages and assign aliases, and then render the resulting
// []gopoet.ImportSpec from your template. Furthermore, the value that the
// template renders can contain instances of gopoet.TypeName, gopoet.Package,
// and gopoet.Symbol, just like when rendering code blocks for function bodies
// Calling imports.QualifyTemplateData(data) will re-write the values in the
// data value so they are properly qualified per the imported packages. Do this
// before rendering the template.
//
// One limitation of re-writing template data is that it cannot change the
// *types* of elements except in limited circumstances. For example, a Type from
// the "go/types" package cannot be converted to a gopoet.TypeName if the
// reference is a struct field whose type is types.Type (since gopoet.TypeName
// does not implement types.Type). Because of this, not all referenced types
// and elements can be re-written so may not be rendered correctly. For this
// reason, it is recommended to use gopoet.TypeName as the means of referring to
// types in a template data value, not types.Type or reflect.Type.
package gopoet
