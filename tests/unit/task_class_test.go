package unit_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/policy"
)

// declaredTaskClasses reads the string of every constant internal/policy declares
// with the type TaskClass, in declaration order. A task class is a defined
// string, so no compiler lists its members; this is the list the source holds.
func declaredTaskClasses(t *testing.T) []policy.TaskClass {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", "internal", "policy", "profiles.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var declared []policy.TaskClass
	for _, declaration := range parsed.Decls {
		group, isGroup := declaration.(*ast.GenDecl)
		if !isGroup || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || value.Type == nil || len(value.Values) != 1 {
				continue
			}
			if name, named := value.Type.(*ast.Ident); !named || name.Name != "TaskClass" {
				continue
			}
			literal, isLiteral := value.Values[0].(*ast.BasicLit)
			if !isLiteral || literal.Kind != token.STRING {
				t.Fatalf("task class %s is not a string literal", value.Names[0].Name)
			}
			text, unquoteError := strconv.Unquote(literal.Value)
			if unquoteError != nil {
				t.Fatal(unquoteError)
			}
			declared = append(declared, policy.TaskClass(text))
		}
	}

	return declared
}

// TestTaskClassesListsEveryDeclaredClass holds the one member list to the
// constants beside it. The list is hand-kept because a defined string has no
// iota to derive it from, so a class declared and not listed would be refused at
// decode and absent from the history filter; this test is what fails instead.
func TestTaskClassesListsEveryDeclaredClass(t *testing.T) {
	declared := declaredTaskClasses(t)
	if len(declared) == 0 {
		t.Fatal("internal/policy declares no task class")
	}
	listed := policy.TaskClasses()
	if !slices.Equal(listed, declared) {
		t.Errorf("TaskClasses lists %v, and internal/policy declares %v", listed, declared)
	}
	// The order the history filter lists them in, which docs/reference/cli.md publishes.
	if want := []policy.TaskClass{"ephemeral", "service", "transactional", "release"}; !slices.Equal(listed, want) {
		t.Errorf("TaskClasses lists %v, want %v", listed, want)
	}
}

func TestTaskClassDecodesOnlyItsMembers(t *testing.T) {
	for _, class := range policy.TaskClasses() {
		var decoded policy.TaskClass
		if err := json.Unmarshal([]byte(strconv.Quote(string(class))), &decoded); err != nil || decoded != class {
			t.Errorf("%q decodes as %q (%v), want the member", class, decoded, err)
		}
	}
	for _, text := range []string{"", "batch", "unknown", "Ephemeral", " service", "release ", "transactional\n"} {
		decoded := policy.TaskService
		err := json.Unmarshal([]byte(strconv.Quote(text)), &decoded)
		if err == nil || decoded != policy.TaskService || !strings.Contains(err.Error(), strconv.Quote(text)) {
			t.Errorf("%q decodes as %q (%v), want a refusal naming it that leaves the class as it was", text, decoded, err)
		}
	}
}

// Every text a class can be read from, the members and the words that are not.
var classTexts = []string{"ephemeral", "service", "transactional", "release", "", "batch", "Ephemeral", " service", "release "}

// TestStrictAndTolerantReadersAgreeOnWhatIsAMember holds the two readers to one
// membership rule: the strict parse accepts exactly the texts the tolerant
// reader reads with a member, and the tolerant reader keeps every text as it was.
func TestStrictAndTolerantReadersAgreeOnWhatIsAMember(t *testing.T) {
	for _, text := range classTexts {
		parsed, parseError := policy.ParseTaskClass(text)
		var recorded policy.RecordedTaskClass
		if err := recorded.UnmarshalText([]byte(text)); err != nil {
			t.Errorf("the tolerant reader refused %q: %v", text, err)
		}
		member, known := recorded.TaskClass()
		if (parseError == nil) != known || parsed != member {
			t.Errorf("%q: strict read %q (%v), tolerant read %q (known=%t)", text, parsed, parseError, member, known)
		}
		if recorded.String() != text || recorded.IsZero() != (text == "") {
			t.Errorf("%q: the tolerant reader kept %q (zero=%t)", text, recorded.String(), recorded.IsZero())
		}
		if parseError != nil && !strings.Contains(parseError.Error(), strconv.Quote(text)) {
			t.Errorf("the refusal of %q does not name it: %v", text, parseError)
		}
		if policy.RecordedClass(policy.TaskClass(text)) != recorded {
			t.Errorf("%q: RecordedClass and the tolerant reader disagree", text)
		}
	}
}

func TestRecordedTaskClassKeepsItsTextThroughJSON(t *testing.T) {
	type row struct {
		Class policy.RecordedTaskClass `json:"class,omitzero"`
	}
	for _, text := range classTexts {
		var decoded row
		if err := json.Unmarshal([]byte(`{"class":`+strconv.Quote(text)+`}`), &decoded); err != nil {
			t.Errorf("%q was refused by a reader of recorded evidence: %v", text, err)

			continue
		}
		encoded, err := json.Marshal(decoded)
		want := `{"class":` + strconv.Quote(text) + `}`
		if text == "" {
			want = "{}"
		}
		if err != nil || string(encoded) != want {
			t.Errorf("%q encodes as %s (%v), want %s", text, encoded, err, want)
		}
	}
	var absent row
	if err := json.Unmarshal([]byte(`{"class":null}`), &absent); err != nil || !absent.Class.IsZero() {
		t.Errorf("a null class reads as %q (%v), want nothing recorded", absent.Class.String(), err)
	}
}
