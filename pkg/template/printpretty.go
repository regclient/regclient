package template

import (
	"bytes"
	"encoding/json"
)

type prettyPrinter interface {
	MarshalPretty() ([]byte, error)
}

func printPretty(v interface{}) string {
	if pp, ok := v.(prettyPrinter); ok {
		b, err := pp.MarshalPretty()
		if err != nil {
			return ""
		}
		return string(b)
	}
	// fall through for objects without a prettyPrinter interface
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return buf.String()
}
