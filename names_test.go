package gopoet

import "testing"

func TestExport(t *testing.T) {
	checkName(t, Export, "foo", "Foo")
	checkName(t, Export, "Foo", "Foo")
	checkName(t, Export, "_foo", "Xfoo")
	checkName(t, Export, "1foo", "X1foo")
}

func TestUnexport(t *testing.T) {
	checkName(t, Unexport, "foo", "foo")
	checkName(t, Unexport, "Foo", "foo")
	checkName(t, Unexport, "_foo", "_foo")
	checkName(t, Unexport, "1foo", "1foo")
}

func checkName(t *testing.T, fn func(string) string, input, output string) {
	actual := fn(input)
	if actual != output {
		t.Errorf("%q: expected %q; got %q", input, output, actual)
	}
}
