package model_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The compiler cannot catch two constants in a typed group sharing a value. That is
// how AggregateReviewStateNone sat on the zero value alongside the unknown sentinel
// until STE-100 — every call site read "no reviews" for PRs that had never been
// fetched, and every test passed. These tests read the package's own source so a new
// enum, or a new member of an existing one, is covered without anyone remembering
// to add it to a list.

// constantMember is one declared constant, kept with its position for error messages.
type constantMember struct {
	name     string
	value    string // canonical value: the unquoted string, or the decimal for integers
	kind     valueKind
	position token.Position
}

type valueKind int

const (
	kindString valueKind = iota
	kindInt
)

func (kind valueKind) zeroValue() string {
	if kind == kindString {
		return ""
	}
	return "0"
}

func (kind valueKind) describe(value string) string {
	if kind == kindString {
		return strconv.Quote(value)
	}
	return value
}

func TestEnumMembersArePairwiseDistinct(t *testing.T) {
	groups := parseTypedConstantGroups(t)

	for _, typeName := range sortedTypeNames(groups) {
		valueOwner := make(map[string]constantMember, len(groups[typeName]))

		for _, member := range groups[typeName] {
			if previous, duplicate := valueOwner[member.value]; duplicate {
				t.Errorf(
					"%s: %s and %s both equal %s\n\t%s\n\t%s\n\ttwo members sharing a value make them the same state; give each real answer its own value",
					typeName, previous.name, member.name, member.kind.describe(member.value),
					previous.position, member.position,
				)
				continue
			}
			valueOwner[member.value] = member
		}
	}
}

// TestEnumsNameTheirZeroValue enforces the other half of the convention: the zero
// value always has a name, so a composite literal that never assigns the field reads
// as unknown rather than silently claiming whichever answer landed at zero.
func TestEnumsNameTheirZeroValue(t *testing.T) {
	groups := parseTypedConstantGroups(t)

	for _, typeName := range sortedTypeNames(groups) {
		members := groups[typeName]

		named := false
		for _, member := range members {
			if member.value == member.kind.zeroValue() {
				named = true
				break
			}
		}

		if !named {
			t.Errorf(
				"%s: no member sits at the zero value %s\n\t%s\n\tdeclare one (conventionally %sUnknown) so an unassigned field is not mistaken for a real answer",
				typeName, members[0].kind.describe(members[0].kind.zeroValue()),
				members[0].position, typeName,
			)
		}
	}
}

// parseTypedConstantGroups reads the package's non-test sources and returns every
// constant that was declared with a named type from this package, keyed by type name.
func parseTypedConstantGroups(t *testing.T) map[string][]constantMember {
	t.Helper()

	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", func(fileInfo fs.FileInfo) bool {
		return !strings.HasSuffix(fileInfo.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package source: %v", err)
	}

	groups := make(map[string][]constantMember)
	for _, parsedPackage := range packages {
		for _, file := range parsedPackage.Files {
			for _, declaration := range file.Decls {
				genDecl, isGenDecl := declaration.(*ast.GenDecl)
				if !isGenDecl || genDecl.Tok != token.CONST {
					continue
				}
				collectConstantGroup(t, fileSet, genDecl, groups)
			}
		}
	}

	if len(groups) == 0 {
		t.Fatal("found no typed constant groups; this test would silently cover nothing")
	}
	return groups
}

// collectConstantGroup walks one `const (...)` block, resolving the Go rule that a
// spec with no value repeats the previous spec's type and expression with iota
// advanced — which is how LoadState and any future iota enum are declared.
func collectConstantGroup(
	t *testing.T,
	fileSet *token.FileSet,
	genDecl *ast.GenDecl,
	groups map[string][]constantMember,
) {
	t.Helper()

	var inheritedType ast.Expr
	var inheritedValues []ast.Expr

	for iotaIndex, spec := range genDecl.Specs {
		valueSpec, isValueSpec := spec.(*ast.ValueSpec)
		if !isValueSpec {
			continue
		}

		specType, specValues := valueSpec.Type, valueSpec.Values
		if len(specValues) == 0 {
			specType, specValues = inheritedType, inheritedValues
		} else {
			inheritedType, inheritedValues = specType, specValues
		}

		typeIdent, isNamedType := specType.(*ast.Ident)
		if !isNamedType {
			continue // untyped constant, or a qualified type from another package
		}

		for nameIndex, name := range valueSpec.Names {
			if name.Name == "_" || nameIndex >= len(specValues) {
				continue
			}

			value, kind, ok := evaluateConstantExpr(specValues[nameIndex], iotaIndex)
			if !ok {
				// Failing here rather than skipping is the point: an unevaluated
				// member is an unchecked member, and silence is what let the
				// original collision survive.
				t.Errorf(
					"%s: cannot evaluate the value of %s at %s\n\textend evaluateConstantExpr to cover this declaration form",
					typeIdent.Name, name.Name, fileSet.Position(name.Pos()),
				)
				continue
			}

			groups[typeIdent.Name] = append(groups[typeIdent.Name], constantMember{
				name:     name.Name,
				value:    value,
				kind:     kind,
				position: fileSet.Position(name.Pos()),
			})
		}
	}
}

// evaluateConstantExpr handles the declaration forms this package actually uses:
// string literals, integer literals, and bare iota. Anything else returns false so
// the caller can fail rather than quietly drop the member.
func evaluateConstantExpr(expr ast.Expr, iotaIndex int) (string, valueKind, bool) {
	switch typedExpr := expr.(type) {
	case *ast.BasicLit:
		switch typedExpr.Kind {
		case token.STRING:
			unquoted, err := strconv.Unquote(typedExpr.Value)
			if err != nil {
				return "", kindString, false
			}
			return unquoted, kindString, true
		case token.INT:
			return typedExpr.Value, kindInt, true
		}
	case *ast.Ident:
		if typedExpr.Name == "iota" {
			return strconv.Itoa(iotaIndex), kindInt, true
		}
	}
	return "", kindString, false
}

func sortedTypeNames(groups map[string][]constantMember) []string {
	typeNames := make([]string, 0, len(groups))
	for typeName := range groups {
		typeNames = append(typeNames, typeName)
	}
	sort.Strings(typeNames)
	return typeNames
}
