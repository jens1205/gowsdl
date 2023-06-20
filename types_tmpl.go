// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gowsdl

var typesTmpl = `
{{define "SimpleType"}}
	{{$typeName := replaceReservedWords .Name | makePublic}}
	{{if .Doc}} {{.Doc | comment}} {{end}}
	{{if ne .List.ItemType ""}}
		type {{$typeName}} []{{toGoType .List.ItemType false "" | removePointerFromType}}
	{{else if ne .Union.MemberTypes ""}}
		type {{$typeName}} string
	{{else if .Union.SimpleType}}
		type {{$typeName}} string
	{{else if .Restriction.Base}}
		type {{$typeName}} {{toGoType .Restriction.Base false "" | removePointerFromType}}
    {{else}}
		type {{$typeName}} interface{}
	{{end}}

	{{if .Restriction.Enumeration}}
	const (
		{{with .Restriction}}
			{{range .Enumeration}}
				{{if .Doc}} {{.Doc | comment}} {{end}}
				{{$typeName}}{{$value := replaceReservedWords .Value}}{{$value | makePublic}} {{$typeName}} = "{{goString .Value}}" {{end}}
		{{end}}
	)
	{{end}}
{{end}}

{{define "ComplexContent"}}
	{{$baseType := toGoType .Extension.Base false ""}}
	{{ if $baseType }}
		{{$baseType}}
	{{end}}

	{{template "Elements" wrapElement .Extension.Sequence ""}}
	{{template "Elements" wrapElement .Extension.Choice ""}}
	{{template "Elements" wrapElement .Extension.SequenceChoice ""}}
	{{template "Attributes" .Extension.Attributes}}
{{end}}

{{define "Attributes"}}
    {{ $targetNamespace := getNS }}
	{{range .}}
		{{if .Doc}} {{.Doc | comment}} {{end}}
		{{ if ne .Type "" }}
			{{ normalize .Name | makeFieldPublic}} {{toGoType .Type false ""}} ` + "`" + `xml:"{{with $targetNamespace}}{{.}} {{end}}{{.Name}},attr,omitempty" json:"{{.Name}},omitempty"` + "`" + `
		{{ else }}
			{{ normalize .Name | makeFieldPublic}} string ` + "`" + `xml:"{{with $targetNamespace}}{{.}} {{end}}{{.Name}},attr,omitempty" json:"{{.Name}},omitempty"` + "`" + `
		{{ end }}
	{{end}}
{{end}}

{{define "SimpleContent"}}
	Value {{toGoType .Extension.Base false ""}} ` + "`xml:\",chardata\" json:\"-,\"`" + `
	{{template "Attributes" .Extension.Attributes}}
{{end}}

{{define "ComplexTypeInline"}}
	{{$parentName := .ParentName}}
	{{range .Elements}}
	  {{$name := .Name}}
	    {{with .ComplexType}}
	    {{$fullName := print $parentName "_" $name}}
		{{$typeName := replaceReservedWords $fullName | makePublic}}
		type {{$typeName}} struct {
	    	{{if ne .ComplexContent.Extension.Base ""}}
	    		{{template "ComplexContent" .ComplexContent}}
	    	{{else if ne .SimpleContent.Extension.Base ""}}
	    		{{template "SimpleContent" .SimpleContent}}
	    	{{else}}
	    		{{template "Elements" wrapElement .Sequence $name}}
	    		{{template "Elements" wrapElement .Choice $name}}
	    		{{template "Elements" wrapElement .SequenceChoice $name}}
	    		{{template "Elements" wrapElement .All $name}}
	    	{{end}}
	    }
	    {{end}}

	{{end}}

{{end}}

{{define "Elements"}}
	{{$parentName := .ParentName}}
	{{range .Elements}}
		{{if ne .Ref ""}}
	        {{ $prefix := getNSPrefix .Ref }}
			{{removeNS .Ref | replaceReservedWords  | makePublic}} {{if eq .MaxOccurs "unbounded"}}[]{{end}}{{toGoType .Ref .Nillable .MinOccurs }} ` + "`" + `xml:"{{getNSFromMap $prefix}} {{.Ref | removeNS}},omitempty" json:"{{.Ref | removeNS}},omitempty"` + "`" + `
		{{else}}
		{{if not .Type}}
			{{if .SimpleType}}
				{{if .Doc}} {{.Doc | comment}} {{end}}
				{{if ne .SimpleType.List.ItemType ""}}
					{{ normalize .Name | makeFieldPublic}} []{{toGoType .SimpleType.List.ItemType false "" }} ` + "`" + `xml:"{{.Name}},omitempty" json:"{{.Name}},omitempty"` + "`" + `
				{{else}}
					{{ normalize .Name | makeFieldPublic}} {{toGoType .SimpleType.Restriction.Base false ""}} ` + "`" + `xml:"{{.Name}},omitempty" json:"{{.Name}},omitempty"` + "`" + `
				{{end}}
			{{end}}
	        {{if .ComplexType}}
				{{if .Doc}} {{.Doc | comment}} {{end}}
	            {{replaceReservedWords .Name | makePublic}} {{if eq .MaxOccurs "unbounded"}}[]{{end}} {{toGoType $parentName .Nillable .MinOccurs}}_{{.Name}} ` + "`" + `xml:"{{.Name}},omitempty" json:"{{.Name}},omitempty"` + "`" + `
	        {{end}}
		{{else}}
			{{if .Doc}}{{.Doc | comment}} {{end}}
			{{replaceAttrReservedWords .Name | makeFieldPublic}} {{if eq .MaxOccurs "unbounded"}}[]{{end}}{{toGoType .Type .Nillable .MinOccurs }} ` + "`" + `xml:"{{.Name}},omitempty" json:"{{.Name}},omitempty"` + "`" + ` {{end}}
		{{end}}
	{{end}}
{{end}}

{{define "Any"}}
	{{range .}}
		Items     []string ` + "`" + `xml:",any" json:"items,omitempty"` + "`" + `
	{{end}}
{{end}}

{{range .Schemas}}
	{{ $targetNamespace := setNS .TargetNamespace }}
	{{ $foo := setNSMap .Xmlns }}

	{{range .SimpleType}}
		{{template "SimpleType" .}}
	{{end}}

	{{range .Elements}}
		{{$name := .Name}}
		{{$typeName := replaceReservedWords $name | makePublic}}
		{{if not .Type}}
			{{/* ComplexTypeLocal */}}
			{{with .ComplexType}}
				type {{$typeName}} struct {
					XMLName xml.Name ` + "`xml:\"{{$targetNamespace}} {{$name}}\"`" + `
					{{if ne .ComplexContent.Extension.Base ""}}
						{{template "ComplexContent" .ComplexContent}}
					{{else if ne .SimpleContent.Extension.Base ""}}
						{{template "SimpleContent" .SimpleContent}}
					{{else}}
						{{template "Elements" wrapElement .Sequence $name}}
						{{template "Any" .Any }}
						{{template "Elements" wrapElement .Choice $name}}
						{{template "Elements" wrapElement .SequenceChoice $name}}
						{{template "Elements" wrapElement .All $name}}
						{{template "Attributes" .Attributes}}
					{{end}}
				}
			    {{template "ComplexTypeInline" wrapElement .Sequence $name}}
			    {{template "ComplexTypeInline" wrapElement .Choice $name}}
			    {{template "ComplexTypeInline" wrapElement .SequenceChoice $name}}
			    {{template "ComplexTypeInline" wrapElement .All $name}}
			{{end}}
			{{/* SimpleTypeLocal */}}
			{{with .SimpleType}}
				{{if .Doc}} {{.Doc | comment}} {{end}}
				{{if ne .List.ItemType ""}}
					type {{$typeName}} []{{toGoType .List.ItemType false "" | removePointerFromType}}
				{{else if ne .Union.MemberTypes ""}}
					type {{$typeName}} string
				{{else if .Union.SimpleType}}
					type {{$typeName}} string
				{{else if .Restriction.Base}}
					type {{$typeName}} {{toGoType .Restriction.Base false "" | removePointerFromType}}
				{{else}}
					type {{$typeName}} interface{}
				{{end}}

				{{if .Restriction.Enumeration}}
				const (
					{{with .Restriction}}
						{{range .Enumeration}}
							{{if .Doc}} {{.Doc | comment}} {{end}}
							{{$typeName}}{{$value := replaceReservedWords .Value}}{{$value | makePublic}} {{$typeName}} = "{{goString .Value}}" {{end}}
					{{end}}
				)
				{{end}}
			{{end}}
		{{else}}
			{{$type := toGoType .Type .Nillable .MinOccurs | removePointerFromType}}
			{{if ne ($typeName) ($type)}}
				type {{$typeName}} {{$type}}
				{{if eq ($type) ("soap.XSDDateTime")}}
					func (xdt {{$typeName}}) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
						return soap.XSDDateTime(xdt).MarshalXML(e, start)
					}

					func (xdt *{{$typeName}}) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
						return (*soap.XSDDateTime)(xdt).UnmarshalXML(d, start)
					}
				{{else if eq ($type) ("soap.XSDDate")}}
					func (xd {{$typeName}}) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
						return soap.XSDDate(xd).MarshalXML(e, start)
					}

					func (xd *{{$typeName}}) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
						return (*soap.XSDDate)(xd).UnmarshalXML(d, start)
					}
				{{else if eq ($type) ("soap.XSDTime")}}
					func (xt {{$typeName}}) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
						return soap.XSDTime(xt).MarshalXML(e, start)
					}

					func (xt *{{$typeName}}) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
						return (*soap.XSDTime)(xt).UnmarshalXML(d, start)
					}
				{{end}}
			{{end}}
		{{end}}
	{{end}}

	{{range .ComplexTypes}}
		{{/* ComplexTypeGlobal */}}
		{{$typeName := replaceReservedWords .Name | makePublic}}
		{{if and (eq (len .SimpleContent.Extension.Attributes) 0) (eq (toGoType .SimpleContent.Extension.Base false "") "string") }}
			type {{$typeName}} string
		{{else}}
	        // in main template - ComplexTypes
			type {{$typeName}} struct {
				{{$type := findNameByType .Name}}
				{{if ne .Name $type}}
					XMLName xml.Name ` + "`xml:\"{{$targetNamespace}} {{$type}}\"`" + `
				{{end}}

				{{if ne .ComplexContent.Extension.Base ""}}
					{{template "ComplexContent" .ComplexContent}}
				{{else if ne .SimpleContent.Extension.Base ""}}
					{{template "SimpleContent" .SimpleContent}}
				{{else}}
					{{template "Elements" wrapElement .Sequence $typeName}}
					{{template "Any" .Any }}
					{{template "Elements" wrapElement .Choice $typeName}}
					{{template "Elements" wrapElement .SequenceChoice $typeName}}
					{{template "Elements" wrapElement .All $typeName}}
					{{template "Attributes" .Attributes}}
				{{end}}
			}
			{{template "ComplexTypeInline" wrapElement .Sequence $typeName}}
			{{template "ComplexTypeInline" wrapElement .Choice $typeName}}
			{{template "ComplexTypeInline" wrapElement .SequenceChoice $typeName}}
			{{template "ComplexTypeInline" wrapElement .All $typeName}}

		{{end}}
	{{end}}
{{end}}
`
