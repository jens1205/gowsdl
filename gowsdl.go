// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gowsdl

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode"

	"golang.org/x/exp/slices"
)

const maxRecursion uint8 = 20

type NamespaceMapping map[string]string

func (i *NamespaceMapping) String() string {
	var s strings.Builder

	for k, v := range *i {
		s.WriteString(k)
		s.WriteString("=")
		s.WriteString(v)
		s.WriteString("\n")
	}

	return s.String()
}

func (i *NamespaceMapping) Set(value string) error {
	parts := strings.Split(value, "=")
	if len(parts) != 2 {
		return fmt.Errorf("expected format pkg=ns")
	}
	(*i)[parts[1]] = parts[0]
	return nil
}

func (i *NamespaceMapping) GetPackages() []string {
	var result []string
	for _, v := range *i {
		result = append(result, v)
	}
	return result
}

// GoWSDL defines the struct for WSDL generator.
type GoWSDL struct {
	loc                   *Location
	rawWSDL               []byte
	pkg                   string
	ignoreTLS             bool
	makePublicFn          func(string) string
	wsdl                  *WSDL
	resolvedXSDExternals  map[string]bool
	currentRecursionLevel uint8
	currentNamespace      string
	currentNamespaceMap   map[string]string
	nsToPkg               NamespaceMapping
	pkgBaseURL            string
	// imports remembers the imports (pakage names) we need per target namespace
	imports map[string][]string
}

// Method setNS sets (and returns) the currently active XML namespace.
func (g *GoWSDL) setNS(ns string) string {
	g.currentNamespace = ns
	return ns
}

// Method setNS returns the currently active XML namespace.
func (g *GoWSDL) getNS() string {
	return g.currentNamespace
}

func (g *GoWSDL) setNSMap(nsMap map[string]string) map[string]string {
	g.currentNamespaceMap = nsMap
	return nsMap
}

func (g *GoWSDL) getNSFromMap(prefix string) string {
	if result, ok := g.currentNamespaceMap[prefix]; ok {
		return result
	}
	return ""
}

func (g *GoWSDL) getNSPackage(ns string) string {
	if result, ok := g.nsToPkg[ns]; ok {
		return result
	}
	return ""
}

var cacheDir = filepath.Join(os.TempDir(), "gowsdl-cache")

func init() {
	err := os.MkdirAll(cacheDir, 0700)
	if err != nil {
		log.Println("Create cache directory", "error", err)
		os.Exit(1)
	}
}

var timeout = time.Duration(30 * time.Second)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

func downloadFile(url string, ignoreTLS bool) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: ignoreTLS,
		},
		Dial: dialTimeout,
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Received response code %d", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// NewGoWSDL initializes WSDL generator.
func NewGoWSDL(file, pkg string, nsToPkg NamespaceMapping, pkgBaseURL string, ignoreTLS bool, exportAllTypes bool) (*GoWSDL, error) {
	file = strings.TrimSpace(file)
	if file == "" {
		return nil, errors.New("WSDL file is required to generate Go proxy")
	}

	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		pkg = "myservice"
	}
	makePublicFn := func(id string) string { return id }
	if exportAllTypes {
		makePublicFn = makePublic
	}

	r, err := ParseLocation(file)
	if err != nil {
		return nil, err
	}

	return &GoWSDL{
		loc:          r,
		pkg:          pkg,
		ignoreTLS:    ignoreTLS,
		makePublicFn: makePublicFn,
		nsToPkg:      nsToPkg,
		pkgBaseURL:   pkgBaseURL,
		imports:      make(map[string][]string),
	}, nil
}

type GenerationResult struct {
	// Types is a map from package to generated code
	// If no subpackages are used, the key is ""
	Types        map[string][]byte
	Header       map[string][]byte
	Operations   []byte
	Server       []byte
	ServerHeader []byte
	ServerWSDL   []byte
}

func NewGenerationResult() *GenerationResult {
	return &GenerationResult{
		Types:  make(map[string][]byte),
		Header: make(map[string][]byte),
	}
}

// Start initiaties the code generation process by starting two goroutines: one
// to generate types and another one to generate operations.
func (g *GoWSDL) Start() (*GenerationResult, error) {
	result := NewGenerationResult()

	err := g.unmarshal()
	if err != nil {
		return nil, err
	}

	// Process WSDL nodes
	for _, schema := range g.wsdl.Types.Schemas {
		newTraverser(schema, g.wsdl.Types.Schemas).traverse()
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error

		if len(g.nsToPkg) == 0 {
			result.Types[""], err = g.genTypes("")
			if err != nil {
				log.Println("genTypes", "error", err)
			}
		} else {
			for ns, pkg := range g.nsToPkg {
				result.Types[pkg], err = g.genTypes(ns)
				if err != nil {
					log.Println("genTypes", "error", err)
				}
			}
		}
		log.Println("genTypes", "done")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error

		result.Operations, err = g.genOperations()
		if err != nil {
			log.Println(err)
		}
		log.Println("genOperations", "done")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error

		result.Server, err = g.genServer()
		if err != nil {
			log.Println(err)
		}
		log.Println("genServer", "done")
	}()

	wg.Wait()

	if len(g.nsToPkg) == 0 {
		result.Header[""], err = g.genHeader("")
		if err != nil {
			log.Println(err)
		}
	} else {
		for ns, pkg := range g.nsToPkg {
			result.Header[pkg], err = g.genHeader(ns)
			if err != nil {
				log.Println(err)
			}
		}
	}
	log.Println("genHeader", "done")

	result.ServerHeader, err = g.genServerHeader()
	if err != nil {
		log.Println(err)
	}
	log.Println("genServerHeader", "done")

	result.ServerWSDL = []byte("var wsdl = `" + string(g.rawWSDL) + "`")
	log.Println("genServerWSDL", "done")

	return result, nil
}

func (g *GoWSDL) fetchFile(loc *Location) (data []byte, err error) {
	if loc.f != "" {
		log.Println("Reading", "file", loc.f)
		data, err = ioutil.ReadFile(loc.f)
	} else {
		log.Println("Downloading", "file", loc.u.String())
		data, err = downloadFile(loc.u.String(), g.ignoreTLS)
	}
	return
}

func (g *GoWSDL) unmarshal() error {
	data, err := g.fetchFile(g.loc)
	if err != nil {
		return err
	}

	g.wsdl = new(WSDL)
	err = xml.Unmarshal(data, g.wsdl)
	if err != nil {
		return err
	}
	g.rawWSDL = data

	var newSchemas []*XSDSchema
	for _, schema := range g.wsdl.Types.Schemas {
		log.Println("Resolving XSD externals", "schema", schema.TargetNamespace)
		if len(g.nsToPkg) > 0 && g.getNSPackage(schema.TargetNamespace) == "" {
			return fmt.Errorf("no package mapping for namespace %s", schema.TargetNamespace)
		}
		schemas, err := g.resolveXSDExternals(schema, g.loc)
		if err != nil {
			return err
		}
		newSchemas = append(newSchemas, schemas...)
	}
	g.wsdl.Types.Schemas = append(g.wsdl.Types.Schemas, newSchemas...)

	return nil
}

func (g *GoWSDL) resolveXSDExternals(schema *XSDSchema, loc *Location) ([]*XSDSchema, error) {
	download := func(base *Location, ref string) ([]*XSDSchema, error) {
		location, err := base.Parse(ref)
		if err != nil {
			return nil, err
		}
		schemaKey := location.String()
		if g.resolvedXSDExternals[location.String()] {
			return nil, nil
		}
		if g.resolvedXSDExternals == nil {
			g.resolvedXSDExternals = make(map[string]bool, maxRecursion)
		}
		g.resolvedXSDExternals[schemaKey] = true

		var data []byte
		if data, err = g.fetchFile(location); err != nil {
			return nil, err
		}

		var downloadResult []*XSDSchema
		newschema := new(XSDSchema)

		err = xml.Unmarshal(data, newschema)
		if err != nil {
			return nil, err
		}

		if (len(newschema.Includes) > 0 || len(newschema.Imports) > 0) &&
			maxRecursion > g.currentRecursionLevel {
			g.currentRecursionLevel++

			schemas, err := g.resolveXSDExternals(newschema, location)
			if err != nil {
				return nil, err
			}
			downloadResult = append(downloadResult, schemas...)
		}

		log.Println("Adding external schema for", newschema.TargetNamespace)
		if len(g.nsToPkg) > 0 && g.getNSPackage(newschema.TargetNamespace) == "" {
			return nil, fmt.Errorf("no package mapping for namespace %s", newschema.TargetNamespace)
		}

		return append(downloadResult, newschema), nil
	}

	var result []*XSDSchema

	for _, impts := range schema.Imports {
		// Download the file only if we have a hint in the form of schemaLocation.
		if impts.SchemaLocation == "" {
			log.Printf("[WARN] Don't know where to find XSD for %s", impts.Namespace)
			continue
		}

		newSchemas, err := download(loc, impts.SchemaLocation)
		if err != nil {
			return nil, err
		}
		if newSchemas != nil {
			result = append(result, newSchemas...)
		}
	}

	for _, incl := range schema.Includes {
		newSchemas, err := download(loc, incl.SchemaLocation)
		if err != nil {
			return nil, err
		}
		if newSchemas != nil {
			result = append(result, newSchemas...)
		}
	}

	return result, nil
}

func (g *GoWSDL) genTypes(ns string) ([]byte, error) {
	funcMap := template.FuncMap{
		"toGoType":                 g.toGoType,
		"stripns":                  stripns,
		"addBlank":                 addBlank,
		"replaceReservedWords":     replaceReservedWords,
		"replaceAttrReservedWords": replaceAttrReservedWords,
		"normalize":                normalize,
		"makePublic":               g.makePublicFn,
		"makeFieldPublic":          makePublic,
		"comment":                  comment,
		"removeNS":                 removeNS,
		"getNSPrefix":              getNSPrefix,
		"goString":                 goString,
		"findNameByType":           g.findNameByType,
		"removePointerFromType":    removePointerFromType,
		"setNS":                    g.setNS,
		"getNS":                    g.getNS,
		"setNSMap":                 g.setNSMap,
		"getNSFromMap":             g.getNSFromMap,
		"wrapElement":              wrapElement,
		"getNSPackage":             g.getNSPackage,
	}

	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("types").Funcs(funcMap).Parse(typesTmpl))
	err := tmpl.Execute(data, g.wsdl.Types.FilterNamespace(ns))
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func (g *GoWSDL) genOperations() ([]byte, error) {
	funcMap := template.FuncMap{
		"toGoType":             g.toGoType,
		"stripns":              stripns,
		"replaceReservedWords": replaceReservedWords,
		"normalize":            normalize,
		"makePublic":           g.makePublicFn,
		"makePrivate":          makePrivate,
		"findType":             g.findType,
		"findSOAPAction":       g.findSOAPAction,
		"findServiceAddress":   g.findServiceAddress,
	}

	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("operations").Funcs(funcMap).Parse(opsTmpl))
	err := tmpl.Execute(data, g.wsdl.PortTypes)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func (g *GoWSDL) genServer() ([]byte, error) {
	funcMap := template.FuncMap{
		"toGoType":             g.toGoType,
		"stripns":              stripns,
		"replaceReservedWords": replaceReservedWords,
		"makePublic":           g.makePublicFn,
		"findType":             g.findType,
		"findSOAPAction":       g.findSOAPAction,
		"findServiceAddress":   g.findServiceAddress,
	}

	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("server").Funcs(funcMap).Parse(serverTmpl))
	err := tmpl.Execute(data, g.wsdl.PortTypes)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func (g *GoWSDL) genHeader(ns string) ([]byte, error) {
	funcMap := template.FuncMap{
		"toGoType":             g.toGoType,
		"stripns":              stripns,
		"replaceReservedWords": replaceReservedWords,
		"normalize":            normalize,
		"makePublic":           g.makePublicFn,
		"findType":             g.findType,
		"comment":              comment,
	}

	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("header").Funcs(funcMap).Parse(headerTmpl))

	var pkg string
	if ns != "" {
		pkg = g.getNSPackage(ns)
	} else {
		pkg = g.pkg
	}

	err := tmpl.Execute(data, struct {
		Pkg     string
		BaseURL string
		Imports []string
	}{
		Pkg:     pkg,
		BaseURL: g.pkgBaseURL,
		Imports: g.imports[ns],
	})
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func (g *GoWSDL) genServerHeader() ([]byte, error) {
	funcMap := template.FuncMap{
		"toGoType":             g.toGoType,
		"stripns":              stripns,
		"replaceReservedWords": replaceReservedWords,
		"makePublic":           g.makePublicFn,
		"findType":             g.findType,
		"comment":              comment,
	}

	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("server_header").Funcs(funcMap).Parse(serverHeaderTmpl))
	err := tmpl.Execute(data, g.pkg)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

var reservedWords = map[string]string{
	"break":       "break_",
	"default":     "default_",
	"func":        "func_",
	"interface":   "interface_",
	"select":      "select_",
	"case":        "case_",
	"defer":       "defer_",
	"go":          "go_",
	"map":         "map_",
	"struct":      "struct_",
	"chan":        "chan_",
	"else":        "else_",
	"goto":        "goto_",
	"package":     "package_",
	"switch":      "switch_",
	"const":       "const_",
	"fallthrough": "fallthrough_",
	"if":          "if_",
	"range":       "range_",
	"type":        "type_",
	"continue":    "continue_",
	"for":         "for_",
	"import":      "import_",
	"return":      "return_",
	"var":         "var_",
}

var reservedWordsInAttr = map[string]string{
	"break":       "break_",
	"default":     "default_",
	"func":        "func_",
	"interface":   "interface_",
	"select":      "select_",
	"case":        "case_",
	"defer":       "defer_",
	"go":          "go_",
	"map":         "map_",
	"struct":      "struct_",
	"chan":        "chan_",
	"else":        "else_",
	"goto":        "goto_",
	"package":     "package_",
	"switch":      "switch_",
	"const":       "const_",
	"fallthrough": "fallthrough_",
	"if":          "if_",
	"range":       "range_",
	"type":        "type_",
	"continue":    "continue_",
	"for":         "for_",
	"import":      "import_",
	"return":      "return_",
	"var":         "var_",
	"string":      "astring",
}

var specialCharacterMapping = map[string]string{
	"+": "Plus",
	"@": "At",
}

// Replaces Go reserved keywords to avoid compilation issues
func replaceReservedWords(identifier string) string {
	value := reservedWords[identifier]
	if value != "" {
		return value
	}
	return normalize(identifier)
}

// Replaces Go reserved keywords to avoid compilation issues
func replaceAttrReservedWords(identifier string) string {
	value := reservedWordsInAttr[identifier]
	if value != "" {
		return value
	}
	return normalize(identifier)
}

// Normalizes value to be used as a valid Go identifier, avoiding compilation issues
func normalize(value string) string {
	for k, v := range specialCharacterMapping {
		value = strings.ReplaceAll(value, k, v)
	}

	mapping := func(r rune) rune {
		if r == '.' || r == '-' {
			return '_'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return r
		}
		return -1
	}

	return strings.Map(mapping, value)
}

func goString(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}

var xsd2GoTypes = map[string]string{
	"string":             "string",
	"token":              "string",
	"float":              "float32",
	"double":             "float64",
	"decimal":            "float64",
	"integer":            "int32",
	"int":                "int32",
	"short":              "int16",
	"byte":               "int8",
	"long":               "int64",
	"boolean":            "bool",
	"datetime":           "soap.XSDDateTime",
	"date":               "soap.XSDDate",
	"time":               "soap.XSDTime",
	"base64binary":       "[]byte",
	"hexbinary":          "[]byte",
	"unsignedint":        "uint32",
	"nonnegativeinteger": "uint32",
	"unsignedshort":      "uint16",
	"unsignedbyte":       "byte",
	"unsignedlong":       "uint64",
	"anytype":            "AnyType",
	"ncname":             "NCName",
	"anyuri":             "AnyURI",
}

func getNSPrefix(xsdType string) string {
	r := strings.Split(xsdType, ":")

	if len(r) == 2 {
		return r[0]
	}
	return ""
}

func removeNS(xsdType string) string {
	// Handles name space, ie. xsd:string, xs:string
	r := strings.Split(xsdType, ":")

	if len(r) == 2 {
		return r[1]
	}

	return r[0]
}

func (g *GoWSDL) toGoType(xsdType string, nillable bool, minOccurs string) string {
	// Handles name space, ie. xsd:string, xs:string
	r := strings.Split(xsdType, ":")

	gotype := r[0]

	if len(r) == 2 {
		gotype = r[1]
	}

	value := xsd2GoTypes[strings.ToLower(gotype)]

	if value == "" {
		value = replaceReservedWords(makePublic(gotype))
		if len(r) == 2 {
			if ns := g.getNSFromMap(r[0]); ns != g.currentNamespace {
				if pkg := g.getNSPackage(ns); pkg != "" {
					value = fmt.Sprintf("%s.%s", pkg, value)
					if pkgList, found := g.imports[g.currentNamespace]; found {
						if !slices.Contains(pkgList, pkg) {
							g.imports[g.currentNamespace] = append(g.imports[g.currentNamespace], pkg)
						}
					} else {
						g.imports[g.currentNamespace] = []string{pkg}
					}
				}
			}
		}

	}

	if nillable || minOccurs == "0" {
		value = "*" + value
	}
	return value

}

func removePointerFromType(goType string) string {
	return regexp.MustCompile(`^\\s*\\*`).ReplaceAllLiteralString(goType, "")
}

// Given a message, finds its type.
//
// I'm not very proud of this function but
// it works for now and performance doesn't
// seem critical at this point
func (g *GoWSDL) findType(message string) string {
	message = stripns(message)

	for _, msg := range g.wsdl.Messages {
		if msg.Name != message {
			continue
		}

		// Assumes document/literal wrapped WS-I
		if len(msg.Parts) == 0 {
			// Message does not have parts. This could be a Port
			// with HTTP binding or SOAP 1.2 binding, which are not currently
			// supported.
			log.Printf("[WARN] %s message doesn't have any parts, ignoring message...", msg.Name)
			continue
		}

		part := msg.Parts[0]
		if part.Type != "" {
			return stripns(part.Type)
		}

		elRef := stripns(part.Element)

		for _, schema := range g.wsdl.Types.Schemas {
			for _, el := range schema.Elements {
				if strings.EqualFold(elRef, el.Name) {
					if el.Type != "" {
						return stripns(el.Type)
					}
					return el.Name
				}
			}
		}
	}
	return ""
}

// Given a type, check if there's an Element with that type, and return its name.
func (g *GoWSDL) findNameByType(name string) string {
	return newTraverser(nil, g.wsdl.Types.Schemas).findNameByType(name)
}

// TODO(c4milo): Add support for namespaces instead of striping them out
// TODO(c4milo): improve runtime complexity if performance turns out to be an issue.
func (g *GoWSDL) findSOAPAction(operation, portType string) string {
	for _, binding := range g.wsdl.Binding {
		if strings.ToUpper(stripns(binding.Type)) != strings.ToUpper(portType) {
			continue
		}

		for _, soapOp := range binding.Operations {
			if soapOp.Name == operation {
				return soapOp.SOAPOperation.SOAPAction
			}
		}
	}
	return ""
}

func (g *GoWSDL) findServiceAddress(name string) string {
	for _, service := range g.wsdl.Service {
		for _, port := range service.Ports {
			if port.Name == name {
				return port.SOAPAddress.Location
			}
		}
	}
	return ""
}

// TODO(c4milo): Add namespace support instead of stripping it
func stripns(xsdType string) string {
	r := strings.Split(xsdType, ":")
	t := r[0]

	if len(r) == 2 {
		t = r[1]
	}

	return t
}

func addBlank(s string) string {
	if s == "" {
		return ""
	} else {
		return s + " "
	}
}

func makePublic(identifier string) string {
	if isBasicType(identifier) {
		return identifier
	}
	if identifier == "" {
		return "EmptyString"
	}
	field := []rune(identifier)
	if len(field) == 0 {
		return identifier
	}

	field[0] = unicode.ToUpper(field[0])
	return string(field)
}

func wrapElement(elements []*XSDElement, parentName string) interface{} {
	type wrappedElement struct {
		ParentName string
		Elements   []*XSDElement
	}
	return wrappedElement{parentName, elements}

}

var basicTypes = map[string]string{
	"string":      "string",
	"float32":     "float32",
	"float64":     "float64",
	"int":         "int",
	"int8":        "int8",
	"int16":       "int16",
	"int32":       "int32",
	"int64":       "int64",
	"bool":        "bool",
	"time.Time":   "time.Time",
	"[]byte":      "[]byte",
	"byte":        "byte",
	"uint16":      "uint16",
	"uint32":      "uint32",
	"uinit64":     "uint64",
	"interface{}": "interface{}",
}

func isBasicType(identifier string) bool {
	if _, exists := basicTypes[identifier]; exists {
		return true
	}
	return false
}

func makePrivate(identifier string) string {
	field := []rune(identifier)
	if len(field) == 0 {
		return identifier
	}

	field[0] = unicode.ToLower(field[0])
	return string(field)
}

func comment(text string) string {
	lines := strings.Split(text, "\n")

	var output string
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}

	// Helps to determine if there is an actual comment without screwing newlines
	// in real comments.
	hasComment := false

	for _, line := range lines {
		line = strings.TrimLeftFunc(line, unicode.IsSpace)
		if line != "" {
			hasComment = true
		}
		output += "\n// " + line
	}

	if hasComment {
		return output
	}
	return ""
}
