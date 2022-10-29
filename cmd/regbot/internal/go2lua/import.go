package go2lua

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// Import takes a Lua value and copies matching values into the provided Go interface.
// By providing the orig interface, values that cannot be imported from Lua will be copied from orig.
func Import(ls *lua.LState, lv lua.LValue, v, orig interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("go2lua import panic: %v\n%s", r, string(debug.Stack()))
		}
	}()
	// orig may not be a pointer or empty interface, dereference v until the two values match
	rV := reflect.ValueOf(v)
	rOrig := reflect.ValueOf(orig)
	for rV.IsValid() && rOrig.IsValid() && rV.Type() != rOrig.Type() &&
		(rV.Type().Kind() == reflect.Interface || rV.Type().Kind() == reflect.Ptr) {
		rV = rV.Elem()
	}
	return importReflect(ls, lv, rV, rOrig)
}

func importReflect(ls *lua.LState, lv lua.LValue, v, orig reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	// fmt.Printf("v-type: %s, orig valid: %t\n", v.Type().String(), orig.IsValid()) // for debugging
	// Switch based on the kind of object we are creating.
	// Each basic type is similar: if lua has a matching basic type, export the lua value and set with reflection.
	switch v.Type().Kind() {
	case reflect.Bool:
		if lvi, ok := lv.(lua.LBool); ok {
			v.SetBool(bool(lvi))
		}
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if lvi, ok := lv.(lua.LNumber); ok {
			v.SetInt(int64(lvi))
		}
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if lvi, ok := lv.(lua.LNumber); ok {
			v.SetUint(uint64(lvi))
		}
		return nil
	case reflect.Float32, reflect.Float64:
		if lvi, ok := lv.(lua.LNumber); ok {
			v.SetFloat(float64(lvi))
		}
		return nil
	case reflect.String:
		if lvi, ok := lv.(lua.LString); ok {
			v.SetString(string(lvi))
		}
		return nil
	case reflect.Array:
		// If we have an array, and lua is the expected table, iterate and recursively import contents.
		if lvi, ok := lv.(*lua.LTable); ok {
			for i := 0; i < v.Len(); i++ {
				// Orig is also iterated on if it has a matching type and length
				var origI reflect.Value
				if orig.IsValid() && orig.Type() == v.Type() && orig.Len() < i {
					origI = orig.Index(i)
				}
				importReflect(ls, lvi.RawGetInt(i+1), v.Index(i), origI)
			}
		}
		return nil
	case reflect.Slice:
		// Slice follows the same pattern as array, except the slice is first created with the desired size.
		if lvi, ok := lv.(*lua.LTable); ok {
			newV := reflect.MakeSlice(v.Type(), lvi.Len(), lvi.Len())
			for i := 0; i < newV.Len(); i++ {
				var origI reflect.Value
				if orig.IsValid() && orig.Type() == v.Type() && orig.Len() > i {
					origI = orig.Index(i)
				}
				importReflect(ls, lvi.RawGetInt(i+1), newV.Index(i), origI)
			}
			v.Set(newV)
		}
		return nil
	case reflect.Map:
		if lvi, ok := lv.(*lua.LTable); ok {
			newV := reflect.MakeMap(v.Type())
			lvi.ForEach(func(lvtKey, lvtElem lua.LValue) {
				newKey := reflect.Indirect(reflect.New(v.Type().Key()))
				newElem := reflect.Indirect(reflect.New(v.Type().Elem()))
				importReflect(ls, lvtKey, newKey, reflect.Value{})
				var origElem reflect.Value
				if orig.IsValid() && orig.Type() == v.Type() {
					origElem = orig.MapIndex(newKey)
				}
				importReflect(ls, lvtElem, newElem, origElem)
				newV.SetMapIndex(newKey, newElem)
			})
			v.Set(newV)
		}
		return nil
	case reflect.Struct:
		foundExported := false
		if lvi, ok := lv.(*lua.LTable); ok {
			vType := v.Type()
			for i := 0; i < vType.NumField(); i++ {
				field := vType.Field(i)
				// skip unexported fields
				if !v.FieldByName(field.Name).CanInterface() {
					continue
				}
				foundExported = true
				key := field.Name
				// use json keys if defined
				jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
				if jsonName != "" && jsonName != "-" {
					key = jsonName
				}
				var origElem reflect.Value
				if orig.IsValid() && orig.Type() == v.Type() {
					origElem = orig.FieldByName(field.Name)
				}
				importReflect(ls, lvi.RawGetString(key), v.FieldByName(field.Name), origElem)
			}
		}
		if !foundExported && orig.IsValid() && orig.Type() == v.Type() {
			v.Set(orig)
		}
		return nil
	case reflect.Ptr, reflect.Interface:
		if lv != lua.LNil {
			// if pointer is nil, create a new value
			if v.IsNil() {
				newV := reflect.New(v.Type().Elem())
				v.Set(newV)
			}
			var origElem reflect.Value
			if orig.IsValid() && orig.Type() == v.Type() {
				origElem = orig.Elem()
			} else if orig.IsValid() && orig.Type() == v.Elem().Type() {
				origElem = orig
			}
			// dereference pointer and recurse
			importReflect(ls, lv, v.Elem(), origElem)
			// } else {
			// 	// this doesn't work: reflect.Value.Set using unaddressable value
			// 	v.Set(reflect.Zero(v.Type()))
		}
		return nil
	default:
		// Unsupported reflect types:
		// Invalid
		// Uintptr
		// Complex64
		// Complex128
		// Chan
		// Func
		// Interface
		// UnsafePointer
		if orig.IsValid() && orig.Type() == v.Type() {
			v.Set(orig)
		}
		return nil
	}
}
