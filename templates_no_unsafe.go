//+build appengine gopherjs purego
// NB: other environments where unsafe is unappropriate should use "purego" build tag
// https://github.com/golang/go/issues/23172

package gopoet

import (
	"reflect"
)

func getField(v reflect.Value, index int) (reflect.Value, bool) {
	fld := v.Field(index)
	// We can't use unsafe, so return false for unexported fields :(
	return fld, !fld.IsValid() || fld.CanInterface()
}
