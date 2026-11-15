//go:build linux

package portal

import "reflect"

// normalizeOther flattens the godbus decode shapes the type switch
// does not name: struct arrays like a(ua{sv}) become []any rows of
// normalized fields, plain structs like (ii) become a []any of
// their fields, and typed slices like []int32 become []any. Values
// with unexported fields (dbus.Signature and friends) pass through
// untouched.
func normalizeOther(v any) any {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return v
	}
	switch rv.Kind() {
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return v // byte slices are data, not structure
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = normalize(rv.Index(i).Interface())
		}
		return out
	case reflect.Struct:
		n := rv.NumField()
		out := make([]any, 0, n)
		for f := 0; f < n; f++ {
			field := rv.Field(f)
			if !field.CanInterface() {
				return v
			}
			out = append(out, normalize(field.Interface()))
		}
		return out
	default:
		return v
	}
}
