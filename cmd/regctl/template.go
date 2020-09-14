package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"text/template"
)

var tmplFuncs = template.FuncMap{
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
	"split": strings.Split,
	"join":  strings.Join,
	"title": strings.Title,
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
}

func templateRun(out io.Writer, tmpl string, data interface{}) error {
	t, err := template.New("out").Funcs(tmplFuncs).Parse(tmpl)
	if err != nil {
		return err
	}
	return t.Execute(out, data)
}
