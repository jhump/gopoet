package gopoet

import "reflect"

var typeOfTypeName = reflect.TypeOf((*TypeName)(nil)).Elem()

func qualifyTemplateData(imports *Imports, data reflect.Value) (reflect.Value, bool) {
	switch data.Kind() {
	case reflect.Interface:
		if data.Elem().Type().Implements(typeOfTypeName) {
			origT := data.Interface().(TypeName)
			newT := imports.EnsureTypeImported(origT)
			newRv := reflect.ValueOf(newT)
			if newRv.Type().Implements(data.Type()) {
				// should always be true; but if not (e.g. user-provided
				// TypeName impl that provides broader impl than qualified form)
				// it would panic with a strange error and stack trace, so just
				// fall through below...
				return reflect.ValueOf(newT), origT != newT
			}
		}
		return qualifyTemplateData(imports, data.Elem())

	case reflect.Struct:
		switch t := data.Interface().(type) {
		case Package:
			p := imports.RegisterImportForPackage(t)
			if p != t.Name {
				return reflect.ValueOf(Package{Name: p, ImportPath: t.ImportPath}), true
			}
		case Symbol:
			oldSym := &t
			newSym := imports.EnsureImported(oldSym)
			if newSym != oldSym {
				return reflect.ValueOf(newSym).Elem(), true
			}
		case MethodReference:
			newSym := imports.EnsureImported(t.Type)
			if newSym != t.Type {
				return reflect.ValueOf(MethodReference{Type: newSym, Method: t.Method}), true
			}
		case Signature:
			oldSig := &t
			newSig := imports.EnsureAllTypesImported(oldSig)
			if newSig != oldSig {
				return reflect.ValueOf(newSig).Elem(), true
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
		switch t := data.Interface().(type) {
		case *Package:
			p := imports.RegisterImportForPackage(*t)
			if p != t.Name {
				return reflect.ValueOf(&Package{Name: p, ImportPath: t.ImportPath}), true
			}
		case *Symbol:
			newSym := imports.EnsureImported(t)
			if newSym != t {
				return reflect.ValueOf(newSym), true
			}
		case *MethodReference:
			newSym := imports.EnsureImported(t.Type)
			if newSym != t.Type {
				return reflect.ValueOf(&MethodReference{Type: newSym, Method: t.Method}), true
			}
		case *Signature:
			newSig := imports.EnsureAllTypesImported(t)
			if newSig != t {
				return reflect.ValueOf(newSig), true
			}
		case *ConstSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newCs := *t
					newCs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newCs), true
				}
			}
		case *VarSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newVs := *t
					newVs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newVs), true
				}
			}
		case *TypeSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newTs := *t
					newTs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newTs), true
				}
			}
		case *FuncSpec:
			if t.parent != nil {
				oldPkg := t.parent.PackageName
				newPkg := imports.RegisterImportForPackage(t.parent.Package())
				if newPkg != oldPkg {
					newFs := *t
					newFs.parent = &GoFile{PackageName: newPkg}
					return reflect.ValueOf(&newFs), true
				}
			}
		case *InterfaceEmbed:
			t.qualify(imports)
		default:
			if newElem, changed := qualifyTemplateData(imports, data.Elem()); changed {
				if newElem.CanAddr() {
					return newElem.Addr(), true
				}
				dest := reflect.New(newElem.Type())
				dest.Elem().Set(newElem)
				return dest, true
			}
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
