package model

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi/pkg/resource"
	"github.com/pulumi/pulumi/pkg/util/contract"
	"github.com/zclconf/go-cty/cty"
)

func (b *binder) bindExpression(syntax hclsyntax.Node) (Expression, hcl.Diagnostics) {
	switch syntax := syntax.(type) {
	case *hclsyntax.AnonSymbolExpr:
		return b.bindAnonSymbolExpression(syntax)
	case *hclsyntax.BinaryOpExpr:
		return b.bindBinaryOpExpression(syntax)
	case *hclsyntax.ConditionalExpr:
		return b.bindConditionalExpression(syntax)
	case *hclsyntax.ForExpr:
		return b.bindForExpression(syntax)
	case *hclsyntax.FunctionCallExpr:
		return b.bindFunctionCallExpression(syntax)
	case *hclsyntax.IndexExpr:
		return b.bindIndexExpression(syntax)
	case *hclsyntax.LiteralValueExpr:
		return b.bindLiteralValueExpression(syntax)
	case *hclsyntax.ObjectConsExpr:
		return b.bindObjectConsExpression(syntax)
	case *hclsyntax.ObjectConsKeyExpr:
		return b.bindObjectConsKeyExpr(syntax)
	case *hclsyntax.RelativeTraversalExpr:
		return b.bindRelativeTraversalExpression(syntax)
	case *hclsyntax.ScopeTraversalExpr:
		return b.bindScopeTraversalExpression(syntax)
	case *hclsyntax.SplatExpr:
		return b.bindSplatExpression(syntax)
	case *hclsyntax.TemplateExpr:
		return b.bindTemplateExpression(syntax)
	case *hclsyntax.TemplateJoinExpr:
		return b.bindTemplateJoinExpression(syntax)
	case *hclsyntax.TemplateWrapExpr:
		return b.bindTemplateWrapExpression(syntax)
	case *hclsyntax.TupleConsExpr:
		return b.bindTupleConsExpression(syntax)
	case *hclsyntax.UnaryOpExpr:
		return b.bindUnaryOpExpression(syntax)
	default:
		contract.Failf("unexpected expression node of type %T (%v)", syntax, syntax.Range())
		return nil, nil
	}
}

func ctyTypeToType(t cty.Type, optional bool) Type {
	// TODO(pdg): non-primitive types. We simply don't need these yet.
	var result Type
	switch {
	case t.Equals(cty.Bool):
		result = BoolType
	case t.Equals(cty.Number):
		result = NumberType
	case t.Equals(cty.String):
		result = StringType
	case t.Equals(cty.DynamicPseudoType):
		result = AnyType
	default:
		contract.Failf("NYI: cty type %v", t.FriendlyName())
		return nil
	}
	if optional {
		return NewOptionalType(result)
	}
	return result
}

func getOperationSignature(op *hclsyntax.Operation) FunctionSignature {
	ctyParams := op.Impl.Params()

	sig := FunctionSignature{
		Parameters: make([]Parameter, len(ctyParams)),
	}
	for i, p := range ctyParams {
		sig.Parameters[i] = Parameter{
			Name: p.Name,
			Type: inputType(ctyTypeToType(p.Type, p.AllowNull)),
		}
	}
	if p := op.Impl.VarParam(); p != nil {
		sig.VarargsParameter = &Parameter{
			Name: p.Name,
			Type: inputType(ctyTypeToType(p.Type, p.AllowNull)),
		}
	}

	sig.ReturnType = ctyTypeToType(op.Type, false)

	return sig
}

func typecheckArgs(srcRange hcl.Range, signature FunctionSignature, args ...Expression) hcl.Diagnostics {
	var diagnostics hcl.Diagnostics

	remainingArgs := args
	for _, param := range signature.Parameters {
		if len(remainingArgs) == 0 {
			if !IsOptionalType(param.Type) {
				diagnostics = append(diagnostics, missingRequiredArgument(param, srcRange))
			}
		} else {
			if !param.Type.AssignableFrom(remainingArgs[0].Type()) {
				diagnostics = append(diagnostics, exprNotAssignable(param.Type, remainingArgs[0]))
			}
			remainingArgs = remainingArgs[1:]
		}
	}

	if len(remainingArgs) > 0 {
		varargs := signature.VarargsParameter
		if varargs == nil {
			diagnostics = append(diagnostics, extraArguments(len(signature.Parameters), len(args), srcRange))
		} else {
			for _, arg := range remainingArgs {
				if !varargs.Type.AssignableFrom(arg.Type()) {
					diagnostics = append(diagnostics, exprNotAssignable(varargs.Type, arg))
				}
			}
		}
	}

	return diagnostics
}

func (b *binder) bindAnonSymbolExpression(syntax *hclsyntax.AnonSymbolExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	lv, ok := b.anonSymbols[syntax]
	if !ok {
		diagnostics = append(diagnostics, internalError(syntax.Range(), "undefined anonymous symbol"))
		return &ErrorExpression{Syntax: syntax, exprType: AnyType}, diagnostics
	}

	return &ScopeTraversalExpression{
		Syntax: &hclsyntax.ScopeTraversalExpr{
			Traversal: hcl.Traversal{hcl.TraverseRoot{Name: "<anonymous>", SrcRange: syntax.SrcRange}},
			SrcRange:  syntax.SrcRange,
		},
		Node:  lv,
		Types: []Type{lv.Type()},
	}, diagnostics
}

func (b *binder) bindBinaryOpExpression(syntax *hclsyntax.BinaryOpExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	leftOperand, leftDiags := b.bindExpression(syntax.LHS)
	diagnostics = append(diagnostics, leftDiags...)

	rightOperand, rightDiags := b.bindExpression(syntax.RHS)
	diagnostics = append(diagnostics, rightDiags...)

	signature := getOperationSignature(syntax.Op)
	contract.Assert(len(signature.Parameters) == 2)

	typecheckDiags := typecheckArgs(syntax.Range(), signature, leftOperand, rightOperand)
	diagnostics = append(diagnostics, typecheckDiags...)

	return &BinaryOpExpression{
		Syntax:       syntax,
		LeftOperand:  leftOperand,
		RightOperand: rightOperand,
		exprType:     liftOperationType(signature.ReturnType, leftOperand, rightOperand),
	}, diagnostics
}

func (b *binder) bindConditionalExpression(syntax *hclsyntax.ConditionalExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	condition, conditionDiags := b.bindExpression(syntax.Condition)
	diagnostics = append(diagnostics, conditionDiags...)

	trueResult, trueDiags := b.bindExpression(syntax.TrueResult)
	diagnostics = append(diagnostics, trueDiags...)

	falseResult, falseDiags := b.bindExpression(syntax.FalseResult)
	diagnostics = append(diagnostics, falseDiags...)

	tff := trueResult.Type().AssignableFrom(falseResult.Type())
	fft := falseResult.Type().AssignableFrom(trueResult.Type())
	var resultType Type
	switch {
	case !tff && !fft:
		resultType = NewUnionType(trueResult.Type(), falseResult.Type())
	case !tff && fft:
		resultType = falseResult.Type()
	case tff && !fft:
		resultType = trueResult.Type()
	default:
		// TODO(pdg): unify these types.
		resultType = trueResult.Type()
	}

	signature := FunctionSignature{Parameters: []Parameter{{Name: "condition", Type: inputType(BoolType)}}}
	typecheckDiags := typecheckArgs(syntax.Range(), signature, condition)
	diagnostics = append(diagnostics, typecheckDiags...)

	return &ConditionalExpression{
		Syntax:      syntax,
		Condition:   condition,
		TrueResult:  trueResult,
		FalseResult: falseResult,
		exprType:    liftOperationType(resultType, condition),
	}, diagnostics
}

func unwrapIterableSourceType(t Type) Type {
	for {
		switch tt := t.(type) {
		case *OptionalType:
			t = tt.ElementType
		case *OutputType:
			t = tt.ElementType
		case *PromiseType:
			t = tt.ElementType
		default:
			return t
		}
	}
}

func wrapIterableResultType(sourceType, iterableType Type) Type {
	for {
		switch t := sourceType.(type) {
		case *OptionalType:
			sourceType, iterableType = t.ElementType, NewOptionalType(iterableType)
		case *OutputType:
			sourceType, iterableType = t.ElementType, NewOutputType(iterableType)
		case *PromiseType:
			sourceType, iterableType = t.ElementType, NewPromiseType(iterableType)
		default:
			return iterableType
		}
	}
}

func (b *binder) bindForExpression(syntax *hclsyntax.ForExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	collection, collectionDiags := b.bindExpression(syntax.CollExpr)
	diagnostics = append(diagnostics, collectionDiags...)

	collectionType := unwrapIterableSourceType(collection.Type())

	var keyType, valueType Type
	switch collectionType := collectionType.(type) {
	case *ArrayType:
		keyType, valueType = NumberType, collectionType.ElementType
	case *MapType:
		keyType, valueType = StringType, collectionType.ElementType
	case *ObjectType:
		// TODO(pdg): might be better to make this a union type?
		keyType, valueType = StringType, AnyType
	default:
		if collectionType != AnyType {
			diagnostics = append(diagnostics, unsupportedCollectionType(collectionType, syntax.CollExpr.Range()))
		}
		keyType, valueType = AnyType, AnyType
	}

	forScope := b.scopes.push()
	if syntax.KeyVar != "" {
		ok := forScope.define(syntax.KeyVar, &LocalVariable{Name: syntax.KeyVar, VariableType: keyType})
		contract.Assert(ok)
	}
	if ok := forScope.define(syntax.ValVar, &LocalVariable{Name: syntax.ValVar, VariableType: valueType}); !ok {
		diagnostics = append(diagnostics, nameAlreadyDefined(syntax.ValVar, syntax.Range()))
	}

	var key Expression
	if syntax.KeyExpr != nil {
		keyExpr, keyDiags := b.bindExpression(syntax.KeyExpr)
		key, diagnostics = keyExpr, append(diagnostics, keyDiags...)

		// A key expression is only present when producing a map. Key types must therefore be strings.
		if !inputType(StringType).AssignableFrom(key.Type()) {
			diagnostics = append(diagnostics, exprNotAssignable(inputType(StringType), key))
		}
	}

	value, valueDiags := b.bindExpression(syntax.ValExpr)
	diagnostics = append(diagnostics, valueDiags...)

	var condition Expression
	if syntax.CondExpr != nil {
		condExpr, conditionDiags := b.bindExpression(syntax.CondExpr)
		condition, diagnostics = condExpr, append(diagnostics, conditionDiags...)

		if !inputType(BoolType).AssignableFrom(condition.Type()) {
			diagnostics = append(diagnostics, exprNotAssignable(inputType(BoolType), condition))
		}
	}

	var resultType Type
	if key != nil {
		if syntax.Group {
			valueType = NewArrayType(value.Type())
		}
		resultType = wrapIterableResultType(collectionType, NewMapType(value.Type()))
	} else {
		resultType = wrapIterableResultType(collectionType, NewArrayType(value.Type()))
	}

	var liftArgs []Expression
	if key != nil {
		liftArgs = append(liftArgs, key)
	}
	if condition != nil {
		liftArgs = append(liftArgs, condition)
	}

	return &ForExpression{
		Syntax:     syntax,
		Collection: collection,
		Key:        key,
		Value:      value,
		exprType:   liftOperationType(resultType, liftArgs...),
	}, diagnostics
}

func (b *binder) bindFunctionCallExpression(syntax *hclsyntax.FunctionCallExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	definition, definitionDiags := getFunctionDefinition(syntax.Name, syntax.NameRange)
	diagnostics = append(diagnostics, definitionDiags...)

	args := make([]Expression, len(syntax.Args))
	for i, syntax := range syntax.Args {
		arg, argDiagnostics := b.bindExpression(syntax)
		args[i], diagnostics = arg, append(diagnostics, argDiagnostics...)
	}

	if definition == nil {
		return &FunctionCallExpression{
			Syntax: syntax,
			Name:   syntax.Name,
			Signature: FunctionSignature{
				VarargsParameter: &Parameter{Name: "args", Type: AnyType},
				ReturnType:       AnyType,
			},
			Args: args,
		}, diagnostics
	}

	signature, sigDiags := definition.signature(args)
	diagnostics = append(diagnostics, sigDiags...)

	for i := range signature.Parameters {
		signature.Parameters[i].Type = inputType(signature.Parameters[i].Type)
	}
	if signature.VarargsParameter != nil {
		signature.VarargsParameter.Type = inputType(signature.VarargsParameter.Type)
	}

	typecheckDiags := typecheckArgs(syntax.Range(), signature, args...)
	diagnostics = append(diagnostics, typecheckDiags...)

	signature.ReturnType = liftOperationType(signature.ReturnType, args...)

	return &FunctionCallExpression{
		Syntax:    syntax,
		Name:      syntax.Name,
		Signature: signature,
		Args:      args,
	}, diagnostics
}

func (b *binder) bindIndexExpression(syntax *hclsyntax.IndexExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	collection, collectionDiags := b.bindExpression(syntax.Collection)
	diagnostics = append(diagnostics, collectionDiags...)

	key, keyDiags := b.bindExpression(syntax.Key)
	diagnostics = append(diagnostics, keyDiags...)

	typ, typDiags := b.bindIndexType(collection.Type(), key.Type(), cty.DynamicVal, syntax.Key.Range())
	diagnostics = append(diagnostics, typDiags...)

	return &IndexExpression{
		Syntax:     syntax,
		Collection: collection,
		Key:        key,
		exprType:   liftOperationType(typ, collection, key),
	}, diagnostics
}

func (b *binder) bindLiteralValueExpression(syntax *hclsyntax.LiteralValueExpr) (Expression, hcl.Diagnostics) {
	pv, typ, diagnostics := resource.PropertyValue{}, Type(nil), hcl.Diagnostics(nil)

	v := syntax.Val
	switch {
	case v.IsNull():
		// OK
	case v.Type() == cty.Bool:
		pv, typ = resource.NewBoolProperty(v.True()), BoolType
	case v.Type() == cty.Number:
		f, _ := v.AsBigFloat().Float64()
		pv, typ = resource.NewNumberProperty(f), NumberType
	case v.Type() == cty.String:
		pv, typ = resource.NewStringProperty(v.AsString()), StringType
	default:
		typ, diagnostics = AnyType, hcl.Diagnostics{unsupportedLiteralValue(syntax)}
	}

	return &LiteralValueExpression{
		Syntax:   syntax,
		Value:    pv,
		exprType: typ,
	}, diagnostics
}

func (b *binder) bindObjectConsExpression(syntax *hclsyntax.ObjectConsExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	items := make([]ObjectConsItem, len(syntax.Items))
	for i, item := range syntax.Items {
		keyExpr, keyDiags := b.bindExpression(item.KeyExpr)
		diagnostics = append(diagnostics, keyDiags...)

		if !StringType.AssignableFrom(keyExpr.Type()) {
			// TODO(pdg): this does not match the default HCL2 evaluation semantics.
			diagnostics = append(diagnostics, objectKeysMustBeStrings(keyExpr))
		}

		valExpr, valDiags := b.bindExpression(item.ValueExpr)
		diagnostics = append(diagnostics, valDiags...)

		items[i] = ObjectConsItem{Key: keyExpr, Value: valExpr}
	}

	// Attempt to build an object type out of the result. If there are any attribute names that come from variables,
	// type the result as Any.
	//
	// TODO(pdg): can we refine this?
	// TODO(pdg): proper typing w.r.t. eventual keys
	properties, isAnyType, typ := map[string]Type{}, false, Type(nil)
	for _, item := range items {
		keyLit, ok := item.Key.(*LiteralValueExpression)
		if !ok || !keyLit.Value.IsString() {
			isAnyType, typ = true, AnyType
			break
		}
		properties[keyLit.Value.StringValue()] = item.Value.Type()
	}
	if !isAnyType {
		typ = NewObjectType(properties)
	}

	return &ObjectConsExpression{
		Syntax:   syntax,
		Items:    items,
		exprType: typ,
	}, diagnostics
}

func (b *binder) bindObjectConsKeyExpr(syntax *hclsyntax.ObjectConsKeyExpr) (Expression, hcl.Diagnostics) {
	if !syntax.ForceNonLiteral {
		if name := hcl.ExprAsKeyword(syntax); name != "" {
			return b.bindExpression(&hclsyntax.LiteralValueExpr{
				Val:      cty.StringVal(name),
				SrcRange: syntax.Range(),
			})
		}
	}
	return b.bindExpression(syntax.Wrapped)
}

func (b *binder) bindRelativeTraversalExpression(syntax *hclsyntax.RelativeTraversalExpr) (Expression, hcl.Diagnostics) {
	source, diagnostics := b.bindExpression(syntax.Source)

	types, typDiags := b.bindTraversalTypes(source.Type(), syntax.Traversal)
	diagnostics = append(diagnostics, typDiags...)

	return &RelativeTraversalExpression{
		Syntax: syntax,
		Source: source,
		Types:  types,
	}, diagnostics
}

func (b *binder) bindScopeTraversalExpression(syntax *hclsyntax.ScopeTraversalExpr) (Expression, hcl.Diagnostics) {
	node, ok := b.scopes.bindReference(syntax.Traversal.RootName())
	if !ok {
		types := make([]Type, len(syntax.Traversal))
		for i := range types {
			types[i] = AnyType
		}
		return &ScopeTraversalExpression{
			Syntax: syntax,
			Types:  types,
		}, hcl.Diagnostics{undefinedVariable(syntax.Traversal.SimpleSplit().Abs.SourceRange())}
	}

	types, diagnostics := b.bindTraversalTypes(node.Type(), syntax.Traversal.SimpleSplit().Rel)
	return &ScopeTraversalExpression{
		Syntax: syntax,
		Node:   node,
		Types:  types,
	}, diagnostics
}

func (b *binder) bindSplatExpression(syntax *hclsyntax.SplatExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	source, sourceDiags := b.bindExpression(syntax.Source)
	diagnostics = append(diagnostics, sourceDiags...)

	sourceType := unwrapIterableSourceType(source.Type())
	elementType := sourceType
	if arr, isArray := sourceType.(*ArrayType); isArray {
		elementType = arr.ElementType
	} else if sourceType != AnyType {
		source = &TupleConsExpression{
			Syntax: &hclsyntax.TupleConsExpr{
				Exprs:     []hclsyntax.Expression{syntax.Source},
				SrcRange:  syntax.Source.Range(),
				OpenRange: syntax.Source.StartRange(),
			},
			Expressions: []Expression{source},
			exprType:    NewArrayType(source.Type()),
		}
	}

	item := &LocalVariable{
		Name:         "<anonymous>",
		VariableType: elementType,
	}
	b.anonSymbols[syntax.Item] = item

	each, eachDiags := b.bindExpression(syntax.Each)
	diagnostics = append(diagnostics, eachDiags...)

	return &SplatExpression{
		Syntax:   syntax,
		Source:   source,
		Each:     each,
		Item:     item,
		exprType: wrapIterableResultType(source.Type(), NewArrayType(each.Type())),
	}, diagnostics
}

func (b *binder) bindTemplateExpression(syntax *hclsyntax.TemplateExpr) (Expression, hcl.Diagnostics) {
	if syntax.IsStringLiteral() {
		return b.bindExpression(syntax.Parts[0])
	}

	var diagnostics hcl.Diagnostics
	parts := make([]Expression, len(syntax.Parts))
	for i, syntax := range syntax.Parts {
		part, partDiags := b.bindExpression(syntax)
		parts[i], diagnostics = part, append(diagnostics, partDiags...)
	}

	return &TemplateExpression{
		Syntax:   syntax,
		Parts:    parts,
		exprType: liftOperationType(StringType, parts...),
	}, diagnostics
}

func (b *binder) bindTemplateJoinExpression(syntax *hclsyntax.TemplateJoinExpr) (Expression, hcl.Diagnostics) {
	tuple, diagnostics := b.bindExpression(syntax.Tuple)

	return &TemplateJoinExpression{
		Syntax:   syntax,
		Tuple:    tuple,
		exprType: liftOperationType(StringType, tuple),
	}, diagnostics
}

func (b *binder) bindTemplateWrapExpression(syntax *hclsyntax.TemplateWrapExpr) (Expression, hcl.Diagnostics) {
	return b.bindExpression(syntax.Wrapped)
}

func (b *binder) bindTupleConsExpression(syntax *hclsyntax.TupleConsExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics
	exprs := make([]Expression, len(syntax.Exprs))
	for i, syntax := range syntax.Exprs {
		expr, exprDiags := b.bindExpression(syntax)
		exprs[i], diagnostics = expr, append(diagnostics, exprDiags...)
	}

	// TODO(pdg): better typing. Need an algorithm for finding the best type.
	var typ Type
	for _, expr := range exprs {
		if typ == nil {
			typ = expr.Type()
		} else if expr.Type() != typ {
			typ = AnyType
			break
		}
	}

	return &TupleConsExpression{
		Syntax:      syntax,
		Expressions: exprs,
		exprType:    NewArrayType(typ),
	}, diagnostics
}

func (b *binder) bindUnaryOpExpression(syntax *hclsyntax.UnaryOpExpr) (Expression, hcl.Diagnostics) {
	var diagnostics hcl.Diagnostics

	operand, operandDiags := b.bindExpression(syntax.Val)
	diagnostics = append(diagnostics, operandDiags...)

	signature := getOperationSignature(syntax.Op)
	contract.Assert(len(signature.Parameters) == 1)

	typecheckDiags := typecheckArgs(syntax.Range(), signature, operand)
	diagnostics = append(diagnostics, typecheckDiags...)

	return &UnaryOpExpression{
		Syntax:   syntax,
		Operand:  operand,
		exprType: liftOperationType(signature.ReturnType, operand),
	}, diagnostics
}