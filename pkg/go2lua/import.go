package go2lua

import (
	"fmt"
	"reflect"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// Import takes a Lua value and copies matching values into the provided interface
func Import(ls *lua.LState, lv lua.LValue, v, orig interface{}) error {
	return importReflect(ls, lv, reflect.ValueOf(v), reflect.ValueOf(orig))
}

func importReflect(ls *lua.LState, lv lua.LValue, v, orig reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	if !orig.IsValid() {
		fmt.Printf("Orig is not valid, processing %s\n", v.Type().String())
	}
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
		if lvi, ok := lv.(*lua.LTable); ok {
			for i := 0; i < v.Len(); i++ {
				var origI reflect.Value
				if orig.IsValid() && orig.Type() == v.Type() && orig.Len() < i {
					origI = orig.Index(i)
				}
				importReflect(ls, lvi.RawGetInt(i), v.Index(i), origI)
			}
		}
		return nil
	case reflect.Slice:
		if lvi, ok := lv.(*lua.LTable); ok {
			newV := reflect.MakeSlice(v.Type(), lvi.Len(), lvi.Len())
			for i := 0; i < newV.Len(); i++ {
				var origI reflect.Value
				if orig.IsValid() && orig.Type() == v.Type() && orig.Len() > i {
					origI = orig.Index(i)
				} else if !orig.IsValid() {
					fmt.Printf("Skipping orig in slice, not valid\n")
				} else {
					fmt.Printf("Skipping orig in slice, i = %d, len = %d, v type = %s, o type = %s\n", i, orig.Len(), v.Type().String(), orig.Type().String())
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
					fmt.Printf("Skipping internal field %s\n", field.Name)
					continue
				}
				fmt.Printf("Processing field %s\n", field.Name)
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
			fmt.Printf("Struct without exported fields or lua table available copied with Set\n")
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
			fmt.Printf("Pointer or interface dereferenced\n")
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
			fmt.Printf("Unsupported type, orig copied, type = %s, kind = %s\n", v.Type(), v.Type().Kind())
			v.Set(orig)
		} else {
			fmt.Printf("Unsupported type, orig unavailable, type = %s, kind = %s\n", v.Type(), v.Type().Kind())
		}
		return nil
	}
}
