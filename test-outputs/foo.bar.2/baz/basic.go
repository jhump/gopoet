package baz

import "errors"
import "fmt"
import "net/http"
import "strings"

// FooState is the state of a foo
type FooState int64

// FooDetails provide the gory details for your foo.
type FooDetails struct {
	// ID is a unique identifier for this foo
	ID int64 `json:"id"`
	// Name is the display name for this foo
	Name string `json:"name"`
	// Categories is a slice of category names
	Categories []string `json:"categories"`
}

// IFoo is for producing frobnitzes.
type IFoo interface {
	// All IFoos can print to string.
	fmt.Stringer
	// DoIt massages the foo into producing one or more frobnitz.
	DoIt(http.RoundTripper, bool) error
}

// some constants
const (
	// The state that is fooey.
	FOO = FooState(101)
	// The state that is barry.
	BAR = FooState(202)
)

// some variables
var (
	// Global variable that holds a foo string.
	vfoo string
	// Variable that is a mapping of strings to foo states.
	vmap = map[string]FooState{
		"foo": FOO,
		"bar": BAR,
	}
)

// MakeIt does a bunch of silly things, possibly returning an error.
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

// String implements the fmt.Stringer interface
func (s FooState) String() string {
	return "foo"
}

// String implements the fmt.Stringer interface
func (d *FooDetails) String() string {
	return d.Name
}
