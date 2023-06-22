// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gowsdl

var headerTmpl = `
// Code generated by gowsdl DO NOT EDIT.

package {{.Pkg}}

import (
	"context"
	"encoding/xml"
	"time"
	"github.com/jens1205/gowsdl/soap"

	{{ $baseURL := .BaseURL }}
	{{range .Imports}}
		"{{print $baseURL "/" .}}"
	{{end}}
)

// against "unused imports"
var _ time.Time
var _ xml.Name
var _ soap.XSDDateTime
var _ context.Context

type AnyType struct {
	InnerXML string ` + "`" + `xml:",innerxml"` + "`" + `
}

type AnyURI string

type NCName string

`
