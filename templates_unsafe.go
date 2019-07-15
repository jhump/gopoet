//+build !appengine,!gopherjs,!purego
// NB: other environments where unsafe is unappropriate should use "purego" build tag
// https://github.com/golang/go/issues/23172

package gopoet

import (
	"reflect"
	"unsafe"
)

func getField(v reflect.Value, index int) (reflect.Value, bool) {
	fld := v.Field(index)
	if !fld.IsValid() || fld.CanInterface() {
		return fld, true
	}

	// NB: We are being super-sneaky. Go reflection will not let us call
	// fld.Interface() if fld was obtained via unexported fields (which it
	// was!). So we use unsafe to create an alternate reflect.Value instance
	// that represents the same value (same type and address). We can then
	// call Interface() on *that*.
	val := reflect.NewAt(fld.Type(), unsafe.Pointer(fld.UnsafeAddr())).Elem()
	return val, true
}
