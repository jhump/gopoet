package gopoet

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Unexport returns an unexported form of the given name. It does not try to
// verify that s is a valid symbol in Go nor does it check whether s is already
// unexported.
func Unexport(s string) string {
	if s == "" {
		return s
	}
	r, sz := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		panic(fmt.Sprintf("%q is not valid UTF8", s))
	}
	r = unicode.ToLower(r)
	return string(r) + s[sz:]
}

// Export returns an exported form of the given name. It does not try to verify
// that s is a valid symbol in Go nor does it check whether s is already
// exported. If s begins with an underscore, it will be replaced with a capital
// "X". If s begins with an invalid character (such as a number, which is not
// allowed as the first character of an identifier), the returned string will
// be prefixed with "X".
func Export(s string) string {
	if s == "" {
		return s
	}
	r, sz := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		panic(fmt.Sprintf("%q is not valid UTF8", s))
	}
	upperR := unicode.ToUpper(r)
	if upperR == r {
		if r == '_' {
			return "X" + s[sz:]
		}
		// s does not start with a valid identifier character :(
		// just prefix with capital X to make returned string exported
		return "X" + s
	}
	return string(upperR) + s[sz:]
}
