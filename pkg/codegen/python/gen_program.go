// Copyright 2016-2020, Pulumi Corporation.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package python

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/codegen"
	"github.com/pulumi/pulumi/pkg/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/codegen/hcl2/model/format"
	"github.com/pulumi/pulumi/pkg/util/contract"
)

type generator struct {
	// The formatter to use when generating code.
	*format.Formatter

	program         *model.Program
	outputDirectory string
	diagnostics     hcl.Diagnostics
}

func GenerateProgram(program *model.Program, outputDirectory string) (hcl.Diagnostics, error) {
	// Linearize the nodes into an order appropriate for procedural code generation.
	nodes := model.Linearize(program)

	g := &generator{
		program:         program,
		outputDirectory: outputDirectory,
	}
	g.Formatter = format.NewFormatter(g)

	index, err := os.Create(filepath.Join(outputDirectory, "__main__.py"))
	if err != nil {
		return nil, err
	}
	defer contract.IgnoreClose(index)

	g.genPreamble(index, program)

	for _, n := range nodes {
		g.genNode(index, n)
	}

	return g.diagnostics, nil
}

func pyName(pulumiName string, isObjectKey bool) string {
	if isObjectKey {
		return fmt.Sprintf("%q", pulumiName)
	}
	return PyName(cleanName(pulumiName))
}

func (g *generator) genPreamble(w io.Writer, program *model.Program) {
	// Print the pulumi import at the top.
	g.Fprintln(w, "import pulumi")

	// Accumulate other imports for the various providers. Don't emit them yet, as we need to sort them later on.
	var imports []string
	importSet := codegen.StringSet{}
	for _, n := range program.Nodes {
		// TODO: invokes
		if r, isResource := n.(*model.Resource); isResource {
			pkg, _, _, _ := r.DecomposeToken()

			if !importSet.Has(pkg) {
				imports = append(imports, fmt.Sprintf("import pulumi_%[1]s as %[1]s", pkg))
				importSet.Add(pkg)
			}
		}
	}

	// Now sort the imports, so we emit them deterministically, and emit them.
	sort.Strings(imports)
	for _, line := range imports {
		g.Fprintln(w, line)
	}
	g.Fprint(w, "\n")
}

func (g *generator) genNode(w io.Writer, n model.Node) {
	switch n := n.(type) {
	case *model.Resource:
		g.genResource(w, n)
	case *model.ConfigVariable:
	case *model.LocalVariable:
	case *model.OutputVariable:
	}
}

// resourceTypeName computes the NodeJS package, module, and type name for the given resource.
func resourceTypeName(r *model.Resource) (string, string, string, hcl.Diagnostics) {
	// Compute the resource type from the Pulumi type token.
	pkg, module, member, diagnostics := r.DecomposeToken()
	return pyName(pkg, false), strings.Replace(module, "/", ".", -1), title(member), diagnostics
}

// makeResourceName returns the expression that should be emitted for a resource's "name" parameter given its base name
// and the count variable name, if any.
func (g *generator) makeResourceName(baseName, count string) string {
	if count == "" {
		return fmt.Sprintf(`"%s"`, baseName)
	}
	return fmt.Sprintf(`f"%s-${%s}"`, baseName, count)
}

// genResource handles the generation of instantiations of non-builtin resources.
func (g *generator) genResource(w io.Writer, r *model.Resource) {
	pkg, module, memberName, diagnostics := resourceTypeName(r)
	g.diagnostics = append(g.diagnostics, diagnostics...)

	if module != "" {
		module = "." + module
	}

	qualifiedMemberName := fmt.Sprintf("%s%s.%s", pkg, module, memberName)

	optionsBag := ""

	inputs := r.Inputs.(*model.ObjectConsExpression)

	name := pyName(r.Name(), false)
	resName := g.makeResourceName(name, "")
	g.Fgenf(w, "%s%s = %s(%s", g.Indent, name, qualifiedMemberName, resName)
	indenter := func(f func()) { f() }
	if len(inputs.Items) > 1 {
		indenter = g.Indented
	}
	indenter(func() {
		for _, item := range inputs.Items {
			lit := item.Key.(*model.LiteralValueExpression)
			propertyName := pyName(lit.Value.StringValue(), false)
			if len(inputs.Items) == 1 {
				g.Fgenf(w, ", %s=%v", propertyName, item.Value)
			} else {
				g.Fgenf(w, ",\n%s%s=%v", g.Indent, propertyName, item.Value)
			}
		}
	})
	g.Fgenf(w, "%s)\n", optionsBag)
}

func (g *generator) genNYI(w io.Writer, reason string, vs ...interface{}) {
	g.Fgenf(w, "(lambda: throw Error(%q))()", fmt.Sprintf(reason, vs...))
}