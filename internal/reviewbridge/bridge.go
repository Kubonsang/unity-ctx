package reviewbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/reviewgrant"
	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

const ProtocolVersion = 1

type Request struct {
	ProtocolVersion int                              `json:"protocol_version"`
	Action          string                           `json:"action"`
	ProjectRoot     string                           `json:"project_root"`
	CurrentPath     string                           `json:"current_path"`
	CurrentHash     string                           `json:"current_hash"`
	DraftPath       string                           `json:"draft_path"`
	Reviewer        string                           `json:"reviewer"`
	Grant           spatialcontract.ApprovalEvidence `json:"grant"`
}

type Response struct {
	OK           bool   `json:"ok"`
	Status       string `json:"status,omitempty"`
	CurrentPath  string `json:"current_path,omitempty"`
	Backup       string `json:"backup,omitempty"`
	ContractHash string `json:"contract_hash,omitempty"`
	Written      bool   `json:"written,omitempty"`
	Error        string `json:"error,omitempty"`
}

type Config struct {
	Verifier spatialcontract.ApprovalVerifier
	Consumer spatialcontract.ApprovalGrantConsumer
	Ledger   *reviewgrant.Ledger
}

// Run serves exactly one request over stdio. Exit 0 means the approved
// contract was written (or already identical), 2 means malformed/untrusted
// input, and 1 means a verified request could not be applied.
func Run(input io.Reader, output, errorOutput io.Writer, config Config) int {
	decoder := json.NewDecoder(io.LimitReader(input, 1<<20))
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return fail(output, errorOutput, 2, fmt.Errorf("invalid bridge request: %w", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fail(output, errorOutput, 2, errors.New("invalid bridge request: trailing JSON content"))
	}
	if request.ProtocolVersion != ProtocolVersion || request.Action != spatialcontract.ApprovalActionApproveApply {
		return fail(output, errorOutput, 2, errors.New("unsupported bridge protocol or action"))
	}
	if !filepath.IsAbs(strings.TrimSpace(request.ProjectRoot)) {
		return fail(output, errorOutput, 2, errors.New("project_root must be an absolute project directory"))
	}
	projectRoot := filepath.Clean(request.ProjectRoot)
	if !filepath.IsAbs(request.DraftPath) {
		return fail(output, errorOutput, 2, errors.New("draft_path must be absolute and under the project's Library directory"))
	}
	draftPath := filepath.Clean(request.DraftPath)
	if !within(filepath.Join(projectRoot, "Library"), draftPath) {
		return fail(output, errorOutput, 2, errors.New("draft_path must be under the project's Library directory"))
	}
	if err := ensureExistingPathWithin(filepath.Join(projectRoot, "Library"), draftPath); err != nil {
		return fail(output, errorOutput, 2, err)
	}
	if !filepath.IsAbs(request.CurrentPath) {
		return fail(output, errorOutput, 2, errors.New("current_path must be absolute"))
	}
	currentPath := filepath.Clean(request.CurrentPath)
	draft, err := spatialcontract.Load(draftPath)
	if err != nil {
		return fail(output, errorOutput, 2, fmt.Errorf("load validated approval draft: %w", err))
	}
	verifier := config.Verifier
	consumer := config.Consumer
	if verifier == nil {
		authorityRoot, rootErr := reviewgrant.DefaultAuthorityRoot()
		if rootErr != nil {
			return fail(output, errorOutput, 1, rootErr)
		}
		verifier = reviewgrant.Verifier{AuthorityRoot: authorityRoot}
	}
	ledger := config.Ledger
	if ledger == nil {
		if concrete, ok := consumer.(*reviewgrant.Ledger); ok {
			ledger = concrete
		}
	}
	if consumer == nil || (draft.ContractType == spatialcontract.TypeInteraction && ledger == nil) {
		defaultLedger, ledgerErr := reviewgrant.DefaultLedger()
		if ledgerErr != nil {
			return fail(output, errorOutput, 1, ledgerErr)
		}
		if consumer == nil {
			consumer = defaultLedger
		}
		if ledger == nil {
			ledger = defaultLedger
		}
	}
	geometry := spatialcontract.ApprovalGeometryBindings{}
	if draft.ContractType == spatialcontract.TypeInteraction {
		if ledger == nil {
			return fail(output, errorOutput, 1, errors.New("interaction approval requires the concrete external approval ledger"))
		}
		provenance, resolveErr := ledger.ResolveInteractionGeometry(projectRoot, draft)
		if resolveErr != nil {
			return fail(output, errorOutput, 1, resolveErr)
		}
		geometry = provenance.ApprovalBindings()
		geometry.RevalidateCurrent = func() error {
			current, currentErr := ledger.ResolveInteractionGeometry(projectRoot, draft)
			if currentErr != nil {
				return currentErr
			}
			if !strings.EqualFold(current.SubjectGeometryHash, provenance.SubjectGeometryHash) || !strings.EqualFold(current.TargetGeometryHash, provenance.TargetGeometryHash) {
				return errors.New("SUPPORT_CONTRACT_STALE: dependency geometry changed before interaction apply")
			}
			return nil
		}
	}
	result, err := spatialcontract.ApproveAndApplyAuthorizedWithGeometry(projectRoot, currentPath, draftPath, request.CurrentHash, request.Reviewer, request.Grant, geometry, verifier, consumer)
	if err != nil {
		if result.Written || result.Backup != "" || result.Status != "" {
			response := Response{OK: false, Status: result.Status, CurrentPath: result.Current, Backup: result.Backup, ContractHash: result.ContractHash, Written: result.Written, Error: err.Error()}
			if encodeErr := json.NewEncoder(output).Encode(response); encodeErr != nil {
				_, _ = fmt.Fprintf(errorOutput, "ERROR encode committed bridge response: %v\n", encodeErr)
				return 1
			}
			_, _ = fmt.Fprintf(errorOutput, "ERROR %v current=%s backup=%s\n", err, result.Current, result.Backup)
			return 1
		}
		return fail(output, errorOutput, 1, err)
	}
	response := Response{OK: true, Status: result.Status, CurrentPath: result.Current, Backup: result.Backup, ContractHash: result.ContractHash, Written: result.Written}
	if err := json.NewEncoder(output).Encode(response); err != nil {
		_, _ = fmt.Fprintf(errorOutput, "ERROR encode bridge response: %v\n", err)
		return 1
	}
	return 0
}

func fail(output, errorOutput io.Writer, code int, err error) int {
	_ = json.NewEncoder(output).Encode(Response{OK: false, Error: err.Error()})
	_, _ = fmt.Fprintf(errorOutput, "ERROR %v\n", err)
	return code
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)))
}

func ensureExistingPathWithin(root, candidate string) error {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("draft_path escapes the project Library directory")
	}
	current := filepath.Clean(root)
	parts := strings.Split(relative, string(filepath.Separator))
	var info os.FileInfo
	for index := -1; index < len(parts); index++ {
		if index >= 0 {
			current = filepath.Join(current, parts[index])
		}
		info, err = os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect draft_path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("draft_path contains a symlink or junction")
		}
	}
	if info == nil || info.IsDir() {
		return errors.New("draft_path must be an existing file")
	}
	return nil
}
