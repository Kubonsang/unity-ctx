package document

import (
	"errors"
	"path/filepath"
	"testing"

	"unity-ctx/internal/parser"
)

func TestBuildFindByFileIDReturnsMatchingBlock(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	doc := Build(blocks)

	block, ok := doc.FindByFileID(2000)
	if !ok {
		t.Fatal("FindByFileID(2000) found = false, want true")
	}

	if block.TypeName != "GameObject" {
		t.Fatalf("TypeName mismatch: got %q want %q", block.TypeName, "GameObject")
	}

	name, ok := block.Fields["m_Name"].(string)
	if !ok {
		t.Fatalf("m_Name type mismatch: got %T want string", block.Fields["m_Name"])
	}

	if name != "Chair_01" {
		t.Fatalf("m_Name mismatch: got %q want %q", name, "Chair_01")
	}
}

func TestFindUniqueByNameReturnsAmbiguousNameError(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "duplicate_names_scene.unity")

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	doc := Build(blocks)

	_, err = doc.FindUniqueByName("Enemy")
	if err == nil {
		t.Fatal("FindUniqueByName(\"Enemy\") error = nil, want ambiguous name error")
	}

	var lookupErr *LookupError
	if !errors.As(err, &lookupErr) {
		t.Fatalf("error type mismatch: got %T want *LookupError", err)
	}

	if lookupErr.Code != CodeAmbiguousName {
		t.Fatalf("Code mismatch: got %q want %q", lookupErr.Code, CodeAmbiguousName)
	}
}

func TestFindUniqueByNameReturnsNotFoundError(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	doc := Build(blocks)

	_, err = doc.FindUniqueByName("Missing")
	if err == nil {
		t.Fatal("FindUniqueByName(\"Missing\") error = nil, want not found error")
	}

	var lookupErr *LookupError
	if !errors.As(err, &lookupErr) {
		t.Fatalf("error type mismatch: got %T want *LookupError", err)
	}

	if lookupErr.Code != CodeNotFound {
		t.Fatalf("Code mismatch: got %q want %q", lookupErr.Code, CodeNotFound)
	}
}

func TestBuildClonesFieldsFromCallerBlocks(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	doc := Build(blocks)

	blocks[2].Fields["m_Name"] = "MutatedOutsideDoc"
	position := blocks[3].Fields["m_LocalPosition"].(map[string]any)
	position["x"] = int64(999)

	block, ok := doc.FindByFileID(2000)
	if !ok {
		t.Fatal("FindByFileID(2000) found = false, want true")
	}

	name, ok := block.Fields["m_Name"].(string)
	if !ok {
		t.Fatalf("m_Name type mismatch: got %T want string", block.Fields["m_Name"])
	}

	if name != "Chair_01" {
		t.Fatalf("m_Name mismatch after caller mutation: got %q want %q", name, "Chair_01")
	}

	transform, ok := doc.FindByFileID(2001)
	if !ok {
		t.Fatal("FindByFileID(2001) found = false, want true")
	}

	localPosition, ok := transform.Fields["m_LocalPosition"].(map[string]any)
	if !ok {
		t.Fatalf("m_LocalPosition type mismatch: got %T want map[string]any", transform.Fields["m_LocalPosition"])
	}

	if localPosition["x"] != 2.1 {
		t.Fatalf("m_LocalPosition.x mismatch after caller mutation: got %#v want %v", localPosition["x"], 2.1)
	}
}

func TestFindByFileIDReturnsIndependentBlockCopy(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	doc := Build(blocks)

	block, ok := doc.FindByFileID(2000)
	if !ok {
		t.Fatal("FindByFileID(2000) found = false, want true")
	}

	block.Fields["m_Name"] = "ChangedByCaller"

	again, ok := doc.FindByFileID(2000)
	if !ok {
		t.Fatal("second FindByFileID(2000) found = false, want true")
	}

	name, ok := again.Fields["m_Name"].(string)
	if !ok {
		t.Fatalf("m_Name type mismatch: got %T want string", again.Fields["m_Name"])
	}

	if name != "Chair_01" {
		t.Fatalf("m_Name mismatch after lookup mutation: got %q want %q", name, "Chair_01")
	}
}
