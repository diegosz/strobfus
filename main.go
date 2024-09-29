// Copyright © 2018 Zenly <hello@zen.ly>.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/tools/go/ast/astutil"
)

const version = "0.0.2000000002002"

const header = `// Code generated by "strobfus" v` + version + `; DO NOT EDIT.
// source: https://github.com/diegosz/strobfus

`

const tmpl = `
func init() {
	var __privateKeyObfuscator = []byte{
		{{- range .PrivateKey }}
		{{ . }}
		{{- end}}
	}
	var __nonceObfuscator = []byte{
		{{- range .Nonce }}
		{{ . }}
		{{- end}}
	}

	block, err := aes.NewCipher(__privateKeyObfuscator)
	if err != nil {
		panic(err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}

	{{- range .Variables }}
	{
		{{ if .IsArray -}}
		var __{{ .Name }} = [][]byte{
		{{- range $i, $e := .Values }}
			{
				{{- range $e }}
				{{ . }}
				{{- end}}
			},
		{{- end}}
		}
		ret := make([]string, 0, len(__{{ .Name }}))
		for _, v := range __{{ .Name }} {
			plaintext, err := aesgcm.Open(nil, __nonceObfuscator, v, nil)
			if err != nil {
				panic(err)
			}
			ret = append(ret, string(plaintext))
		}
		{{ .Name }} = ret
		{{- else -}}
		var __{{ .Name }} = []byte{
			{{- range ( index .Values 0) }}
			{{ . }}
			{{- end}}
		}
		plaintext, err := aesgcm.Open(nil, __nonceObfuscator, __{{ .Name }}, nil)
		if err != nil {
			panic(err)
		}
		{{ .Name }} = string(plaintext)
		{{- end }}
	}
	{{- end -}}
	{{ .InitCode -}}
}
`

type variable struct {
	Name    string
	Values  [][]string
	IsArray bool
}

var (
	_output   = flag.String("output", "", "output file name; default <file>_gen.go; you can pass stdout to print the result on the standard output")
	_filename = flag.String("filename", "", "name of the file to be obfuscate")
	_seed     = flag.String("seed", "", "random seed. used to generate deterministic output.")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("strobfus: ")

	checkArgs()

	content, err := ioutil.ReadFile(*_filename)
	if err != nil {
		log.Fatal(err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		log.Fatalf("unable to parse %s: %v", *_filename, err)
	}

	key, nonce, aesgcm, err := setupAES()
	if err != nil {
		log.Fatal(err)
	}

	initCode := "\n"
	variables := make([]*variable, 0)
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		switch typ := c.Node().(type) {
		case *ast.GenDecl:
			for _, spec := range typ.Specs {
				if vSpec, ok := spec.(*ast.ValueSpec); ok {
					for _, v := range vSpec.Values {
						obfuscated := &variable{Name: vSpec.Names[0].Name}
						// for each string or []string
						// we retrieve his name and its value(s)
						switch real := v.(type) {
						case *ast.BasicLit:
							if real.Kind == token.STRING && len(real.Value) > 2 {
								if value, err := strconv.Unquote(real.Value); err == nil {
									obfuscated.Values = [][]string{bytesToHex(aesgcm.Seal(nil, nonce, []byte(value), nil))}
									variables = append(variables, obfuscated)
									real.Value = `"" // ` + real.Value[1:len(real.Value)-1]
								}
							}
						case *ast.CompositeLit:
							for _, elt := range real.Elts {
								if inner, ok := elt.(*ast.BasicLit); ok && inner.Kind == token.STRING && len(inner.Value) > 2 {
									if value, err := strconv.Unquote(inner.Value); err == nil {
										obfuscated.Values = append(obfuscated.Values, bytesToHex(aesgcm.Seal(nil, nonce, []byte(value), nil)))
									}
								}
							}
							// TODO: find a way to add comments with the content of []string
							if len(obfuscated.Values) > 0 {
								obfuscated.IsArray = true
								variables = append(variables, obfuscated)
							}
							real.Elts = []ast.Expr{}
						}
					}
				}
			}
		case *ast.FuncDecl:
			// we have an init function in the file
			// we retrieve the content to append it at the end of our init
			if typ.Name.Name == "init" {
				from, to := int(typ.Body.Lbrace), int(typ.Body.Rbrace)
				initCode = string(content[from : to-1])
				c.Delete()
			}
		case *ast.File:
			// we filter out the comment with "go:build ignore" "+build ignore" and "go:generate strobfus"
			cpy := make([]*ast.CommentGroup, 0, len(typ.Comments))
			for _, comments := range typ.Comments {
				commentCpy := make([]*ast.Comment, 0, len(comments.List))
				for _, comment := range comments.List {
					if !(strings.Contains(comment.Text, "go:build ignore") ||
						strings.Contains(comment.Text, "+build ignore") ||
						strings.Contains(comment.Text, "go:generate strobfus")) {
						commentCpy = append(commentCpy, comment)
					}
				}
				if len(commentCpy) > 0 {
					comments.List = commentCpy
					cpy = append(cpy, comments)
				}
			}
			typ.Comments = cpy
		}
		return true
	}, nil)
	// add missing imports
	astutil.AddImport(fset, f, "crypto/aes")
	astutil.AddImport(fset, f, "crypto/cipher")

	var buf bytes.Buffer

	err = printer.Fprint(&buf, fset, f)
	if err != nil {
		log.Fatal(err)
	}

	vars, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	writer := bufio.NewWriter(getOutput(*_filename, *_output))
	writer.WriteString(header)
	writer.WriteString(string(vars))

	tmpl, err := template.New("").Parse(tmpl)
	if err != nil {
		log.Fatal(err)
	}

	err = tmpl.Execute(writer, struct {
		PrivateKey, Nonce []string
		Variables         []*variable
		InitCode          string
	}{
		PrivateKey: bytesToHex(key),
		Nonce:      bytesToHex(nonce),
		Variables:  variables,
		InitCode:   initCode,
	})
	if err != nil {
		log.Fatal(err)
	}

	writer.Flush()
}

// randomSeed returns a default random seed.
func randomSeed() string {
	randomSeed := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, randomSeed); err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(randomSeed)
}

// setupAES is a little helper to create a AES key
func setupAES() (key, nonce []byte, aesgcm cipher.AEAD, err error) {
	seed := *_seed
	if seed == "" {
		seed = randomSeed()
	}

	// derive AES key and nonce from the seed and a namespace in an insecure
	// way. We could use schemes like blake2b, but our security requirement is a
	// PRNG here, so using a regular crypto hash with concatenated buffers to
	// simulate one is enough.
	keyDerived := sha1.Sum([]byte(seed + ":key"))
	nonceDerived := sha1.Sum([]byte(seed + ":nonce"))

	// sha1 returns 20 byte. get only the bytes we need.
	key = keyDerived[:16]
	nonce = nonceDerived[:12]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, err
	}
	aesgcm, err = cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}
	return key, nonce, aesgcm, nil
}

// bytesToHex takes an array of bytes and returns an array of string formatted in hex
// with 16 characters by line.
func bytesToHex(value []byte) []string {
	ret := []string{}

	for len(value) > 0 {
		n := 16
		if n > len(value) {
			n = len(value)
		}

		s := ""
		for i, c := range value[:n] {
			if i == 0 {
				s += fmt.Sprintf("0x%02x,", c)
			} else {
				s += fmt.Sprintf(" 0x%02x,", c)
			}
		}
		ret = append(ret, s)
		value = value[n:]
	}
	return ret
}

// getOutput returns an io.Writer to the outputFile
// in case of an empty string, it returns an io.Writer on the outputFile_gen.go
// in case of "stdout", it returns os.Stdout
func getOutput(filename, outputFile string) io.Writer {
	out := (io.Writer)(os.Stdout)
	if outputFile == "stdout" {
		return out
	}
	if outputFile == "" {
		outputFile = strings.TrimSuffix(filename, filepath.Ext(filename)) + "_gen.go"
	}
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	out, err = os.Create(filepath.Join(wd, outputFile))
	if err != nil {
		log.Fatal(err)
	}
	return out
}

// usage is used by flag to print the helper
func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s (v%s):\n", os.Args[0], version)
	fmt.Fprintf(os.Stderr, "\tstrobfus -filename <file>.go\n")
	fmt.Fprintf(os.Stderr, "For more information, see:\n")
	fmt.Fprintf(os.Stderr, "\thttp://github.com/diegosz/strobfus\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

// checkArgs uses flag from stdlib to retrive the arguments and panic in case of invalid arguments
func checkArgs() {
	flag.Usage = usage
	flag.Parse()

	if *_filename == "" {
		usage()
		os.Exit(1)
	}
}
