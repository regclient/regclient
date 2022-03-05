//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the pkg/template package

package regclient

import (
	gotemplate "text/template"
)

var TemplateFuncs = gotemplate.FuncMap{}
