package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	found := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipDir(d.Name()) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			found += report(path)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if found > 0 {
		os.Exit(1)
	}
}

func skipDir(name string) bool {
	return name != "." && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata")
}

func report(path string) int {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	found := 0
	for _, group := range file.Comments {
		for _, c := range group.List {
			if !strings.HasPrefix(c.Text, "//go:") {
				fmt.Printf("%s: comment not allowed\n", fset.Position(c.Pos()))
				found++
			}
		}
	}
	return found
}
