package regclient

import (
	"bytes"
	"encoding/json"
	gotemplate "text/template"
)

// TemplateFuncs provides additional functions for handling regclient types in templates
var TemplateFuncs = gotemplate.FuncMap{
	"printPretty": printPretty,
}

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
	enc.Encode(v)
	return buf.String()
}
