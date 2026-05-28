package cli

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCodebergNewClientCallsitesUseTokenThenOrg(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	cliDir := filepath.Dir(thisFile)

	// Keep this list explicit so regressions in known call-site files fail loudly.
	targetFiles := []string{
		"forge_client.go",
		"description_sync.go",
		"showcase_only_handler.go",
		"sync_handlers.go",
	}

	fset := token.NewFileSet()
	totalCalls := 0

	for _, name := range targetFiles {
		path := filepath.Join(cliDir, name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "codeberg" || sel.Sel.Name != "NewClient" {
				return true
			}

			totalCalls++
			if len(call.Args) != 2 {
				t.Errorf("%s: codeberg.NewClient arg count = %d, want 2", name, len(call.Args))
				return true
			}

			firstField := selectorField(call.Args[0])
			secondField := selectorField(call.Args[1])
			if firstField != "CodebergToken" || secondField != "Name" {
				t.Errorf(
					"%s: codeberg.NewClient arg order mismatch: got (%s, %s), want (CodebergToken, Name)",
					name,
					exprString(fset, call.Args[0]),
					exprString(fset, call.Args[1]),
				)
			}

			return true
		})
	}

	if totalCalls == 0 {
		t.Fatal("expected at least one codeberg.NewClient call in CLI call-site files")
	}
}

func selectorField(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if sel.Sel == nil {
		return ""
	}
	return sel.Sel.Name
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, expr); err != nil {
		return "<print error>"
	}
	return b.String()
}
