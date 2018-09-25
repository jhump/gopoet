package gopoet

import (
	"go/types"
	"reflect"
)

func qualifyTemplateData(imports *Imports, data reflect.Value) (reflect.Value, bool) {
	switch data.Kind() {
	case reflect.Interface:
		if data.Elem().IsValid() {
			var newRv reflect.Value
			switch d := data.Interface().(type) {
			case TypeName:
				newT := imports.EnsureTypeImported(d)
				if newT != d {
					newRv = reflect.ValueOf(newT)
				}
			case reflect.Type:
				origT := TypeNameForReflectType(d)
				newT := imports.EnsureTypeImported(origT)
				if newT != origT {
					newRv = reflect.ValueOf(newT)
				}
			case types.Type:
				origT := TypeNameForGoType(d)
				newT := imports.EnsureTypeImported(origT)
				if newT != origT {
					newRv = reflect.ValueOf(newT)
				}
			case types.Object:
				sym := SymbolForGoObject(d)
				switch sym := sym.(type) {
				case Symbol:
					newSym := imports.EnsureImported(sym)
					if newSym != sym {
						newRv = reflect.ValueOf(newSym)
					}
				case MethodReference:
					newSym := imports.EnsureImported(sym.Type)
					if newSym != sym.Type {
						newRv = reflect.ValueOf(&MethodReference{Type: newSym, Method: sym.Method})
					}
				}
			case *types.Package:
				origP := PackageForGoType(d)
				newP := imports.RegisterImportForPackage(origP)
				if newP != origP.Name {
					newRv = reflect.ValueOf(Package{Name: newP, ImportPath: origP.ImportPath})
				}
			}

			if newRv.IsValid() && newRv.Type().Implements(data.Type()) {
				// For TypeName, this should always be true; but for cases
				// where we've changed the type of the value, if we try to
				// return an incompatible type, the result will be a panic
				// with a location and message that is not awesome for
				// users of this package. So we'll ignore the new value if
				// it's not the right type.
				return newRv, true
			}

			return qualifyTemplateData(imports, data.Elem())
		}

	case reflect.Struct:
		switch t := data.Interface().(type) {
		case Package:
			p := imports.RegisterImportForPackage(t)
			if p != t.Name {
				return reflect.ValueOf(&Package{Name: p, ImportPath: t.ImportPath}).Elem(), true
			}
		case Symbol:
			newSym := imports.EnsureImported(t)
			if newSym != t {
				return reflect.ValueOf(&newSym).Elem(), true
			}
		case MethodReference:
			newSym := imports.EnsureImported(t.Type)
			if newSym != t.Type {
				return reflect.ValueOf(&MethodReference{Type: newSym, Method: t.Method}).Elem(), true
			}
		case Signature:
			oldSig := &t
			newSig := imports.EnsureAllTypesImported(oldSig)
			if newSig != oldSig {
				return reflect.ValueOf(newSig).Elem(), true
			}
		case ConstSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newCs := t
					newCs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newCs).Elem(), true
				}
			}
		case VarSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newVs := t
					newVs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newVs).Elem(), true
				}
			}
		case TypeSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newTs := t
					newTs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newTs).Elem(), true
				}
			}
		case FuncSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newFs := t
					newFs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newFs).Elem(), true
				}
			}
		case InterfaceEmbed:
			newEmbed := t
			if newEmbed.qualify(imports) {
				return reflect.ValueOf(&newEmbed).Elem(), true
			}
		case InterfaceMethod:
			newMethod := t
			if newMethod.qualify(imports) {
				return reflect.ValueOf(&newMethod).Elem(), true
			}
		case Imports:
			// intentionally do not touch these
		default:
			var newStruct reflect.Value
			for i := 0; i < data.NumField(); i++ {
				newV, changedV := qualifyTemplateData(imports, data.Field(i))
				if newStruct.IsValid() {
					newStruct.Field(i).Set(newV)
				} else if changedV {
					newStruct = reflect.New(data.Type()).Elem()
					for j := 0; j < i; j++ {
						newStruct.Field(j).Set(data.Field(j))
					}
					newStruct.Field(i).Set(newV)
				}
			}
			if newStruct.IsValid() {
				return newStruct, true
			}
		}

	case reflect.Ptr:
		if newElem, changed := qualifyTemplateData(imports, data.Elem()); changed {
			if newElem.CanAddr() {
				return newElem.Addr(), true
			}
			dest := reflect.New(newElem.Type())
			dest.Elem().Set(newElem)
			return dest, true
		}

	case reflect.Array, reflect.Slice:
		var newArray reflect.Value
		for i := 0; i < data.Len(); i++ {
			newV, changedV := qualifyTemplateData(imports, data.Index(i))
			if newArray.IsValid() {
				newArray.Index(i).Set(newV)
			} else if changedV {
				if data.Kind() == reflect.Array {
					newArray = reflect.New(data.Type()).Elem()
				} else {
					newArray = reflect.MakeSlice(data.Type(), data.Len(), data.Len())
				}
				reflect.Copy(newArray, data.Slice(0, i))
				newArray.Index(i).Set(newV)
			}
		}
		if newArray.IsValid() {
			return newArray, true
		}

	case reflect.Map:
		var newMap reflect.Value
		seenKeys := make([]reflect.Value, 0, data.Len())
		for _, k := range data.MapKeys() {
			newK, changedK := qualifyTemplateData(imports, k)
			newV, changedV := qualifyTemplateData(imports, data.MapIndex(k))
			if newMap.IsValid() {
				newMap.SetMapIndex(newK, newV)
			} else if changedK || changedV {
				newMap = reflect.MakeMap(data.Type())
				for _, sk := range seenKeys {
					newMap.SetMapIndex(sk, data.MapIndex(sk))
				}
				newMap.SetMapIndex(newK, newV)
				seenKeys = nil
			} else {
				seenKeys = append(seenKeys, k)
			}
		}
		if newMap.IsValid() {
			return newMap, true
		}
	}

	return data, false
}
