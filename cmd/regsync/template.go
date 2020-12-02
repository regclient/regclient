package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"
)

var tmplFuncs = template.FuncMap{
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
	"time":  TimeFunc,
	"title": strings.Title,
	"upper": strings.ToUpper,
}

func templateWritter(out io.Writer, tmpl string, data interface{}) error {
	t, err := template.New("out").Funcs(tmplFuncs).Parse(tmpl)
	if err != nil {
		return err
	}
	return t.Execute(out, data)
}

func templateString(tmpl string, data interface{}) (string, error) {
	var sb strings.Builder
	err := templateWritter(&sb, tmpl, data)
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

// TimeFunc provides the "time" template, returning a struct with methods
func TimeFunc() *TimeFuncs {
	return &TimeFuncs{}
}

// TimeFuncs wraps all time based templates
type TimeFuncs struct{}

// Now returns current time
func (t *TimeFuncs) Now() time.Time {
	return time.Now()
}

// Parse parses the current time according to layout
func (t *TimeFuncs) Parse(layout string, value string) (time.Time, error) {
	return time.Parse(layout, value)
}
