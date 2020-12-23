package go2lua

import (
	"fmt"
	"reflect"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// Convert takes an input interface and converts it to a Lua value
func Convert(ls *lua.LState, v interface{}) lua.LValue {
	return convertReflect(ls, reflect.ValueOf(v))
}

func convertReflect(ls *lua.LState, v reflect.Value) lua.LValue {
	if !v.IsValid() {
		return lua.LNil
	}
	switch v.Type().Kind() {
	case reflect.Bool:
		return lua.LBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return lua.LNumber(float64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return lua.LNumber(float64(v.Uint()))
	case reflect.Float32, reflect.Float64:
		return lua.LNumber(v.Float())
	case reflect.String:
		return lua.LString(v.String())
	case reflect.Array:
		lTab := ls.NewTable()
		for i := 0; i < v.Len(); i++ {
			lTab.RawSetInt(i, convertReflect(ls, v.Index(i)))
		}
		return lTab
	case reflect.Slice:
		lTab := ls.NewTable()
		for i := 0; i < v.Len(); i++ {
			lTab.RawSetInt(i, convertReflect(ls, v.Index(i)))
		}
		return lTab
	case reflect.Map:
		lTab := ls.NewTable()
		for _, k := range v.MapKeys() {
			// only support String and Int keys, all other map keys are ignored
			if k.Type().Kind() == reflect.String {
				lTab.RawSetString(k.String(), convertReflect(ls, v.MapIndex(k)))
			} else if k.Type().Kind() == reflect.Int {
				lTab.RawSetInt(int(k.Int()), convertReflect(ls, v.MapIndex(k)))
			}
		}
		return lTab
	case reflect.Struct:
		vType := v.Type()
		lTab := ls.NewTable()
		foundExported := false
		for i := 0; i < vType.NumField(); i++ {
			field := vType.Field(i)
			// skip unexported fields
			if !v.FieldByName(field.Name).CanInterface() {
				continue
			}
			foundExported = true
			lVal := convertReflect(ls, v.FieldByName(field.Name))
			lTab.RawSetString(field.Name, lVal)
			// map json keys to values if defined
			jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
			if jsonName != "" && jsonName != "-" {
				lTab.RawSetString(jsonName, lVal)
			}
		}
		if foundExported {
			return lTab
		}
		// fallback to trying to export as a string
		vInterface := v.Interface()
		vStringer, ok := vInterface.(fmt.Stringer)
		if ok {
			return lua.LString(vStringer.String())
		}
		// Unsupported struct, no exported fields and no string interface
		return lua.LNil
	case reflect.Ptr:
		if v.IsNil() {
			return lua.LNil
		}
		return convertReflect(ls, v.Elem())
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

		return lua.LNil
	}
}
