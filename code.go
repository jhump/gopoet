package gopoet

type CodeBlock struct {
	lines      []line
	currIndent int
}

func (cb *CodeBlock) Print(text string) *CodeBlock {
	return cb.Printf("%s", text)
}

func (cb *CodeBlock) Println(text string) *CodeBlock {
	return cb.Printlnf("%s", text)
}

func (cb *CodeBlock) Printf(fmt string, args ...interface{}) *CodeBlock {
	cb.lines = append(cb.lines, line{indent: cb.currIndent, format: fmt, args: args})
	return cb
}

func (cb *CodeBlock) Printlnf(fmt string, args ...interface{}) *CodeBlock {
	return cb.Printf(fmt + "\n", args...)
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

func (cb *CodeBlock) Indent() *CodeBlock {
	cb.currIndent++
	return cb
}

func (cb *CodeBlock) Unindent() *CodeBlock {
	if cb.currIndent == 0 {
		panic("cannot unindent: current indent level is zero")
	}
	cb.currIndent--
	return cb
}

type line struct {
	indent int
	format string
	args   []interface{}
}
