# Go Poet

## Still Under Construction!

This library was inspired by [Java Poet](https://github.com/square/javapoet), as a library that provides
a simple language model for generating Go code.

The model it provides is much simpler than the model exposed by `go/ast` and `go/types`, both in its
expressive power and in its usability. It cannot model implementation code, which is handled in the
library with special support for formatting (a la `fmt.Printf`) references to symbols and types. This
support allows it to provide automatic management of import statements: so you just build the model of
the program to generate, and it will emit the program with all of the necessary imports and properly
qualified references in the code.

TODO: more doc
