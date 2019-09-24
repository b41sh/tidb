// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

// +build ignore

package main

import (
	"bytes"
	"flag"
	"go/format"
	"io/ioutil"
	"log"
	"path/filepath"
	"text/template"
)

const header = `// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by go generate in expression/generator; DO NOT EDIT.

package expression
`

const newLine = "\n"

const builtinControlImports = `import (
	"github.com/pingcap/tidb/types"
	"github.com/pingcap/tidb/util/chunk"
)
`

var builtinIfVec = template.Must(template.New("").Parse(`
func (b *builtinIf{{ .TypeName }}Sig) vecEval{{ .TypeName }}(input *chunk.Chunk, result *chunk.Column) error {
	n := input.NumRows()
	buf0, err := b.bufAllocator.get(types.ETInt, n)
	if err != nil {
		return err
	}
	defer b.bufAllocator.put(buf0)
	if err := b.args[0].VecEvalInt(b.ctx, input, buf0); err != nil {
		return err
	}

	buf1, err := b.bufAllocator.get(types.ET{{ .ETName }}, n)
	if err != nil {
		return err
	}
	defer b.bufAllocator.put(buf1)
	if err := b.args[1].VecEval{{ .TypeName }}(b.ctx, input, buf1); err != nil {
		return err
	}

	buf2, err := b.bufAllocator.get(types.ET{{ .ETName }}, n)
	if err != nil {
		return err
	}
	defer b.bufAllocator.put(buf2)
	if err := b.args[2].VecEval{{ .TypeName }}(b.ctx, input, buf2); err != nil {
		return err
	}

{{ if .Fixed }}
	result.Resize{{ .TypeNameInColumn }}(n, false)
{{ else }}
	result.Reserve{{ .TypeNameInColumn }}(n)
{{ end }}
	arg0 := buf0.Int64s()
{{ if .Fixed }}
	arg1 := buf1.{{ .TypeNameInColumn }}s()
	arg2 := buf2.{{ .TypeNameInColumn }}s()
	rs := result.{{ .TypeNameInColumn }}s()
{{ end }}
	for i := 0; i < n; i++ {
		arg := arg0[i]
		isNull0 := buf0.IsNull(i)
		switch {
		case isNull0 || arg == 0:
{{ if .Fixed }}
			if buf2.IsNull(i) {
				result.SetNull(i, true)
			} else {
				rs[i] = arg2[i]
			}
{{ else }}
			if buf2.IsNull(i) {
				result.AppendNull()
			} else {
				result.Append{{ .TypeNameInColumn }}(buf2.Get{{ .TypeNameInColumn }}(i))
			}
{{ end }}
		case arg != 0:
{{ if .Fixed }}
			if buf1.IsNull(i) {
				result.SetNull(i, true)
			} else {
				rs[i] = arg1[i]
			}
{{ else }}
			if buf1.IsNull(i) {
				result.AppendNull()
			} else {
				result.Append{{ .TypeNameInColumn }}(buf1.Get{{ .TypeNameInColumn }}(i))
			}
{{ end }}
		}
	}
	return nil
}

func (b *builtinIf{{ .TypeName }}Sig) vectorized() bool {
	return true
}
`))

var builtinControlVecTest = template.Must(template.New("").Parse(`
import (
	"testing"

	. "github.com/pingcap/check"
	"github.com/pingcap/parser/ast"
	"github.com/pingcap/tidb/types"
)

{{/* Add more test cases here if we have more functions in this file */}}
var vecBuiltinControlCases = map[string][]vecExprBenchCase{
	ast.If: {
{{ range . }}
		{retEvalType: types.ET{{ .ETName }}, childrenTypes: []types.EvalType{types.ETInt, types.ET{{ .ETName }}, types.ET{{ .ETName }}}},
{{ end }}
	},
}

func (s *testEvaluatorSuite) TestVectorizedBuiltinControlEvalOneVec(c *C) {
	testVectorizedEvalOneVec(c, vecBuiltinControlCases)
}

func (s *testEvaluatorSuite) TestVectorizedBuiltinControlFunc(c *C) {
	testVectorizedBuiltinFunc(c, vecBuiltinControlCases)
}

func BenchmarkVectorizedBuiltinControlEvalOneVec(b *testing.B) {
	benchmarkVectorizedEvalOneVec(b, vecBuiltinControlCases)
}

func BenchmarkVectorizedBuiltinControlFunc(b *testing.B) {
	benchmarkVectorizedBuiltinFunc(b, vecBuiltinControlCases)
}
`))

type typeContext struct {
	// Describe the name of "github.com/pingcap/tidb/types".ET{{ .ETName }}
	ETName string
	// Describe the name of "github.com/pingcap/tidb/expression".VecExpr.VecEval{{ .TypeName }}
	// If undefined, it's same as ETName.
	TypeName string
	// Describe the name of "github.com/pingcap/tidb/util/chunk".*Column.Append{{ .TypeNameInColumn }},
	// Resize{{ .TypeNameInColumn }}, Reserve{{ .TypeNameInColumn }}, Get{{ .TypeNameInColumn }} and
	// {{ .TypeNameInColumn }}s.
	// If undefined, it's same as TypeName.
	TypeNameInColumn string
	// Same as "github.com/pingcap/tidb/util/chunk".getFixedLen()
	Fixed bool
}

var typesMap = []typeContext{
	{ETName: "Int", TypeNameInColumn: "Int64", Fixed: true},
	{ETName: "Real", TypeNameInColumn: "Float64", Fixed: true},
	{ETName: "Decimal", Fixed: true},
	{ETName: "String", Fixed: false},
	{ETName: "Datetime", TypeName: "Time", Fixed: true},
	{ETName: "Duration", TypeNameInColumn: "GoDuration", Fixed: true},
	{ETName: "Json", TypeName: "JSON", Fixed: false},
}

func generateDotGo(fileName string, imports string, types []typeContext, tmpls ...*template.Template) error {
	w := new(bytes.Buffer)
	w.WriteString(header)
	w.WriteString(newLine)
	w.WriteString(imports)
	for _, tmpl := range tmpls {
		for _, ctx := range types {
			if ctx.TypeName == "" {
				ctx.TypeName = ctx.ETName
			}
			if ctx.TypeNameInColumn == "" {
				ctx.TypeNameInColumn = ctx.TypeName
			}
			err := tmpl.Execute(w, ctx)
			if err != nil {
				return err
			}
		}
	}
	data, err := format.Source(w.Bytes())
	if err != nil {
		log.Println("[Warn]", fileName+": gofmt failed", err)
		data = w.Bytes() // write original data for debugging
	}
	return ioutil.WriteFile(fileName, data, 0644)
}

func generateTestDotGo(fileName string, types []typeContext) error {
	w := new(bytes.Buffer)
	w.WriteString(header)
	err := builtinControlVecTest.Execute(w, types)
	if err != nil {
		return err
	}
	data, err := format.Source(w.Bytes())
	if err != nil {
		log.Println("[Warn]", fileName+": gofmt failed", err)
		data = w.Bytes() // write original data for debugging
	}
	return ioutil.WriteFile(fileName, data, 0644)
}

// generateOneFile generate one xxx.go file and the associated xxx_test.go file.
func generateOneFile(fileNamePrefix string, imports string, types []typeContext,
	tmpls ...*template.Template) (err error) {

	err = generateDotGo(fileNamePrefix+".go", imports, types,
		tmpls...,
	)
	if err != nil {
		return
	}
	err = generateTestDotGo(fileNamePrefix+"_test.go", types)
	return
}

func main() {
	flag.Parse()
	var err error
	outputDir := "."
	err = generateOneFile(filepath.Join(outputDir, "builtin_control_vec_generated"), builtinControlImports, typesMap,
		// Add to the list if the file has more template to execute.
		builtinIfVec,
	)
	if err != nil {
		log.Fatalln("generateOneFile", err)
	}
}
