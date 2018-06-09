package baz

import "errors"
import "fmt"
import "net/http"
import "strings"

type FooState int64

type FooDetails struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Categories []string `json:"categories"`
}

type IFoo interface {
	fmt.Stringer
	DoIt(http.RoundTripper, bool) error
}

const (
	FOO = FooState(101)
	BAR = FooState(202)
)

var (
	vfoo string
	vmap = map[string]FooState{
		"foo": FOO,
		"bar": BAR,
	}
)

func MakeIt(foo FooState, bar []string, baz func(chan struct{}, ...bool)) error {
	if foo == FOO {
		return errors.New(strings.Join(bar, vfoo))
	}
	bools := make([]bool, len(bar))
	for i := range bar {
		_, bools[i] = vmap[bar[i]]
	}
	ch := make(chan struct{}, 1024)
	baz(ch, bools...)
	return nil
}

func (s FooState) String() string {
	return "foo"
}

func (d *FooDetails) String() string {
	return d.Name
}
