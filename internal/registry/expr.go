package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"
	"strings"
)

// ExprGenerator evaluates an arithmetic expression over {field} references in the current row.
type ExprGenerator struct {
	expr compiledExpr
}

// Next returns an error because expr requires row context.
func (g *ExprGenerator) Next(_ *rng.Rng) (any, error) {
	return nil, fmt.Errorf("expr: requires row context")
}

// NextWithRow evaluates the expression against the current row.
func (g *ExprGenerator) NextWithRow(_ *rng.Rng, row RowContext) (any, error) {
	f, err := g.expr.eval(row)
	if err != nil {
		return nil, err
	}
	return numericResult(f), nil
}

// Dependencies returns the field names the expression references.
func (g *ExprGenerator) Dependencies() []string {
	return g.expr.deps
}

// compiledExpr is a parsed arithmetic expression plus its field bindings.
type compiledExpr struct {
	root   ast.Expr
	fields map[string]string // placeholder ident -> field name
	deps   []string          // referenced fields, in first-use order
}

// compileExpr rewrites {field} references to placeholder identifiers, parses the
// arithmetic with go/parser, and rejects anything the evaluator can't handle.
func compileExpr(src string) (compiledExpr, error) {
	rewritten, fields, deps, err := substituteFields(src)
	if err != nil {
		return compiledExpr{}, err
	}
	root, err := parser.ParseExpr(rewritten)
	if err != nil {
		return compiledExpr{}, fmt.Errorf("expr: invalid expression %q", src)
	}
	if err := checkSupported(root, fields); err != nil {
		return compiledExpr{}, err
	}
	return compiledExpr{root: root, fields: fields, deps: deps}, nil
}

// eval computes the expression's value against a row.
func (c compiledExpr) eval(row RowContext) (float64, error) {
	return evalNode(c.root, c.fields, row)
}

func evalNode(e ast.Expr, fields map[string]string, row RowContext) (float64, error) {
	switch n := e.(type) {
	case *ast.ParenExpr:
		return evalNode(n.X, fields, row)
	case *ast.UnaryExpr:
		v, err := evalNode(n.X, fields, row)
		if err != nil || n.Op != token.SUB {
			return v, err
		}
		return -v, nil
	case *ast.BinaryExpr:
		l, err := evalNode(n.X, fields, row)
		if err != nil {
			return 0, err
		}
		r, err := evalNode(n.Y, fields, row)
		if err != nil {
			return 0, err
		}
		return applyOp(n.Op, l, r)
	case *ast.BasicLit:
		return strconv.ParseFloat(n.Value, 64)
	case *ast.Ident:
		name := fields[n.Name]
		v, ok := row.Get(name)
		if !ok {
			return 0, fmt.Errorf("expr: field %q not found in row context", name)
		}
		f, ok := toNumber(v)
		if !ok {
			return 0, fmt.Errorf("expr: field %q is not numeric (got %T)", name, v)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("expr: unsupported expression")
	}
}

func applyOp(op token.Token, l, r float64) (float64, error) {
	switch op {
	case token.ADD:
		return l + r, nil
	case token.SUB:
		return l - r, nil
	case token.MUL:
		return l * r, nil
	case token.QUO:
		if r == 0 {
			return 0, fmt.Errorf("expr: division by zero")
		}
		return l / r, nil
	default:
		return 0, fmt.Errorf("expr: unsupported operator %q", op)
	}
}

// substituteFields replaces each {field} reference with a placeholder identifier
// so go/parser can handle field names that aren't valid Go identifiers (e.g.
// "type"). It returns the rewritten source, the placeholder->field map, and the
// referenced fields in first-use order.
func substituteFields(src string) (string, map[string]string, []string, error) {
	var out strings.Builder
	fields := map[string]string{}
	placeholders := map[string]string{} // field name -> placeholder
	var deps []string

	for i := 0; i < len(src); {
		if src[i] != '{' {
			out.WriteByte(src[i])
			i++
			continue
		}
		rel := strings.IndexByte(src[i:], '}')
		if rel == -1 {
			return "", nil, nil, fmt.Errorf("expr: unclosed '{' at position %d", i)
		}
		name := src[i+1 : i+rel]
		if name == "" {
			return "", nil, nil, fmt.Errorf("expr: empty field reference '{}' at position %d", i)
		}
		if strings.ContainsAny(name, "{}") {
			return "", nil, nil, fmt.Errorf("expr: nested braces at position %d", i)
		}
		ph, ok := placeholders[name]
		if !ok {
			ph = fmt.Sprintf("__f%d", len(deps))
			placeholders[name] = ph
			fields[ph] = name
			deps = append(deps, name)
		}
		out.WriteString(ph)
		i += rel + 1
	}
	return out.String(), fields, deps, nil
}

// checkSupported rejects operators and constructs the evaluator can't handle,
// and any identifier that isn't a {field} placeholder, so bad expressions fail
// at config time instead of per row.
func checkSupported(root ast.Expr, fields map[string]string) error {
	var bad error
	ast.Inspect(root, func(n ast.Node) bool {
		switch x := n.(type) {
		case nil, *ast.ParenExpr:
		case *ast.UnaryExpr:
			if x.Op != token.SUB && x.Op != token.ADD {
				bad = fmt.Errorf("expr: unsupported unary operator %q", x.Op)
			}
		case *ast.BinaryExpr:
			switch x.Op {
			case token.ADD, token.SUB, token.MUL, token.QUO:
			default:
				bad = fmt.Errorf("expr: unsupported operator %q", x.Op)
			}
		case *ast.BasicLit:
			if x.Kind != token.INT && x.Kind != token.FLOAT {
				bad = fmt.Errorf("expr: unsupported literal %s", x.Value)
			}
		case *ast.Ident:
			if _, ok := fields[x.Name]; !ok {
				bad = fmt.Errorf("expr: unknown identifier %q (reference fields as {name})", x.Name)
			}
		default:
			bad = fmt.Errorf("expr: unsupported expression syntax")
		}
		return bad == nil
	})
	return bad
}

// numericResult emits a whole number as int64, anything else as float64.
func numericResult(f float64) any {
	if !math.IsInf(f, 0) && !math.IsNaN(f) && f == math.Trunc(f) && math.Abs(f) < (1<<53) {
		return int64(f)
	}
	return f
}

// toNumber coerces a numeric row value to float64; false if not numeric.
func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func init() {
	MustRegister("expr", func(config map[string]any) (Generator, error) {
		src, ok := config["expr"].(string)
		if !ok {
			return nil, fmt.Errorf("expr: 'expr' must be a string")
		}
		expr, err := compileExpr(src)
		if err != nil {
			return nil, err
		}
		return &ExprGenerator{expr: expr}, nil
	})
	MustRegisterInfo("expr", GeneratorInfo{
		Description: "Arithmetic over {field} references and numbers (+ - * /, parentheses)",
		ConfigKeys: []ConfigKey{
			{Name: "expr", Type: "string", Required: true, Desc: "Expression, e.g. \"{total} / 12\" or \"{amount} * {fx_rate}\""},
		},
		Example: `- name: amount
  gen: expr
  config:
    expr: "{sub_total} / 12"`,
	})
}
