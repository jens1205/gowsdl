// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
/*

Gowsdl generates Go code from a WSDL file.

This project is originally intended to generate Go clients for WS-* services.

Usage: gowsdl [options] myservice.wsdl
  -o string
        File where the generated code will be saved (default "myservice.go")
  -p string
        Package under which code will be generated (default "myservice")
  -v    Shows gowsdl version

Features

Supports only Document/Literal wrapped services, which are WS-I (http://ws-i.org/) compliant.

Attempts to generate idiomatic Go code as much as possible.

Supports WSDL 1.1, XML Schema 1.0, SOAP 1.1.

Resolves external XML Schemas

Supports providing WSDL HTTP URL as well as a local WSDL file.

Not supported

UDDI.

TODO

Add support for filters to allow the user to change the generated code.

If WSDL file is local, resolve external XML schemas locally too instead of failing due to not having a URL to download them from.

Resolve XSD element references.

Support for generating namespaces.

Make code generation agnostic so generating code to other programming languages is feasible through plugins.

*/

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	gen "github.com/jens1205/gowsdl"
)

// Version is initialized in compilation time by go build.
var Version string

// Name is initialized in compilation time by go build.
var Name string

var vers = flag.Bool("v", false, "Shows gowsdl version")
var pkg = flag.String("p", "myservice", "Package under which code will be generated")
var pkgBaseUrl = flag.String("pkgBaseUrl", "", "Base URL for the generated package")
var outFile = flag.String("o", "myservice.go", "File where the generated code will be saved")
var dir = flag.String("d", "./", "Directory under which package directory will be created")
var insecure = flag.Bool("i", false, "Skips TLS Verification")
var makePublic = flag.Bool("make-public", true, "Make the generated types public/exported")
var nsToPkg gen.NamespaceMapping = make(map[string]string)

func init() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
	log.SetPrefix("🍀  ")
	flag.Var(&nsToPkg, "pkg", "Namespace to package mapping. Format: pkg=ns")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] myservice.wsdl\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if len(nsToPkg) > 0 && *pkgBaseUrl == "" {
		log.Fatalln("pkgBaseUrl is required when using pkg")
	}

	// Show app version
	if *vers {
		log.Println(Version)
		os.Exit(0)
	}

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(0)
	}

	wsdlPath := os.Args[len(os.Args)-1]

	if *outFile == wsdlPath {
		log.Fatalln("Output file cannot be the same WSDL file")
	}

	// load wsdl
	gowsdl, err := gen.NewGoWSDL(wsdlPath, *pkg, nsToPkg, *pkgBaseUrl, *insecure, *makePublic)
	if err != nil {
		log.Fatalln(err)
	}

	// generate code
	generationResult, err := gowsdl.Start()
	if err != nil {
		log.Fatalln(err)
	}

	pkgPath := filepath.Join(*dir, *pkg)
	err = os.Mkdir(pkgPath, 0744)
	if !errors.Is(err, fs.ErrExist) {
		log.Fatalln(err)
	}
	for _, subPkg := range nsToPkg.GetPackages() {
		err = os.Mkdir(filepath.Join(pkgPath, subPkg), 0744)
		if !errors.Is(err, fs.ErrExist) {
			log.Fatalln(err)
		}
	}

	file, err := os.Create(filepath.Join(pkgPath, *outFile))
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	data := new(bytes.Buffer)
	data.Write(generationResult.Header[""])
	data.Write(generationResult.Types[""])
	data.Write(generationResult.Operations)

	// go fmt the generated code
	source, err := format.Source(data.Bytes())
	if err != nil {
		_, _ = file.Write(data.Bytes())
		log.Fatalln(err)
	}

	_, _ = file.Write(source)

	// all types in subpackages
	for _, subPkg := range nsToPkg.GetPackages() {
		log.Println("subPkg", subPkg)
		log.Println("pkg", pkgPath)
		log.Println("Generating", filepath.Join(pkgPath, subPkg, subPkg+".go"))
		pkgFile, err := os.Create(filepath.Join(pkgPath, subPkg, subPkg+".go"))
		if err != nil {
			log.Fatalln(err)
		}
		defer pkgFile.Close()
		data := new(bytes.Buffer)
		data.Write(generationResult.Header[subPkg])
		data.Write(generationResult.Types[subPkg])

		// go fmt the generated code
		source, err := format.Source(data.Bytes())
		if err != nil {
			_, _ = pkgFile.Write(data.Bytes())
			log.Fatalln(err)
		}

		_, _ = pkgFile.Write(source)
	}

	// server
	serverFile, err := os.Create(filepath.Join(pkgPath, "server"+*outFile))
	if err != nil {
		log.Fatalln(err)
	}
	defer serverFile.Close()

	serverData := new(bytes.Buffer)
	serverData.Write(generationResult.ServerHeader)
	serverData.Write(generationResult.ServerWSDL)
	serverData.Write(generationResult.Server)

	serverSource, err := format.Source(serverData.Bytes())
	if err != nil {
		serverFile.Write(serverData.Bytes())
		log.Fatalln(err)
	}
	serverFile.Write(serverSource)

	log.Println("Done 👍")
}
