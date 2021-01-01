package template

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	gotemplate "text/template"
)

var tmplFuncs = gotemplate.FuncMap{
	"default": func(def, orig interface{}) interface{} {
		if orig == nil || orig == reflect.Zero(reflect.TypeOf(orig)).Interface() {
			return def
		}
		return orig
	},
	"env": func(key string) string {
		return os.Getenv(key)
	},
	"file": func(filename string) string {
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	},
	"join": strings.Join,
	"json": func(v interface{}) string {
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		enc.Encode(v)
		return buf.String()
	},
	"jsonPretty": func(v interface{}) string {
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		enc.Encode(v)
		return buf.String()
	},
	"lower": strings.ToLower,
	"split": strings.Split,
	"time":  func() *TimeFuncs { return &TimeFuncs{} },
	"title": strings.Title,
	"upper": strings.ToUpper,
}

// Writer outputs a template to an io.Writer
func Writer(out io.Writer, tmpl string, data interface{}) error {
	t, err := gotemplate.New("out").Funcs(tmplFuncs).Parse(tmpl)
	if err != nil {
		return err
	}
	return t.Execute(out, data)
}

// String converts a template to a string
func String(tmpl string, data interface{}) (string, error) {
	var sb strings.Builder
	err := Writer(&sb, tmpl, data)
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}
