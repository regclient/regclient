//go:build legacy
// +build legacy

// Legacy package, this has been moved to the pkg/template package

package regclient

import (
	gotemplate "text/template"
)

// TemplateFuncs adds functions to a template.
//
// Deprecated: replace with [gotemplate.FuncMap].
var TemplateFuncs = gotemplate.FuncMap{}
