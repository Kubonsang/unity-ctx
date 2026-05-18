package mutation

import (
	"fmt"
	"path/filepath"
	"strings"

	"unity-ctx/internal/document"
	"unity-ctx/internal/parser"
)

type PrefabSetRequest struct {
	Path    string
	HasID   bool
	ID      int64
	Field   string
	Value   string
	Rewrite bool
}

type PrefabSetResult struct {
	Field       string
	OldValue    string
	NewValue    string
	TypeHint    string
	Changed     bool
	UpdatedData []byte
}

func PlanPrefabSet(input []byte, blocks []parser.Block, req PrefabSetRequest) (PrefabSetResult, error) {
	if err := validatePrefabMutationPath(req.Path); err != nil {
		return PrefabSetResult{}, err
	}

	target, err := resolvePrefabTarget(blocks, req)
	if err != nil {
		return PrefabSetResult{}, err
	}

	oldValue, ok := document.ResolveField(target.Fields, req.Field)
	if !ok {
		return PrefabSetResult{}, fmt.Errorf("FIELD_NOT_FOUND field=%s", req.Field)
	}

	newScalar, typeHint, changed, err := coerceScalar(oldValue, req.Value)
	if err != nil {
		return PrefabSetResult{}, err
	}

	updated := cloneBytes(input)
	if !changed {
		newScalar = renderedScalarValue(input, target, req.Field, formatValue(oldValue))
	}
	if req.Rewrite && changed {
		updated, err = rewriteScalarField(input, target, req.Field, newScalar)
		if err != nil {
			return PrefabSetResult{}, err
		}
	}

	return PrefabSetResult{
		Field:       req.Field,
		OldValue:    formatValue(oldValue),
		NewValue:    newScalar,
		TypeHint:    typeHint,
		Changed:     changed,
		UpdatedData: updated,
	}, nil
}

func validatePrefabMutationPath(path string) error {
	kind := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if kind == ".prefab" {
		return nil
	}
	if kind == "" {
		kind = "unknown"
	}

	return fmt.Errorf("UNSUPPORTED_FILE_KIND kind=%s allowed=.prefab", kind)
}

func resolvePrefabTarget(blocks []parser.Block, req PrefabSetRequest) (parser.Block, error) {
	if !req.HasID {
		return parser.Block{}, fmt.Errorf("NEED_RULE target=fileID")
	}

	for _, block := range blocks {
		if block.FileID == req.ID {
			return block, nil
		}
	}

	return parser.Block{}, fmt.Errorf("NOT_FOUND fileID=%d", req.ID)
}
