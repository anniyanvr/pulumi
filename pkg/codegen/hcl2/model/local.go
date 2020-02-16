package model

import (
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type LocalVariable struct {
	Syntax *hclsyntax.Attribute

	Type  Type
	Value Expression

	state bindState
}

func (lv *LocalVariable) SyntaxNode() hclsyntax.Node {
	return lv.Syntax
}

func (lv *LocalVariable) getState() bindState {
	return lv.state
}

func (lv *LocalVariable) setState(s bindState) {
	lv.state = s
}

func (*LocalVariable) isNode() {}
