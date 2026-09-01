package debug

import (
	"fmt"
	"reflect"
	"strings"
)

func Dump(v any) {
	dump(reflect.ValueOf(v), 0)
}

func dump(v reflect.Value, indent int) {
	prefix := strings.Repeat("  ", indent)

	if !v.IsValid() {
		fmt.Println(prefix + "nil")
		return
	}

	// Раскрываем interface{}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			fmt.Println(prefix + "nil")
			return
		}

		dump(v.Elem(), indent)
		return
	}

	// Раскрываем указатели
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			fmt.Println(prefix + "nil")
			return
		}

		dump(v.Elem(), indent)
		return
	}

	switch v.Kind() {
	case reflect.Struct:
		fmt.Printf("%s%s\n", prefix, v.Type())

		for i := 0; i < v.NumField(); i++ {
			fieldInfo := v.Type().Field(i)
			field := v.Field(i)

			fmt.Printf("%s  %s:", prefix, fieldInfo.Name)

			switch field.Kind() {
			case reflect.Struct,
				reflect.Pointer,
				reflect.Interface,
				reflect.Slice,
				reflect.Array,
				reflect.Map:

				fmt.Println()
				dump(field, indent+2)

			default:
				if field.CanInterface() {
					fmt.Printf(" %v\n", field.Interface())
				} else {
					fmt.Printf(" %v\n", field)
				}
			}
		}

	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			fmt.Printf("%s[]\n", prefix)
			return
		}

		for i := 0; i < v.Len(); i++ {
			fmt.Printf("%s[%d]:\n", prefix, i)
			dump(v.Index(i), indent+1)
		}

	case reflect.Map:
		if v.Len() == 0 {
			fmt.Printf("%s{}\n", prefix)
			return
		}

		for _, key := range v.MapKeys() {
			fmt.Printf("%s%v:\n", prefix, key)
			dump(v.MapIndex(key), indent+1)
		}

	default:
		if v.CanInterface() {
			fmt.Printf("%s%v\n", prefix, v.Interface())
		} else {
			fmt.Printf("%s%v\n", prefix, v)
		}
	}
}
