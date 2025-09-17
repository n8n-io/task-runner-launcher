package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	fset := token.NewFileSet()
	issues := 0

	err = filepath.WalkDir(wd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip bazel-* directories and test files for now
		if d.IsDir() && strings.HasPrefix(d.Name(), "bazel-") {
			return fs.SkipDir
		}

		// Only process .go files (but skip test files for basic checks)
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileIssues, err := lintGoFile(fset, path)
		if err != nil {
			return err
		}
		issues += fileIssues

		return nil
	})

	if err != nil {
		return err
	}

	if issues > 0 {
		fmt.Printf("\nFound %d issues total\n", issues)
		os.Exit(1)
	}

	fmt.Println("No issues found")
	return nil
}

func lintGoFile(fset *token.FileSet, filename string) (int, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", filename, err)
	}

	// Parse the Go file
	file, err := parser.ParseFile(fset, filename, content, parser.ParseComments)
	if err != nil {
		fmt.Printf("Warning: Could not parse %s: %v\n", filename, err)
		return 0, nil
	}

	issues := 0

	// Basic lint checks
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			// Check for exported functions without comments
			if node.Name.IsExported() && node.Doc == nil {
				pos := fset.Position(node.Pos())
				fmt.Printf("%s:%d:%d: exported function %s should have comment\n", 
					pos.Filename, pos.Line, pos.Column, node.Name.Name)
				issues++
			}
		case *ast.TypeSpec:
			// Check for exported types without comments
			if node.Name.IsExported() && node.Doc == nil {
				pos := fset.Position(node.Pos())
				fmt.Printf("%s:%d:%d: exported type %s should have comment\n", 
					pos.Filename, pos.Line, pos.Column, node.Name.Name)
				issues++
			}
		}
		return true
	})

	return issues, nil
}