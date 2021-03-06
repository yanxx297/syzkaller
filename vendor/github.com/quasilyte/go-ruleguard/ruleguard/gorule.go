package ruleguard

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/quasilyte/go-ruleguard/internal/mvdan.cc/gogrep"
	"github.com/quasilyte/go-ruleguard/ruleguard/quasigo"
)

type goRuleSet struct {
	universal *scopedGoRuleSet

	groups map[string]token.Position // To handle redefinitions
}

type scopedGoRuleSet struct {
	uncategorized   []goRule
	categorizedNum  int
	rulesByCategory [nodeCategoriesCount][]goRule
}

type goRule struct {
	group      string
	filename   string
	line       int
	pat        *gogrep.Pattern
	msg        string
	location   string
	suggestion string
	filter     matchFilter
}

type matchFilterResult string

func (s matchFilterResult) Matched() bool { return s == "" }

func (s matchFilterResult) RejectReason() string { return string(s) }

type filterFunc func(*filterParams) matchFilterResult

type matchFilter struct {
	src string
	fn  func(*filterParams) matchFilterResult
}

type filterParams struct {
	ctx      *RunContext
	filename string
	imports  map[string]struct{}
	env      *quasigo.EvalEnv

	importer *goImporter

	values map[string]ast.Node

	nodeText func(n ast.Node) []byte

	// varname is set only for custom filters before bytecode function is called.
	varname string
}

func (params *filterParams) subExpr(name string) ast.Expr {
	switch n := params.values[name].(type) {
	case ast.Expr:
		return n
	case *ast.ExprStmt:
		return n.X
	default:
		return nil
	}
}

func (params *filterParams) typeofNode(n ast.Node) types.Type {
	if e, ok := n.(ast.Expr); ok {
		if typ := params.ctx.Types.TypeOf(e); typ != nil {
			return typ
		}
	}

	return types.Typ[types.Invalid]
}

func cloneRuleSet(rset *goRuleSet) *goRuleSet {
	out, err := mergeRuleSets([]*goRuleSet{rset})
	if err != nil {
		panic(err) // Should never happen
	}
	return out
}

func mergeRuleSets(toMerge []*goRuleSet) (*goRuleSet, error) {
	out := &goRuleSet{
		universal: &scopedGoRuleSet{},
		groups:    make(map[string]token.Position),
	}

	for _, x := range toMerge {
		out.universal = appendScopedRuleSet(out.universal, x.universal)
		for group, pos := range x.groups {
			if prevPos, ok := out.groups[group]; ok {
				newRef := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
				oldRef := fmt.Sprintf("%s:%d", prevPos.Filename, prevPos.Line)
				return nil, fmt.Errorf("%s: redefenition of %s(), previously defined at %s", newRef, group, oldRef)
			}
			out.groups[group] = pos
		}
	}

	return out, nil
}

func appendScopedRuleSet(dst, src *scopedGoRuleSet) *scopedGoRuleSet {
	dst.uncategorized = append(dst.uncategorized, cloneRuleSlice(src.uncategorized)...)
	for cat, rules := range src.rulesByCategory {
		dst.rulesByCategory[cat] = append(dst.rulesByCategory[cat], cloneRuleSlice(rules)...)
		dst.categorizedNum += len(rules)
	}
	return dst
}

func cloneRuleSlice(slice []goRule) []goRule {
	out := make([]goRule, len(slice))
	for i, rule := range slice {
		clone := rule
		clone.pat = rule.pat.Clone()
		out[i] = clone
	}
	return out
}
