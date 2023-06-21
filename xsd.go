// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gowsdl

import (
	"encoding/xml"
)

const xmlschema11 = "http://www.w3.org/2001/XMLSchema"

// XSDSchema represents an entire Schema structure.
type XSDSchema struct {
	XMLName            xml.Name          `xml:"schema"`
	Xmlns              map[string]string `xml:"-"`
	Tns                string            `xml:"xmlns tns,attr"`
	Xs                 string            `xml:"xmlns xs,attr"`
	Version            string            `xml:"version,attr"`
	TargetNamespace    string            `xml:"targetNamespace,attr"`
	ElementFormDefault string            `xml:"elementFormDefault,attr"`
	Includes           []*XSDInclude     `xml:"include"`
	Imports            []*XSDImport      `xml:"import"`
	Elements           []*XSDElement     `xml:"element"`
	Attributes         []*XSDAttribute   `xml:"attribute"`
	ComplexTypes       []*XSDComplexType `xml:"complexType"` // global
	SimpleType         []*XSDSimpleType  `xml:"simpleType"`
}

// UnmarshalXML implements interface xml.Unmarshaler for XSDSchema.
func (s *XSDSchema) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	s.Xmlns = make(map[string]string)
	s.XMLName = start.Name
	for _, attr := range start.Attr {
		if attr.Name.Space == "xmlns" {
			s.Xmlns[attr.Name.Local] = attr.Value
			continue
		}

		switch attr.Name.Local {
		case "version":
			s.Version = attr.Value
		case "targetNamespace":
			s.TargetNamespace = attr.Value
		case "elementFormDefault":
			s.ElementFormDefault = attr.Value
		}
	}

Loop:
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != xmlschema11 {
				d.Skip()
				continue Loop
			}

			switch t.Name.Local {
			case "include":
				x := new(XSDInclude)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				s.Includes = append(s.Includes, x)
			case "import":
				x := new(XSDImport)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				s.Imports = append(s.Imports, x)
			case "element":
				x := new(XSDElement)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				s.Elements = append(s.Elements, x)
			case "attribute":
				x := new(XSDAttribute)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				s.Attributes = append(s.Attributes, x)
			case "complexType":
				x := new(XSDComplexType)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				s.ComplexTypes = append(s.ComplexTypes, x)
			case "simpleType":
				x := new(XSDSimpleType)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				s.SimpleType = append(s.SimpleType, x)
			default:
				d.Skip()
				continue Loop
			}
		case xml.EndElement:
			break Loop
		}
	}

	return nil
}

// XSDInclude represents schema includes.
type XSDInclude struct {
	SchemaLocation string `xml:"schemaLocation,attr"`
}

// XSDImport represents XSD imports within the main schema.
type XSDImport struct {
	XMLName        xml.Name `xml:"import"`
	SchemaLocation string   `xml:"schemaLocation,attr"`
	Namespace      string   `xml:"namespace,attr"`
}

// XSDElement represents a Schema element.
type XSDElement struct {
	XMLName     xml.Name        `xml:"element"`
	Name        string          `xml:"name,attr"`
	Doc         string          `xml:"annotation>documentation"`
	Nillable    bool            `xml:"nillable,attr"`
	Type        string          `xml:"type,attr"`
	Ref         string          `xml:"ref,attr"`
	MinOccurs   string          `xml:"minOccurs,attr"`
	MaxOccurs   string          `xml:"maxOccurs,attr"`
	ComplexType *XSDComplexType `xml:"complexType"` // local
	SimpleType  *XSDSimpleType  `xml:"simpleType"`
	Groups      []*XSDGroup     `xml:"group"`
}

// XSDAny represents a Schema element.
type XSDAny struct {
	XMLName         xml.Name `xml:"any"`
	Doc             string   `xml:"annotation>documentation"`
	MinOccurs       string   `xml:"minOccurs,attr"`
	MaxOccurs       string   `xml:"maxOccurs,attr"`
	Namespace       string   `xml:"namespace,attr"`
	ProcessContents string   `xml:"processContents,attr"`
}

// XSDComplexType represents a Schema complex type.
type XSDComplexType struct {
	XMLName        xml.Name          `xml:"complexType"`
	Abstract       bool              `xml:"abstract,attr"`
	Name           string            `xml:"name,attr"`
	Mixed          bool              `xml:"mixed,attr"`
	Sequence       []*XSDElement     `xml:"sequence>element"`
	Choice         []*XSDElement     `xml:"choice>element"`
	All            []*XSDElement     `xml:"all>element"`
	ComplexContent XSDComplexContent `xml:"complexContent"`
	SimpleContent  XSDSimpleContent  `xml:"simpleContent"`
	Attributes     []*XSDAttribute   `xml:"attribute"`
	Any            []*XSDAny         `xml:"sequence>any"`
}

type XSDChoiceType struct {
	XMLName   xml.Name      `xml:"choice"`
	Name      string        `xml:"name,attr"`
	MinOccurs string        `xml:"minOccurs,attr"`
	Elements  []*XSDElement `xml:"element"`
}

type XSDAllType struct {
	XMLName  xml.Name      `xml:"all"`
	Elements []*XSDElement `xml:"element"`
}

func (ct *XSDComplexType) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	ct.XMLName = start.Name
	for _, attr := range start.Attr {

		switch attr.Name.Local {
		case "mixed":
			ct.Mixed = attr.Value == "true"
		case "name":
			ct.Name = attr.Value
		case "abstract":
			ct.Abstract = attr.Value == "true"
		}
	}

Loop:
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != xmlschema11 {
				d.Skip()
				continue Loop
			}

			switch t.Name.Local {
			case "attribute":
				x := new(XSDAttribute)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				ct.Attributes = append(ct.Attributes, x)
			case "sequence":
				if err := ct.unmarshalSequence(d, t); err != nil {
					return err
				}
			case "choice":
				choiceElements, err := ct.unmarshalChoice(d, t)
				if err != nil {
					return err
				}
				ct.Choice = append(ct.Choice, choiceElements...)
			case "all":
				x := new(XSDAllType)
				if err := d.DecodeElement(x, &start); err != nil {
					return err
				}
				ct.All = append(ct.All, x.Elements...)
			case "complexContent":
				x := new(XSDComplexContent)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				ct.ComplexContent = *x
			case "simpleContent":
				x := new(XSDSimpleContent)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				ct.SimpleContent = *x

			default:
				d.Skip()
				continue Loop
			}
		case xml.EndElement:
			break Loop
		}
	}
	return nil
}

func (ct *XSDComplexType) unmarshalSequence(d *xml.Decoder, start xml.StartElement) error {
Loop:
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != xmlschema11 {
				d.Skip()
				continue Loop
			}

			switch t.Name.Local {
			case "element":
				x := new(XSDElement)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				ct.Sequence = append(ct.Sequence, x)
			case "choice":
				choiceElements, err := ct.unmarshalChoice(d, t)
				if err != nil {
					return err
				}
				ct.Sequence = append(ct.Sequence, choiceElements...)

			case "any":
				// this logic preserves the old logic of gowsdl although I guess it is as wrong as the
				// old logic for "SequenceChoice", i.e. the ordering of elements is not preserved this way
				// To fix this, we would need to change the type of ct.Sequence to []interface{} (or some marker interface)
				// But this would lead to changes in the template which is consuming all this types
				x := new(XSDAny)
				if err := d.DecodeElement(x, &t); err != nil {
					return err
				}
				ct.Any = append(ct.Any, x)
			default:
				d.Skip()
				continue Loop
			}
		case xml.EndElement:
			break Loop
		}
	}
	return nil
}

func (ct *XSDComplexType) unmarshalChoice(d *xml.Decoder, start xml.StartElement) ([]*XSDElement, error) {
	x := new(XSDChoiceType)
	if err := d.DecodeElement(x, &start); err != nil {
		return nil, err
	}
	if x.MinOccurs != "" {
		for i := range x.Elements {
			x.Elements[i].MinOccurs = x.MinOccurs
		}
	}
	return x.Elements, nil
}

// XSDGroup element is used to define a group of elements to be used in complex type definitions.
type XSDGroup struct {
	Name     string       `xml:"name,attr"`
	Ref      string       `xml:"ref,attr"`
	Sequence []XSDElement `xml:"sequence>element"`
	Choice   []XSDElement `xml:"choice>element"`
	All      []XSDElement `xml:"all>element"`
}

// XSDComplexContent element defines extensions or restrictions on a complex
// type that contains mixed content or elements only.
type XSDComplexContent struct {
	XMLName   xml.Name     `xml:"complexContent"`
	Extension XSDExtension `xml:"extension"`
}

// XSDSimpleContent element contains extensions or restrictions on a text-only
// complex type or on a simple type as content and contains no elements.
type XSDSimpleContent struct {
	XMLName   xml.Name     `xml:"simpleContent"`
	Extension XSDExtension `xml:"extension"`
}

// XSDExtension element extends an existing simpleType or complexType element.
type XSDExtension struct {
	XMLName        xml.Name        `xml:"extension"`
	Base           string          `xml:"base,attr"`
	Attributes     []*XSDAttribute `xml:"attribute"`
	Sequence       []*XSDElement   `xml:"sequence>element"`
	Choice         []*XSDElement   `xml:"choice>element"`
	SequenceChoice []*XSDElement   `xml:"sequence>choice>element"`
}

// XSDAttribute represent an element attribute. Simple elements cannot have
// attributes. If an element has attributes, it is considered to be of a
// complex type. But the attribute itself is always declared as a simple type.
type XSDAttribute struct {
	Doc        string         `xml:"annotation>documentation"`
	Name       string         `xml:"name,attr"`
	Ref        string         `xml:"ref,attr"`
	Type       string         `xml:"type,attr"`
	Use        string         `xml:"use,attr"`
	Fixed      string         `xml:"fixed,attr"`
	SimpleType *XSDSimpleType `xml:"simpleType"`
}

// XSDSimpleType element defines a simple type and specifies the constraints
// and information about the values of attributes or text-only elements.
type XSDSimpleType struct {
	Name        string         `xml:"name,attr"`
	Doc         string         `xml:"annotation>documentation"`
	Restriction XSDRestriction `xml:"restriction"`
	List        XSDList        `xml:"list"`
	Union       XSDUnion       `xml:"union"`
	Final       string         `xml:"final"`
}

// XSDList represents a element list
type XSDList struct {
	Doc        string         `xml:"annotation>documentation"`
	ItemType   string         `xml:"itemType,attr"`
	SimpleType *XSDSimpleType `xml:"simpleType"`
}

// XSDUnion represents a union element
type XSDUnion struct {
	SimpleType  []*XSDSimpleType `xml:"simpleType,omitempty"`
	MemberTypes string           `xml:"memberTypes,attr"`
}

// XSDRestriction defines restrictions on a simpleType, simpleContent, or complexContent definition.
type XSDRestriction struct {
	Base         string                `xml:"base,attr"`
	Enumeration  []XSDRestrictionValue `xml:"enumeration"`
	Pattern      XSDRestrictionValue   `xml:"pattern"`
	MinInclusive XSDRestrictionValue   `xml:"minInclusive"`
	MaxInclusive XSDRestrictionValue   `xml:"maxInclusive"`
	WhiteSpace   XSDRestrictionValue   `xml:"whitespace"`
	Length       XSDRestrictionValue   `xml:"length"`
	MinLength    XSDRestrictionValue   `xml:"minLength"`
	MaxLength    XSDRestrictionValue   `xml:"maxLength"`
}

// XSDRestrictionValue represents a restriction value.
type XSDRestrictionValue struct {
	Doc   string `xml:"annotation>documentation"`
	Value string `xml:"value,attr"`
}
