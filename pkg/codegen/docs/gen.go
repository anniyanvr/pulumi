// Copyright 2016-2020, Pulumi Corporation.
//
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

// Pulling out some of the repeated strings tokens into constants would harm readability, so we just ignore the
// goconst linter's warning.
//
// nolint: lll, goconst
package docs

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/codegen/schema"
	"github.com/pulumi/pulumi/pkg/util/contract"
)

type stringSet map[string]struct{}

func (ss stringSet) add(s string) {
	ss[s] = struct{}{}
}

func (ss stringSet) has(s string) bool {
	_, ok := ss[s]
	return ok
}

type typeDetails struct {
	outputType   bool
	inputType    bool
	functionType bool
}

func title(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(append([]rune{unicode.ToUpper(runes[0])}, runes[1:]...))
}

func lower(s string) string {
	return strings.ToLower(s)
}

func escape(s string) string {
	return html.EscapeString(s)
}

func camel(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	res := make([]rune, 0, len(runes))
	for i, r := range runes {
		if unicode.IsLower(r) {
			res = append(res, runes[i:]...)
			break
		}
		res = append(res, unicode.ToLower(r))
	}
	return string(res)
}

type modContext struct {
	pkg         *schema.Package
	mod         string
	types       []*schema.ObjectType
	resources   []*schema.Resource
	functions   []*schema.Function
	typeDetails map[*schema.ObjectType]*typeDetails
	children    []*modContext
	tool        string
}

func (mod *modContext) details(t *schema.ObjectType) *typeDetails {
	details, ok := mod.typeDetails[t]
	if !ok {
		details = &typeDetails{}
		if mod.typeDetails == nil {
			mod.typeDetails = map[*schema.ObjectType]*typeDetails{}
		}
		mod.typeDetails[t] = details
	}
	return details
}

func (mod *modContext) tokenToType(tok string, input bool) string {
	// token := pkg : module : member
	// module := path/to/module

	components := strings.Split(tok, ":")
	contract.Assertf(len(components) == 3, "malformed token %v", tok)

	modName, name := mod.pkg.TokenToModule(tok), title(components[2])

	root := "outputs."
	if input {
		root = "inputs."
	}

	if modName != "" {
		modName = strings.Replace(modName, "/", ".", -1) + "."
	}

	return root + modName + title(name)
}

func tokenToName(tok string) string {
	components := strings.Split(tok, ":")
	contract.Assertf(len(components) == 3, "malformed token %v", tok)
	return title(components[2])
}

func resourceName(r *schema.Resource) string {
	if r.IsProvider {
		return "Provider"
	}
	return tokenToName(r.Token)
}

func (mod *modContext) pulumiTypeString(t schema.Type, link bool) string {
	var typ string
	switch t := t.(type) {
	case *schema.ArrayType:
		typ = fmt.Sprintf("%s[]", mod.pulumiTypeString(t.ElementType, link))
	case *schema.MapType:
		typ = fmt.Sprintf("map[string]%s", mod.pulumiTypeString(t.ElementType, link))
	case *schema.ObjectType:
		//typ = tokenToName(t.Token)
		return fmt.Sprintf("<a href=\"#%s\">%s</a>", lower(tokenToName(t.Token)), tokenToName(t.Token))
	case *schema.TokenType:
		typ = tokenToName(t.Token)
	case *schema.UnionType:
		var elements []string
		for _, e := range t.ElementTypes {
			elements = append(elements, mod.pulumiTypeString(e, link))
		}
		return strings.Join(elements, " | ")
	default:
		switch t {
		case schema.BoolType:
			typ = "boolean"
		case schema.IntType, schema.NumberType:
			typ = "number"
		case schema.StringType:
			typ = "string"
		case schema.ArchiveType:
			typ = "Archive"
		case schema.AssetType:
			typ = "Asset | Archive"
		case schema.AnyType:
			typ = "any"
		}
	}
	return typ
}

func (mod *modContext) typeString(t schema.Type, input, wrapInput, optional bool) string {
	var typ string
	switch t := t.(type) {
	case *schema.ArrayType:
		typ = mod.typeString(t.ElementType, input, wrapInput, false) + "[]"
	case *schema.MapType:
		typ = fmt.Sprintf("{[key: string]: %v}", mod.typeString(t.ElementType, input, wrapInput, false))
	case *schema.ObjectType:
		typ = mod.tokenToType(t.Token, input)
	case *schema.TokenType:
		typ = tokenToName(t.Token)
	case *schema.UnionType:
		var elements []string
		for _, e := range t.ElementTypes {
			elements = append(elements, mod.typeString(e, input, wrapInput, false))
		}
		return strings.Join(elements, " | ")
	default:
		switch t {
		case schema.BoolType:
			typ = "boolean"
		case schema.IntType, schema.NumberType:
			typ = "number"
		case schema.StringType:
			typ = "string"
		case schema.ArchiveType:
			typ = "pulumi.asset.Archive"
		case schema.AssetType:
			typ = "pulumi.asset.Asset | pulumi.asset.Archive"
		case schema.AnyType:
			typ = "any"
		}
	}

	if wrapInput && typ != "any" {
		typ = fmt.Sprintf("pulumi.Input<%s>", typ)
	}
	if optional {
		return typ + " | undefined"
	}
	return typ
}

func sanitizeComment(str string) string {
	return strings.Replace(str, "*/", "*&#47;", -1)
}

func printComment(w io.Writer, comment, deprecationMessage, indent string) {
	if comment == "" && deprecationMessage == "" {
		return
	}

	lines := strings.Split(sanitizeComment(comment), "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	fmt.Fprintf(w, "%s/**\n", indent)
	for _, l := range lines {
		if l == "" {
			fmt.Fprintf(w, "%s *\n", indent)
		} else {
			fmt.Fprintf(w, "%s * %s\n", indent, l)
		}
	}
	if deprecationMessage != "" {
		if len(lines) > 0 {
			fmt.Fprintf(w, "%s *\n", indent)
		}
		fmt.Fprintf(w, "%s * @deprecated %s\n", indent, deprecationMessage)
	}
	fmt.Fprintf(w, "%s */\n", indent)
}

func (mod *modContext) genPlainType(w io.Writer, name, comment string, properties []*schema.Property, input, wrapInput, readonly bool, level int) {
	indent := strings.Repeat("    ", level)

	printComment(w, comment, "", indent)

	fmt.Fprintf(w, "%sexport interface %s {\n", indent, name)
	for _, p := range properties {
		mod.genProperty(w, p, true, input, wrapInput, readonly, level+1)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

func (mod *modContext) genConstructor(w io.Writer, r *schema.Resource) {
	name := resourceName(r)

	allOptionalInputs := true
	for _, prop := range r.InputProperties {
		allOptionalInputs = allOptionalInputs && !prop.IsRequired
	}

	var argsFlags string
	if allOptionalInputs {
		// If the number of required input properties was zero, we can make the args object optional.
		argsFlags = "?"
	}
	argsType := name + "Args"

	fmt.Fprintf(w, "new %s(name: string, args%s: %s, opts?: pulumi.CustomResourceOptions);\n", name, argsFlags, argsType)
}

func (mod *modContext) genProperty(w io.Writer, prop *schema.Property, comment, input, wrapInput, readonly bool, level int) {
	indent := strings.Repeat("    ", level)

	if comment {
		printComment(w, prop.Comment, prop.DeprecationMessage, indent)
	}

	prefix := ""
	if readonly {
		prefix = "readonly "
	}

	sigil := ""
	if !prop.IsRequired {
		sigil = "?"
	}

	fmt.Fprintf(w, "%s%s%s%s: %s;\n", indent, prefix, prop.Name, sigil, mod.typeString(prop.Type, input, wrapInput, false))
}

func tsPrimitiveValue(value interface{}) (string, error) {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return "true", nil
		}
		return "false", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
	case reflect.String:
		return fmt.Sprintf("%q", v.String()), nil
	default:
		return "", errors.Errorf("unsupported default value of type %T", value)
	}
}

func (mod *modContext) getDefaultValue(dv *schema.DefaultValue, t schema.Type) (string, error) {
	var val string
	if dv.Value != nil {
		v, err := tsPrimitiveValue(dv.Value)
		if err != nil {
			return "", err
		}
		val = v
	}

	if len(dv.Environment) != 0 {
		getType := ""
		switch t {
		case schema.BoolType:
			getType = "Boolean"
		case schema.IntType, schema.NumberType:
			getType = "Number"
		}

		envVars := fmt.Sprintf("%q", dv.Environment[0])
		for _, e := range dv.Environment[1:] {
			envVars += fmt.Sprintf(", %q", e)
		}

		cast := ""
		if t != schema.StringType {
			cast = "<any>"
		}

		getEnv := fmt.Sprintf("%sutilities.getEnv%s(%s)", cast, getType, envVars)
		if val != "" {
			val = fmt.Sprintf("(%s || %s)", getEnv, val)
		} else {
			val = getEnv
		}
	}

	return val, nil
}

func (mod *modContext) genProperties(w io.Writer, properties []*schema.Property, input bool) {
	if len(properties) == 0 {
		return
	}

	for _, prop := range properties {
		var required string
		if input {
			required = "Optional "
			if prop.IsRequired {
				required = "Required "
			}
		}

		fmt.Fprintf(w, "* `%s` -- (%s<code>%s</code>) %s\n", prop.Name, required, mod.pulumiTypeString(prop.Type, true), prop.Comment)
	}

	// for _, prop := range properties {
	// 	fmt.Fprintf(w, "#### %s - %s\n\n", escape(prop.Name), escape(mod.pulumiTypeString(prop.Type)))

	// 	// TODO(justinvp): deprecation message

	// 	fmt.Fprintf(w, "```typescript\n")
	// 	if input {
	// 		mod.genProperty(w, prop, false, true, true, false, 0)
	// 	} else {
	// 		fmt.Fprintf(w, "public %s: pulumi.Output<%s>;\n", escape(prop.Name), escape(mod.typeString(prop.Type, false, false, !prop.IsRequired)))
	// 	}
	// 	fmt.Fprintf(w, "```\n\n")

	// 	preamble := "(Optional) "
	// 	if prop.IsRequired {
	// 		preamble = "(Required) "
	// 	}

	// 	if !input {
	// 		preamble = ""
	// 	}

	// 	fmt.Fprintf(w, "%s%s\n\n", preamble, escape(prop.Comment))
	// }
}

func (mod *modContext) genNested(w io.Writer, properties []*schema.Property, input bool) {
	tokens := stringSet{}
	mod.getTypes(properties, tokens)
	var objs []*schema.ObjectType
	for token := range tokens {
		for _, t := range mod.pkg.Types {
			if obj, ok := t.(*schema.ObjectType); ok && obj.Token == token {
				objs = append(objs, obj)
			}
		}
	}
	sort.Slice(objs, func(i, j int) bool {
		return tokenToName(objs[i].Token) < tokenToName(objs[j].Token)
	})
	for _, obj := range objs {
		fmt.Fprintf(w, "#### %s\n\n", tokenToName(obj.Token))

		mod.genProperties(w, obj.Properties, input)
	}
}

func (mod *modContext) genGet(w io.Writer, r *schema.Resource) {
	name := resourceName(r)

	stateType := name + "State"

	var stateParam string
	if r.StateInputs != nil {
		stateParam = fmt.Sprintf("state?: %s, ", stateType)
	}

	fmt.Fprintf(w, "```typescript\n")
	fmt.Fprintf(w, "public static get(name: string, id: pulumi.Input<pulumi.ID>, %sopts?: pulumi.CustomResourceOptions): %s;\n", stateParam, name)
	fmt.Fprintf(w, "```\n\n")

	fmt.Fprintf(w, "Get an existing %s resource's state with the given name, ID, and optional extra\n", name)
	fmt.Fprintf(w, "properties used to qualify the lookup.\n\n")

	fmt.Fprintf(w, "* `name` - (Required) The _unique_ name of the resulting resource.\n")
	fmt.Fprintf(w, "* `id` - (Required) The _unique_ provider ID of the resource to lookup.\n")
	fmt.Fprintf(w, "* `state` - (Optional) Any extra arguments used during the lookup.\n\n")

	// TODO(justinvp): Do we actually want to generate teh State object here? It's essentially the same
	// if r.StateInputs != nil {
	// 	fmt.Fprintf(w, "#### %s\n\n", stateType)

	// 	mod.genProperties(w, r.StateInputs.Properties, true)
	// }
}

func (mod *modContext) genResource(w io.Writer, r *schema.Resource) {
	// Create a resource module file into which all of this resource's types will go.
	name := resourceName(r)

	fmt.Fprintf(w, "%s\n\n", r.Comment)

	// TODO(justinvp): Remove this. It's just temporary to include some data we don't have here yet.
	mod.genMockupExamples(w, r)

	fmt.Fprintf(w, "## Create a %s Resource\n\n", name)

	fmt.Fprintf(w, "{{< langchoose csharp >}}\n\n")

	fmt.Fprintf(w, "```typescript\n")
	mod.genConstructor(w, r)
	fmt.Fprintf(w, "```\n\n")

	fmt.Fprintf(w, "Creates a %s resource with the given unique name, arguments, and options.\n\n", name)

	allOptionalInputs := true
	for _, prop := range r.InputProperties {
		allOptionalInputs = allOptionalInputs && !prop.IsRequired
	}

	argsRequired := "Required"
	if allOptionalInputs {
		argsRequired = "Optional"
	}

	fmt.Fprintf(w, "* `name` - (Required) The unique name of the resource.\n")
	fmt.Fprintf(w, "* `args` - (%s) The arguments to use to populate this resource's properties.\n", argsRequired)
	fmt.Fprintf(w, "* `opts` - (Optional) A bag of options that control this resource's behavior.\n\n")

	fmt.Fprintf(w, "The following arguments are supported:\n\n")

	mod.genProperties(w, r.InputProperties, true)

	fmt.Fprintf(w, "## %s Output Properties\n\n", name)

	fmt.Fprintf(w, "The following output properties are available:\n\n")

	mod.genProperties(w, r.Properties, false)

	fmt.Fprintf(w, "## Look up an Existing %s Resource\n\n", name)

	mod.genGet(w, r)

	fmt.Fprintf(w, "## Import an Existing %s Resource\n\n", name)

	// TODO(justinvp): How do we want to show import?
	fmt.Fprintf(w, "TODO\n\n")

	fmt.Fprintf(w, "## Support Types\n\n")

	mod.genNested(w, r.InputProperties, true)
}

func (mod *modContext) genFunction(w io.Writer, fun *schema.Function) {
	name := camel(tokenToName(fun.Token))

	fmt.Fprintf(w, "%s\n\n", fun.Comment)

	// =============================================================

	fmt.Fprintf(w, "```typescript\n")

	// Write the TypeDoc/JSDoc for the data source function.
	printComment(w, fun.Comment, "", "")

	if fun.DeprecationMessage != "" {
		fmt.Fprintf(w, "/** @deprecated %s */\n", fun.DeprecationMessage)
	}

	// Now, emit the function signature.
	var argsig string
	argsOptional := true
	if fun.Inputs != nil {
		for _, p := range fun.Inputs.Properties {
			if p.IsRequired {
				argsOptional = false
				break
			}
		}

		optFlag := ""
		if argsOptional {
			optFlag = "?"
		}
		argsig = fmt.Sprintf("args%s: %sArgs, ", optFlag, title(name))
	}
	var retty string
	if fun.Outputs == nil {
		retty = "void"
	} else {
		retty = title(name) + "Result"
	}
	fmt.Fprintf(w, "export function %[1]s(%[2]sopts?: pulumi.InvokeOptions): Promise<%[3]s> & %[3]s {\n", name, argsig, retty)
	if fun.DeprecationMessage != "" {
		fmt.Fprintf(w, "    pulumi.log.warn(\"%s is deprecated: %s\")\n", name, fun.DeprecationMessage)
	}

	// Zero initialize the args if empty and necessary.
	if fun.Inputs != nil && argsOptional {
		fmt.Fprintf(w, "    args = args || {};\n")
	}

	// If the caller didn't request a specific version, supply one using the version of this library.
	fmt.Fprintf(w, "    if (!opts) {\n")
	fmt.Fprintf(w, "        opts = {}\n")
	fmt.Fprintf(w, "    }\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "    if (!opts.version) {\n")
	fmt.Fprintf(w, "        opts.version = utilities.getVersion();\n")
	fmt.Fprintf(w, "    }\n")

	// Now simply invoke the runtime function with the arguments, returning the results.
	fmt.Fprintf(w, "    const promise: Promise<%s> = pulumi.runtime.invoke(\"%s\", {\n", retty, fun.Token)
	if fun.Inputs != nil {
		for _, p := range fun.Inputs.Properties {
			// Pass the argument to the invocation.
			fmt.Fprintf(w, "        \"%[1]s\": args.%[1]s,\n", p.Name)
		}
	}
	fmt.Fprintf(w, "    }, opts);\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "    return pulumi.utils.liftProperties(promise, opts);\n")
	fmt.Fprintf(w, "}\n")

	// If there are argument and/or return types, emit them.
	if fun.Inputs != nil {
		fmt.Fprintf(w, "\n")
		mod.genPlainType(w, title(name)+"Args", fun.Inputs.Comment, fun.Inputs.Properties, true, false, true, 0)
	}
	if fun.Outputs != nil {
		fmt.Fprintf(w, "\n")
		mod.genPlainType(w, title(name)+"Result", fun.Outputs.Comment, fun.Outputs.Properties, false, false, true, 0)
	}

	fmt.Fprintf(w, "```\n")
}

func visitObjectTypes(t schema.Type, visitor func(*schema.ObjectType)) {
	switch t := t.(type) {
	case *schema.ArrayType:
		visitObjectTypes(t.ElementType, visitor)
	case *schema.MapType:
		visitObjectTypes(t.ElementType, visitor)
	case *schema.ObjectType:
		for _, p := range t.Properties {
			visitObjectTypes(p.Type, visitor)
		}
		visitor(t)
	case *schema.UnionType:
		for _, e := range t.ElementTypes {
			visitObjectTypes(e, visitor)
		}
	}
}

func (mod *modContext) genType(w io.Writer, obj *schema.ObjectType, input bool, level int) {
	mod.genPlainType(w, tokenToName(obj.Token), obj.Comment, obj.Properties, input, !mod.details(obj).functionType, false, level)
}

func (mod *modContext) getTypeImports(t schema.Type, imports map[string]stringSet) bool {
	switch t := t.(type) {
	case *schema.ArrayType:
		return mod.getTypeImports(t.ElementType, imports)
	case *schema.MapType:
		return mod.getTypeImports(t.ElementType, imports)
	case *schema.ObjectType:
		return true
	case *schema.TokenType:
		modName, name, modPath := mod.pkg.TokenToModule(t.Token), tokenToName(t.Token), "./index"
		if modName != mod.mod {
			mp, err := filepath.Rel(mod.mod, modName)
			contract.Assert(err == nil)
			if path.Base(mp) == "." {
				mp = path.Dir(mp)
			}
			modPath = filepath.ToSlash(mp)
		}
		if imports[modPath] == nil {
			imports[modPath] = stringSet{}
		}
		imports[modPath].add(name)
		return false
	case *schema.UnionType:
		needsTypes := false
		for _, e := range t.ElementTypes {
			needsTypes = mod.getTypeImports(e, imports) || needsTypes
		}
		return needsTypes
	default:
		return false
	}
}

func (mod *modContext) getImports(member interface{}, imports map[string]stringSet) bool {
	switch member := member.(type) {
	case *schema.ObjectType:
		needsTypes := false
		for _, p := range member.Properties {
			needsTypes = mod.getTypeImports(p.Type, imports) || needsTypes
		}
		return needsTypes
	case *schema.Resource:
		needsTypes := false
		for _, p := range member.Properties {
			needsTypes = mod.getTypeImports(p.Type, imports) || needsTypes
		}
		for _, p := range member.InputProperties {
			needsTypes = mod.getTypeImports(p.Type, imports) || needsTypes
		}
		return needsTypes
	case *schema.Function:
		needsTypes := false
		if member.Inputs != nil {
			needsTypes = mod.getTypeImports(member.Inputs, imports) || needsTypes
		}
		if member.Outputs != nil {
			needsTypes = mod.getTypeImports(member.Outputs, imports) || needsTypes
		}
		return needsTypes
	case []*schema.Property:
		needsTypes := false
		for _, p := range member {
			needsTypes = mod.getTypeImports(p.Type, imports) || needsTypes
		}
		return needsTypes
	default:
		return false
	}
}

func (mod *modContext) getNestedTypes(t schema.Type, types stringSet) {
	switch t := t.(type) {
	case *schema.ArrayType:
		mod.getNestedTypes(t.ElementType, types)
	case *schema.MapType:
		mod.getNestedTypes(t.ElementType, types)
	case *schema.ObjectType:
		types.add(t.Token)
		mod.getTypes(t.Properties, types)
	case *schema.UnionType:
		for _, e := range t.ElementTypes {
			mod.getNestedTypes(e, types)
		}
	}
}

func (mod *modContext) getTypes(member interface{}, types stringSet) {
	switch member := member.(type) {
	case *schema.ObjectType:
		for _, p := range member.Properties {
			mod.getNestedTypes(p.Type, types)
		}
	case *schema.Resource:
		for _, p := range member.Properties {
			mod.getNestedTypes(p.Type, types)
		}
		for _, p := range member.InputProperties {
			mod.getNestedTypes(p.Type, types)
		}
	case *schema.Function:
		if member.Inputs != nil {
			mod.getNestedTypes(member.Inputs, types)
		}
		if member.Outputs != nil {
			mod.getNestedTypes(member.Outputs, types)
		}
	case []*schema.Property:
		for _, p := range member {
			mod.getNestedTypes(p.Type, types)
		}
	}
}

func (mod *modContext) genHeader(w io.Writer, title string) {
	// TODO(justinvp): generate front matter properties
	// Example:
	// title: "Package @pulumi/aws"
	// title_tag: "Package @pulumi/aws | Node.js SDK"
	// linktitle: "@pulumi/aws"
	// meta_desc: "Explore members of the @pulumi/aws package."

	fmt.Fprintf(w, "---\n")
	fmt.Fprintf(w, "title: %q\n", title)
	fmt.Fprintf(w, "---\n\n")

	fmt.Fprintf(w, "<!-- WARNING: this file was generated by %v. -->\n", mod.tool)
	fmt.Fprintf(w, "<!-- Do not edit by hand unless you're certain you know what you are doing! -->\n\n")
}

func (mod *modContext) genTypes() (string, string) {
	imports := map[string]stringSet{}
	for _, t := range mod.types {
		mod.getImports(t, imports)
	}

	inputs, outputs := &bytes.Buffer{}, &bytes.Buffer{}

	mod.genHeader(inputs, "")
	mod.genHeader(outputs, "")

	// Build a namespace tree out of the types, then emit them.

	type namespace struct {
		name     string
		types    []*schema.ObjectType
		children []*namespace
	}

	namespaces := map[string]*namespace{}
	var getNamespace func(string) *namespace
	getNamespace = func(mod string) *namespace {
		ns, ok := namespaces[mod]
		if !ok {
			name := mod
			if mod != "" {
				name = path.Base(mod)
			}

			ns = &namespace{name: name}
			if mod != "" {
				parentMod := path.Dir(mod)
				if parentMod == "." {
					parentMod = ""
				}
				parent := getNamespace(parentMod)
				parent.children = append(parent.children, ns)
			}

			namespaces[mod] = ns
		}
		return ns
	}

	for _, t := range mod.types {
		ns := getNamespace(mod.pkg.TokenToModule(t.Token))
		ns.types = append(ns.types, t)
	}

	var genNamespace func(io.Writer, *namespace, bool, int)
	genNamespace = func(w io.Writer, ns *namespace, input bool, level int) {
		indent := strings.Repeat("    ", level)

		sort.Slice(ns.types, func(i, j int) bool {
			return tokenToName(ns.types[i].Token) < tokenToName(ns.types[j].Token)
		})
		for i, t := range ns.types {
			if input && mod.details(t).inputType || !input && mod.details(t).outputType {
				mod.genType(w, t, input, level)
				if i != len(ns.types)-1 {
					fmt.Fprintf(w, "\n")
				}
			}
		}

		sort.Slice(ns.children, func(i, j int) bool {
			return ns.children[i].name < ns.children[j].name
		})
		for i, ns := range ns.children {
			fmt.Fprintf(w, "%sexport namespace %s {\n", indent, ns.name)
			genNamespace(w, ns, input, level+1)
			fmt.Fprintf(w, "%s}\n", indent)
			if i != len(ns.children)-1 {
				fmt.Fprintf(w, "\n")
			}
		}
	}
	genNamespace(inputs, namespaces[""], true, 0)
	genNamespace(outputs, namespaces[""], false, 0)

	return inputs.String(), outputs.String()
}

type fs map[string][]byte

func (fs fs) add(path string, contents []byte) {
	_, has := fs[path]
	contract.Assertf(!has, "duplicate file: %s", path)
	fs[path] = contents
}

func (mod *modContext) gen(fs fs) error {
	var files []string
	for p := range fs {
		d := path.Dir(p)
		if d == "." {
			d = ""
		}
		if d == mod.mod {
			files = append(files, p)
		}
	}

	addFile := func(name, contents string) {
		p := path.Join(mod.mod, name)
		files = append(files, p)
		fs.add(p, []byte(contents))
	}

	// Resources
	for _, r := range mod.resources {
		buffer := &bytes.Buffer{}
		mod.genHeader(buffer, resourceName(r))

		mod.genResource(buffer, r)

		addFile(lower(resourceName(r))+".md", buffer.String())
	}

	// Functions
	for _, f := range mod.functions {
		buffer := &bytes.Buffer{}
		mod.genHeader(buffer, tokenToName(f.Token))

		mod.genFunction(buffer, f)

		addFile(lower(tokenToName(f.Token))+".md", buffer.String())
	}

	// Index
	fs.add(path.Join(mod.mod, "_index.md"), []byte(mod.genIndex(files)))
	return nil
}

// genIndex emits an _index.md file for the module.
func (mod *modContext) genIndex(exports []string) string {
	w := &bytes.Buffer{}

	name := mod.mod
	if name == "" {
		name = mod.pkg.Name
	}

	mod.genHeader(w, name)

	// If this is the root module, write out the package description.
	// TODO(justinvp): The package description needs to be the equivalent of the sdk/nodejs/README.md and then the
	// other modules should use the simpler content contained in the individual sdk/nodejs/<module>/README.md.
	if mod.mod == "" {
		description := mod.pkg.Description
		if description != "" {
			description += "\n\n"
		}
		fmt.Fprintf(w, description)
	}

	// If there are submodules, list them.
	var children []string
	for _, mod := range mod.children {
		children = append(children, mod.mod)
	}
	if len(children) > 0 {
		sort.Strings(children)
		fmt.Fprintf(w, "<h3>Modules</h3>\n")
		fmt.Fprintf(w, "<ul class=\"api\">\n")
		for _, mod := range children {
			fmt.Fprintf(w, "    <li><a href=\"%s/\"><span class=\"symbol module\"></span>%s</a></li>\n", mod, mod)
		}
		fmt.Fprintf(w, "</ul>\n\n")
	}

	// If there are resources in the root, list them.
	var resources []string
	for _, r := range mod.resources {
		resources = append(resources, resourceName(r))
	}
	if len(resources) > 0 {
		sort.Strings(resources)
		fmt.Fprintf(w, "<h3>Resources</h3>\n")
		fmt.Fprintf(w, "<ul class=\"api\">\n")
		for _, r := range resources {
			fmt.Fprintf(w, "    <li><a href=\"%s\"><span class=\"symbol resource\"></span>%s</a></li>\n", lower(r), r)
		}
		fmt.Fprintf(w, "</ul>\n\n")
	}

	// If there are data sources in the root, list them.
	var functions []string
	for _, f := range mod.functions {
		functions = append(functions, tokenToName(f.Token))
	}
	if len(functions) > 0 {
		sort.Strings(functions)
		fmt.Fprintf(w, "<h3>Data Sources</h3>\n")
		fmt.Fprintf(w, "<ul class=\"api\">\n")
		for _, f := range functions {
			fmt.Fprintf(w, "    <li><a href=\"%s\"><span class=\"symbol datasource\"></span>%s</a></li>\n", lower(f), f)
		}
		fmt.Fprintf(w, "</ul>\n\n")
	}

	return w.String()
}

func GeneratePackage(tool string, pkg *schema.Package) (map[string][]byte, error) {
	// group resources, types, and functions into modules
	modules := map[string]*modContext{}

	var getMod func(token string) *modContext
	getMod = func(token string) *modContext {
		modName := pkg.TokenToModule(token)
		mod, ok := modules[modName]
		if !ok {
			mod = &modContext{
				pkg:  pkg,
				mod:  modName,
				tool: tool,
			}

			if modName != "" {
				parentName := path.Dir(modName)
				if parentName == "." || parentName == "" {
					parentName = ":index:"
				}
				parent := getMod(parentName)
				parent.children = append(parent.children, mod)
			}

			modules[modName] = mod
		}
		return mod
	}

	types := &modContext{pkg: pkg, mod: "types", tool: tool}

	// Create the config module if necessary.
	if len(pkg.Config) > 0 {
		_ = getMod(":config/config:")
	}

	for _, v := range pkg.Config {
		visitObjectTypes(v.Type, func(t *schema.ObjectType) { types.details(t).outputType = true })
	}

	scanResource := func(r *schema.Resource) {
		mod := getMod(r.Token)
		mod.resources = append(mod.resources, r)
		for _, p := range r.Properties {
			visitObjectTypes(p.Type, func(t *schema.ObjectType) { types.details(t).outputType = true })
		}
		for _, p := range r.InputProperties {
			visitObjectTypes(p.Type, func(t *schema.ObjectType) {
				if r.IsProvider {
					types.details(t).outputType = true
				}
				types.details(t).inputType = true
			})
		}
		if r.StateInputs != nil {
			visitObjectTypes(r.StateInputs, func(t *schema.ObjectType) { types.details(t).inputType = true })
		}
	}

	scanResource(pkg.Provider)
	for _, r := range pkg.Resources {
		scanResource(r)
	}

	for _, f := range pkg.Functions {
		mod := getMod(f.Token)
		mod.functions = append(mod.functions, f)
		if f.Inputs != nil {
			visitObjectTypes(f.Inputs, func(t *schema.ObjectType) {
				types.details(t).inputType = true
				types.details(t).functionType = true
			})
		}
		if f.Outputs != nil {
			visitObjectTypes(f.Outputs, func(t *schema.ObjectType) {
				types.details(t).outputType = true
				types.details(t).functionType = true
			})
		}
	}

	if _, ok := modules["types"]; ok {
		return nil, errors.New("this provider has a `types` module which is reserved for input/output types")
	}

	// Create the types module.
	for _, t := range pkg.Types {
		if obj, ok := t.(*schema.ObjectType); ok {
			types.types = append(types.types, obj)
		}
	}
	if len(types.types) > 0 {
		root := modules[""]
		root.children = append(root.children, types)
		modules["types"] = types
	}

	files := fs{}
	for _, mod := range modules {
		if err := mod.gen(files); err != nil {
			return nil, err
		}
	}

	return files, nil
}

// TODO(justinvp): Remove
func (mod *modContext) genMockupExamples(w io.Writer, r *schema.Resource) {

	if resourceName(r) != "Bucket" {
		return
	}

	fmt.Fprintf(w, "## Example Usage\n\n")

	examples := []struct {
		Heading string
		Code    string
	}{
		{
			Heading: "Private Bucket w/ Tags",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const bucket = new aws.s3.Bucket("b", {
	acl: "private",
	tags: {
		Environment: "Dev",
		Name: "My bucket",
	},
});
`,
		},
		{
			Heading: "Static Website Hosting",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";
import * as fs from "fs";

const bucket = new aws.s3.Bucket("b", {
	acl: "public-read",
	policy: fs.readFileSync("policy.json", "utf-8"),
	website: {
		errorDocument: "error.html",
		indexDocument: "index.html",
		routingRules: ` + "`" + `[{
	"Condition": {
		"KeyPrefixEquals": "docs/"
	},
	"Redirect": {
		"ReplaceKeyPrefixWith": "documents/"
	}
}]
` + "`" + `,
	},
});
`,
		},
		{
			Heading: "Using CORS",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const bucket = new aws.s3.Bucket("b", {
	acl: "public-read",
	corsRules: [{
		allowedHeaders: ["*"],
		allowedMethods: [
			"PUT",
			"POST",
		],
		allowedOrigins: ["https://s3-website-test.mydomain.com"],
		exposeHeaders: ["ETag"],
		maxAgeSeconds: 3000,
	}],
});
`,
		},
		{
			Heading: "Using versioning",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const bucket = new aws.s3.Bucket("b", {
	acl: "private",
	versioning: {
		enabled: true,
	},
});
`,
		},
		{
			Heading: "Enable Logging",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const logBucket = new aws.s3.Bucket("logBucket", {
	acl: "log-delivery-write",
});
const bucket = new aws.s3.Bucket("b", {
	acl: "private",
	loggings: [{
		targetBucket: logBucket.id,
		targetPrefix: "log/",
	}],
});
`,
		},
		{
			Heading: "Using object lifecycle",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const bucket = new aws.s3.Bucket("bucket", {
	acl: "private",
	lifecycleRules: [
		{
			enabled: true,
			expiration: {
				days: 90,
			},
			id: "log",
			prefix: "log/",
			tags: {
				autoclean: "true",
				rule: "log",
			},
			transitions: [
				{
					days: 30,
					storageClass: "STANDARD_IA", // or "ONEZONE_IA"
				},
				{
					days: 60,
					storageClass: "GLACIER",
				},
			],
		},
		{
			enabled: true,
			expiration: {
				date: "2016-01-12",
			},
			id: "tmp",
			prefix: "tmp/",
		},
	],
});
const versioningBucket = new aws.s3.Bucket("versioningBucket", {
	acl: "private",
	lifecycleRules: [{
		enabled: true,
		noncurrentVersionExpiration: {
			days: 90,
		},
		noncurrentVersionTransitions: [
			{
				days: 30,
				storageClass: "STANDARD_IA",
			},
			{
				days: 60,
				storageClass: "GLACIER",
			},
		],
		prefix: "config/",
	}],
	versioning: {
		enabled: true,
	},
});
`,
		},
		{
			Heading: "Using replication configuration",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const central = new aws.Provider("central", {
	region: "eu-central-1",
});
const replicationRole = new aws.iam.Role("replication", {
	assumeRolePolicy: ` + "`" + `{
	"Version": "2012-10-17",
	"Statement": [
	{
		"Action": "sts:AssumeRole",
		"Principal": {
		"Service": "s3.amazonaws.com"
		},
		"Effect": "Allow",
		"Sid": ""
	}
	]
}
` + "`" + `,
});
const destination = new aws.s3.Bucket("destination", {
	region: "eu-west-1",
	versioning: {
		enabled: true,
	},
});
const bucket = new aws.s3.Bucket("bucket", {
	acl: "private",
	region: "eu-central-1",
	replicationConfiguration: {
		role: replicationRole.arn,
		rules: [{
			destination: {
				bucket: destination.arn,
				storageClass: "STANDARD",
			},
			id: "foobar",
			prefix: "foo",
			status: "Enabled",
		}],
	},
	versioning: {
		enabled: true,
	},
}, {provider: central});
const replicationPolicy = new aws.iam.Policy("replication", {
	policy: pulumi.interpolate` + "`" + `{
	"Version": "2012-10-17",
	"Statement": [
	{
		"Action": [
		"s3:GetReplicationConfiguration",
		"s3:ListBucket"
		],
		"Effect": "Allow",
		"Resource": [
		"${bucket.arn}"
		]
	},
	{
		"Action": [
		"s3:GetObjectVersion",
		"s3:GetObjectVersionAcl"
		],
		"Effect": "Allow",
		"Resource": [
		"${bucket.arn}/*"
		]
	},
	{
		"Action": [
		"s3:ReplicateObject",
		"s3:ReplicateDelete"
		],
		"Effect": "Allow",
		"Resource": "${destination.arn}/*"
	}
	]
}
` + "`" + `,
});
const replicationRolePolicyAttachment = new aws.iam.RolePolicyAttachment("replication", {
	policyArn: replicationPolicy.arn,
	role: replicationRole.name,
});
`,
		},
		{
			Heading: "Enable Default Server Side Encryption",
			Code: `import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const mykey = new aws.kms.Key("mykey", {
	deletionWindowInDays: 10,
	description: "This key is used to encrypt bucket objects",
});
const mybucket = new aws.s3.Bucket("mybucket", {
	serverSideEncryptionConfiguration: {
		rule: {
			applyServerSideEncryptionByDefault: {
				kmsMasterKeyId: mykey.arn,
				sseAlgorithm: "aws:kms",
			},
		},
	},
});
`,
		},
	}

	for _, example := range examples {
		fmt.Fprintf(w, "### %s\n\n", example.Heading)

		fmt.Fprintf(w, "{{< langchoose nojavascript nogo >}}\n\n")

		fmt.Fprintf(w, "```typescript\n")
		fmt.Fprintf(w, example.Code)
		fmt.Fprintf(w, "```\n\n")

		fmt.Fprintf(w, "```python\nComing soon\n```\n\n")
	}
}
