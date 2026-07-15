package spatialcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/durablefs"
)

const ContractVersion = 1

const (
	TypeAsset       = "asset"
	TypeInteraction = "interaction"
)

const (
	StateDraft               = "Draft"
	StateTechnicalPassed     = "TechnicalPassed"
	StateAwaitingHumanReview = "AwaitingHumanReview"
	StateApproved            = "Approved"
	StateTechnicalFailed     = "TechnicalFailed"
	StateRevisionRequested   = "RevisionRequested"
	StateUnableToJudge       = "UnableToJudge"
	StateStale               = "Stale"
)

type Vec3 [3]float64
type Quat [4]float64

type OBB struct {
	ID       string `json:"id"`
	Center   Vec3   `json:"center"`
	Size     Vec3   `json:"size"`
	Rotation Quat   `json:"rotation"`
}

type ContactFrame struct {
	ID      string     `json:"id"`
	Point   Vec3       `json:"point"`
	Normal  Vec3       `json:"normal"`
	Tangent Vec3       `json:"tangent"`
	Size    [2]float64 `json:"size"`
}

type ContactRequirement struct {
	ID                 string  `json:"id"`
	Kind               string  `json:"kind"`
	FrameID            string  `json:"frame_id"`
	Target             string  `json:"target"`
	MinimumGap         float64 `json:"minimum_gap"`
	MaximumGap         float64 `json:"maximum_gap"`
	MaximumPenetration float64 `json:"maximum_penetration"`
	MinimumSupport     float64 `json:"minimum_support"`
	DirectionAlignment float64 `json:"direction_alignment"`
}

type AssetSpatialContract struct {
	AssetGUID        string               `json:"asset_guid"`
	AssetPath        string               `json:"asset_path"`
	DependencyHash   string               `json:"dependency_hash"`
	Units            string               `json:"units"`
	Forward          Vec3                 `json:"forward"`
	Up               Vec3                 `json:"up"`
	PivotOffset      Vec3                 `json:"pivot_offset"`
	CollisionProxies []OBB                `json:"collision_proxies"`
	ClearanceProxies []OBB                `json:"clearance_proxies,omitempty"`
	Frames           []ContactFrame       `json:"frames"`
	Contacts         []ContactRequirement `json:"contacts"`
	Revision         int                  `json:"revision"`
	GeometryHash     string               `json:"geometry_hash"`
	CaptureSetHash   string               `json:"capture_set_hash"`
}

type InteractionContract struct {
	SubjectGUID       string  `json:"subject_guid"`
	TargetKey         string  `json:"target_key"`
	Relation          string  `json:"relation"`
	SubjectFrame      string  `json:"subject_frame"`
	TargetFrame       string  `json:"target_frame"`
	RelativePosition  Vec3    `json:"relative_position"`
	RelativeRotation  Quat    `json:"relative_rotation"`
	PositionTolerance Vec3    `json:"position_tolerance"`
	AngleTolerance    float64 `json:"angle_tolerance"`
	CollisionPolicy   string  `json:"collision_policy"`
	Revision          int     `json:"revision"`
	InteractionHash   string  `json:"interaction_hash"`
	CaptureSetHash    string  `json:"capture_set_hash"`
}

type TechnicalEvidence struct {
	Passed     bool   `json:"passed"`
	ErrorCount int    `json:"error_count"`
	ReportHash string `json:"report_hash"`
}

// ApprovalEvidence is an opaque attestation issued by the local human-review
// bridge. unity-ctx deliberately does not know how to create this evidence;
// the embedding bridge owns the nonce and proof verification policy.
type ApprovalEvidence struct {
	Authority   string `json:"authority"`
	Nonce       string `json:"nonce"`
	ExpiresUnix int64  `json:"expires_unix,omitempty"`
	Proof       string `json:"proof"`
}

const (
	ApprovalActionReview       = "review"
	ApprovalActionApproveApply = "approve_apply"
	// CurrentHashAbsent is the signed compare-and-swap baseline used when the
	// canonical tracked destination did not exist at diff time.
	CurrentHashAbsent = "absent"
)

// ApprovalVerification is the exact review decision covered by an
// ApprovalEvidence value. Verifiers must bind all of these fields to the
// evidence so a proof cannot be replayed for another contract or capture set.
type ApprovalVerification struct {
	Action                 string
	ContractHash           string
	CurrentHash            string
	CaptureSetHash         string
	Reviewer               string
	Destination            string
	SubjectGeometryHash    string
	TargetGeometryHash     string
	DependencyDestinations []string
	Evidence               ApprovalEvidence
}

// ApprovalGeometryBindings are resolved from independently approved asset
// contracts. They are signed with an interaction approval so a caller cannot
// relabel an old relative pose with whichever geometry hashes are current.
type ApprovalGeometryBindings struct {
	SubjectGeometryHash    string       `json:"subject_geometry_hash,omitempty"`
	TargetGeometryHash     string       `json:"target_geometry_hash,omitempty"`
	DependencyDestinations []string     `json:"-"`
	RevalidateCurrent      func() error `json:"-"`
}

// ApprovalVerifier is supplied by the trusted local human-review bridge.
// The public CLI never supplies a verifier and therefore cannot approve or
// persist an approved contract.
type ApprovalVerifier interface {
	VerifyApproval(ApprovalVerification) error
}

// ApprovalGrantConsumer atomically consumes an action-scoped grant after its
// signature has been verified. Implementations must reject a reused
// authority/nonce pair. The local review bridge keeps this ledger outside the
// tracked Unity project.
type ApprovalGrantConsumer interface {
	ConsumeApprovalGrant(ApprovalVerification, func() error) error
}

// ApprovedContractVerification is used when an Approved contract is consumed
// after the one-shot approval operation (for example by a detailed scan). The
// verifier is expected to consult the trusted bridge's external approval
// ledger; StateApproved on its own is never proof of authority.
type ApprovedContractVerification struct {
	ContractType        string
	ContractHash        string
	CaptureSetHash      string
	Reviewer            string
	ContractPath        string
	SubjectGeometryHash string
	TargetGeometryHash  string
}

type ApprovedContractVerifier interface {
	VerifyApprovedContract(ApprovedContractVerification) error
}

// ApprovedVerification builds the exact, validated ledger lookup for a saved
// Approved contract. Tracked JSON is not authority by itself; callers must pass
// the result to an ApprovedContractVerifier before consuming the contract.
func ApprovedVerification(contract Contract, contractPath string) (ApprovedContractVerification, error) {
	if err := Validate(contract); err != nil {
		return ApprovedContractVerification{}, err
	}
	if contract.State != StateApproved || contract.Review == nil || contract.Review.Decision != StateApproved {
		return ApprovedContractVerification{}, errors.New("spatial contract is not Approved")
	}
	absolute, err := filepath.Abs(contractPath)
	if err != nil {
		return ApprovedContractVerification{}, err
	}
	contractHash, err := ContentHashChecked(contract)
	if err != nil {
		return ApprovedContractVerification{}, err
	}
	return ApprovedContractVerification{
		ContractType:   contract.ContractType,
		ContractHash:   contractHash,
		CaptureSetHash: captureHash(contract),
		Reviewer:       contract.Review.Reviewer,
		ContractPath:   filepath.Clean(absolute),
	}, nil
}

type OverlayPolicy struct {
	Verifier ApprovedContractVerifier
}

type HumanReview struct {
	Decision       string            `json:"decision"`
	ContractHash   string            `json:"contract_hash"`
	CaptureSetHash string            `json:"capture_set_hash"`
	Reviewer       string            `json:"reviewer"`
	Authorization  *ApprovalEvidence `json:"authorization,omitempty"`
	IssueTypes     []string          `json:"issue_types,omitempty"`
	Comment        string            `json:"comment,omitempty"`
	Revision       int               `json:"revision"`
}

type Contract struct {
	ContractVersion int                   `json:"contract_version"`
	ContractType    string                `json:"contract_type"`
	State           string                `json:"state"`
	Asset           *AssetSpatialContract `json:"asset,omitempty"`
	Interaction     *InteractionContract  `json:"interaction,omitempty"`
	Technical       *TechnicalEvidence    `json:"technical,omitempty"`
	Review          *HumanReview          `json:"review,omitempty"`
}

type DiffResult struct {
	Status              string   `json:"status"`
	Current             string   `json:"current"`
	Draft               string   `json:"draft"`
	ContractType        string   `json:"contract_type"`
	ContractHash        string   `json:"contract_hash"`
	ProposalHash        string   `json:"proposal_hash"`
	CurrentHash         string   `json:"current_hash"`
	AssetGUID           string   `json:"asset_guid,omitempty"`
	GeometryHash        string   `json:"geometry_hash,omitempty"`
	SubjectGUID         string   `json:"subject_guid,omitempty"`
	TargetKey           string   `json:"target_key,omitempty"`
	SubjectGeometryHash string   `json:"subject_geometry_hash,omitempty"`
	TargetGeometryHash  string   `json:"target_geometry_hash,omitempty"`
	Changed             bool     `json:"changed"`
	Fields              []string `json:"fields"`
}

type ApplyResult struct {
	Status       string `json:"status"`
	Current      string `json:"current"`
	Draft        string `json:"draft"`
	Backup       string `json:"backup,omitempty"`
	ContractHash string `json:"contract_hash"`
	Changed      bool   `json:"changed"`
	Written      bool   `json:"written"`
	Verified     bool   `json:"verified"`
}

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

func Load(path string) (Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, err
	}
	return Decode(data)
}

func Decode(data []byte) (Contract, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var contract Contract
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, fmt.Errorf("invalid spatial contract: %w", err)
	}
	providedGeometryHash := ""
	providedInteractionHash := ""
	if contract.Asset != nil {
		providedGeometryHash = contract.Asset.GeometryHash
	}
	if contract.Interaction != nil {
		providedInteractionHash = contract.Interaction.InteractionHash
	}
	if contract.State != "" && contract.State != StateDraft {
		if contract.Asset != nil && strings.TrimSpace(providedGeometryHash) == "" {
			return Contract{}, errors.New("invalid spatial contract: non-Draft asset contract requires an embedded geometry_hash")
		}
		if contract.Interaction != nil && strings.TrimSpace(providedInteractionHash) == "" {
			return Contract{}, errors.New("invalid spatial contract: non-Draft interaction contract requires an embedded interaction_hash")
		}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Contract{}, errors.New("invalid spatial contract: unexpected trailing JSON content")
		}
		return Contract{}, fmt.Errorf("invalid spatial contract: %w", err)
	}
	Normalize(&contract)
	if providedGeometryHash != "" && contract.Asset != nil && providedGeometryHash != contract.Asset.GeometryHash {
		return Contract{}, errors.New("invalid spatial contract: asset.geometry_hash does not match geometry")
	}
	if providedInteractionHash != "" && contract.Interaction != nil && providedInteractionHash != contract.Interaction.InteractionHash {
		return Contract{}, errors.New("invalid spatial contract: interaction.interaction_hash does not match interaction")
	}
	if err := Validate(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func Save(path string, contract Contract) error {
	if contract.State != "" && contract.State != StateDraft {
		if contract.Asset != nil && strings.TrimSpace(contract.Asset.GeometryHash) == "" {
			return errors.New("invalid spatial contract: non-Draft asset contract requires an embedded geometry_hash")
		}
		if contract.Interaction != nil && strings.TrimSpace(contract.Interaction.InteractionHash) == "" {
			return errors.New("invalid spatial contract: non-Draft interaction contract requires an embedded interaction_hash")
		}
	}
	contract = cloneContract(contract)
	Normalize(&contract)
	if err := Validate(contract); err != nil {
		return err
	}
	data, err := encode(contract)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Normalize(contract *Contract) {
	if contract == nil {
		return
	}
	previousPayloadHash := payloadHash(contract)
	if contract.ContractVersion == 0 {
		contract.ContractVersion = ContractVersion
	}
	if strings.TrimSpace(contract.State) == "" {
		contract.State = StateDraft
	}
	if contract.Asset != nil {
		a := contract.Asset
		a.AssetGUID = strings.ToLower(strings.TrimSpace(a.AssetGUID))
		a.AssetPath = filepath.ToSlash(strings.TrimSpace(a.AssetPath))
		a.Units = strings.ToLower(strings.TrimSpace(a.Units))
		if a.Revision < 1 {
			a.Revision = 1
		}
		normalizeOBBs(a.CollisionProxies)
		normalizeOBBs(a.ClearanceProxies)
		for i := range a.Frames {
			normalizeVec(&a.Frames[i].Point)
			normalizeVec(&a.Frames[i].Normal)
			normalizeVec(&a.Frames[i].Tangent)
			a.Frames[i].Size[0] = round(a.Frames[i].Size[0])
			a.Frames[i].Size[1] = round(a.Frames[i].Size[1])
		}
		for i := range a.Contacts {
			a.Contacts[i].MinimumGap = round(a.Contacts[i].MinimumGap)
			a.Contacts[i].MaximumGap = round(a.Contacts[i].MaximumGap)
			a.Contacts[i].MaximumPenetration = round(a.Contacts[i].MaximumPenetration)
			a.Contacts[i].MinimumSupport = round(a.Contacts[i].MinimumSupport)
			a.Contacts[i].DirectionAlignment = round(a.Contacts[i].DirectionAlignment)
		}
		normalizeVec(&a.Forward)
		normalizeVec(&a.Up)
		normalizeVec(&a.PivotOffset)
		sort.Slice(a.CollisionProxies, func(i, j int) bool { return utf16OrdinalLess(a.CollisionProxies[i].ID, a.CollisionProxies[j].ID) })
		sort.Slice(a.ClearanceProxies, func(i, j int) bool { return utf16OrdinalLess(a.ClearanceProxies[i].ID, a.ClearanceProxies[j].ID) })
		sort.Slice(a.Frames, func(i, j int) bool { return utf16OrdinalLess(a.Frames[i].ID, a.Frames[j].ID) })
		sort.Slice(a.Contacts, func(i, j int) bool { return utf16OrdinalLess(a.Contacts[i].ID, a.Contacts[j].ID) })
		a.GeometryHash = assetHash(*a)
	}
	if contract.Interaction != nil {
		i := contract.Interaction
		i.SubjectGUID = strings.ToLower(strings.TrimSpace(i.SubjectGUID))
		if i.Revision < 1 {
			i.Revision = 1
		}
		normalizeVec(&i.RelativePosition)
		normalizeVec(&i.PositionTolerance)
		normalizeQuat(&i.RelativeRotation)
		i.AngleTolerance = round(i.AngleTolerance)
		i.InteractionHash = interactionHash(*i)
	}
	if currentPayloadHash := payloadHash(contract); previousPayloadHash != "" && previousPayloadHash != currentPayloadHash {
		invalidateChangedPayload(contract)
	}
	if contract.Review != nil {
		sort.Strings(contract.Review.IssueTypes)
		contract.Review.Reviewer = strings.TrimSpace(contract.Review.Reviewer)
		if contract.Review.Authorization != nil {
			contract.Review.Authorization.Authority = strings.TrimSpace(contract.Review.Authorization.Authority)
			contract.Review.Authorization.Nonce = strings.TrimSpace(contract.Review.Authorization.Nonce)
			contract.Review.Authorization.Proof = strings.TrimSpace(contract.Review.Authorization.Proof)
		}
		if contract.Review.Revision < 1 {
			contract.Review.Revision = 1
		}
	}
}

func payloadHash(contract *Contract) string {
	if contract == nil {
		return ""
	}
	if contract.Asset != nil {
		return contract.Asset.GeometryHash
	}
	if contract.Interaction != nil {
		return contract.Interaction.InteractionHash
	}
	return ""
}

func invalidateChangedPayload(contract *Contract) {
	if contract.State == StateDraft && contract.Technical == nil && contract.Review == nil {
		return
	}
	contract.State = StateStale
	contract.Technical = nil
	contract.Review = nil
}

func Validate(contract Contract) error {
	if contract.ContractVersion != ContractVersion {
		return fmt.Errorf("invalid spatial contract: contract_version must be %d", ContractVersion)
	}
	if !validState(contract.State) {
		return fmt.Errorf("invalid spatial contract: unsupported state %q", contract.State)
	}
	switch contract.ContractType {
	case TypeAsset:
		if contract.Asset == nil || contract.Interaction != nil {
			return errors.New("invalid spatial contract: asset contract requires only asset payload")
		}
		if err := validateAsset(*contract.Asset); err != nil {
			return err
		}
	case TypeInteraction:
		if contract.Interaction == nil || contract.Asset != nil {
			return errors.New("invalid spatial contract: interaction contract requires only interaction payload")
		}
		if err := validateInteraction(*contract.Interaction); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid spatial contract: contract_type must be %q or %q", TypeAsset, TypeInteraction)
	}
	if contract.Technical != nil {
		if contract.Technical.ErrorCount < 0 {
			return errors.New("invalid spatial contract: technical.error_count must be >= 0")
		}
		if strings.TrimSpace(contract.Technical.ReportHash) == "" {
			return errors.New("invalid spatial contract: technical.report_hash is required")
		}
		if contract.Technical.Passed && contract.Technical.ErrorCount != 0 {
			return errors.New("invalid spatial contract: passed technical evidence requires error_count=0")
		}
	}
	switch contract.State {
	case StateDraft:
		if contract.Review != nil {
			return errors.New("invalid spatial contract: Draft cannot contain human review evidence")
		}
	case StateTechnicalPassed, StateAwaitingHumanReview:
		if contract.Technical == nil || !contract.Technical.Passed || contract.Technical.ErrorCount != 0 {
			return fmt.Errorf("invalid spatial contract: %s requires passed technical evidence with zero errors", contract.State)
		}
		if contract.Review != nil {
			return fmt.Errorf("invalid spatial contract: %s cannot contain human review evidence", contract.State)
		}
		if contract.State == StateAwaitingHumanReview && captureHash(contract) == "" {
			return errors.New("invalid spatial contract: AwaitingHumanReview requires capture_set_hash")
		}
	case StateTechnicalFailed:
		if contract.Technical == nil || contract.Technical.Passed || contract.Technical.ErrorCount == 0 {
			return errors.New("invalid spatial contract: TechnicalFailed requires failed technical evidence with errors")
		}
		if contract.Review != nil {
			return errors.New("invalid spatial contract: TechnicalFailed cannot contain human review evidence")
		}
	case StateRevisionRequested, StateUnableToJudge:
		if err := validateCurrentReview(contract, contract.State); err != nil {
			return err
		}
		if contract.Review.Authorization != nil {
			return fmt.Errorf("invalid spatial contract: %s cannot contain approval authorization", contract.State)
		}
	case StateStale:
		if contract.Technical != nil || contract.Review != nil {
			return errors.New("invalid spatial contract: Stale cannot retain technical or human review evidence")
		}
	case StateApproved:
		if contract.Technical == nil || !contract.Technical.Passed || contract.Technical.ErrorCount != 0 {
			return errors.New("invalid spatial contract: Approved requires passed technical evidence with zero errors")
		}
		if err := validateCurrentReview(contract, StateApproved); err != nil {
			return err
		}
		// Legacy approved records remain readable for migration. Any new
		// authorized approval contains all three fields, and partial evidence is
		// always invalid. ApplyAuthorized additionally requires and verifies it.
		if authorization := contract.Review.Authorization; authorization != nil {
			if authorization.Authority == "" || authorization.Nonce == "" || authorization.Proof == "" {
				return errors.New("invalid spatial contract: approval authorization requires authority, nonce, and proof")
			}
		}
	}
	return nil
}

func validateCurrentReview(contract Contract, decision string) error {
	if contract.Review == nil || contract.Review.Decision != decision {
		return fmt.Errorf("invalid spatial contract: %s requires a matching human review", decision)
	}
	if contract.Review.Revision < 1 {
		return errors.New("invalid spatial contract: review.revision must be >= 1")
	}
	hash, err := ContentHashChecked(contract)
	if err != nil {
		return fmt.Errorf("invalid spatial contract: canonical contract hash: %w", err)
	}
	if contract.Review.ContractHash != hash {
		return fmt.Errorf("invalid spatial contract: review contract_hash is stale got=%s want=%s", contract.Review.ContractHash, hash)
	}
	if contract.Review.CaptureSetHash == "" || contract.Review.CaptureSetHash != captureHash(contract) {
		return errors.New("invalid spatial contract: review capture_set_hash is stale")
	}
	if strings.TrimSpace(contract.Review.Reviewer) == "" {
		return fmt.Errorf("invalid spatial contract: %s requires review.reviewer", decision)
	}
	return nil
}

func Approve(contract *Contract, reviewer string) error {
	return errors.New("human approval requires authorization from the local review bridge")
}

func Review(contract *Contract, decision, reviewer string, issues []string, comment string) error {
	if contract == nil {
		return errors.New("contract is nil")
	}
	Normalize(contract)
	if decision != StateApproved && decision != StateRevisionRequested && decision != StateUnableToJudge {
		return errors.New("human review decision must be Approved, RevisionRequested, or UnableToJudge")
	}
	if decision == StateApproved {
		return errors.New("human approval requires authorization from the local review bridge")
	}
	return recordReview(contract, decision, reviewer, issues, comment)
}

// ReviewAuthorized records an Approved decision only after a trusted local
// bridge has verified its human-review evidence. It exists for trusted
// in-process integrations and tests. Production bridges should prefer
// ApproveAndApplyAuthorized, which consumes a one-shot grant and never leaves
// an independently writable approved draft behind.
func ReviewAuthorized(contract *Contract, reviewer string, evidence ApprovalEvidence, verifier ApprovalVerifier) error {
	if contract == nil {
		return errors.New("contract is nil")
	}
	Normalize(contract)
	if verifier == nil {
		return errors.New("human approval requires a bridge approval verifier")
	}
	evidence.Authority = strings.TrimSpace(evidence.Authority)
	evidence.Nonce = strings.TrimSpace(evidence.Nonce)
	evidence.Proof = strings.TrimSpace(evidence.Proof)
	if evidence.Authority == "" || evidence.Nonce == "" || evidence.Proof == "" {
		return errors.New("human approval evidence requires authority, nonce, and proof")
	}
	if contract.State != StateAwaitingHumanReview {
		return fmt.Errorf("human approval requires %s state, got %s", StateAwaitingHumanReview, contract.State)
	}
	if strings.TrimSpace(reviewer) == "" {
		return errors.New("human review requires reviewer")
	}
	if contract.Technical == nil || !contract.Technical.Passed || contract.Technical.ErrorCount != 0 {
		return errors.New("technical validation must pass before human approval")
	}
	capture := captureHash(*contract)
	if capture == "" {
		return errors.New("capture_set_hash is required before human approval")
	}
	contractHash, err := ContentHashChecked(*contract)
	if err != nil {
		return fmt.Errorf("human approval canonical contract hash: %w", err)
	}
	verification := ApprovalVerification{
		Action:         ApprovalActionReview,
		ContractHash:   contractHash,
		CaptureSetHash: capture,
		Reviewer:       strings.TrimSpace(reviewer),
		Evidence:       evidence,
	}
	if err := verifier.VerifyApproval(verification); err != nil {
		return fmt.Errorf("human approval authorization failed: %w", err)
	}
	// A grant is authorization for one action, not a durable credential. Never
	// serialize its nonce or proof into the tracked contract.
	return recordReview(contract, StateApproved, reviewer, nil, "")
}

func recordReview(contract *Contract, decision, reviewer string, issues []string, comment string) error {
	if contract.State != StateAwaitingHumanReview {
		return fmt.Errorf("human review requires %s state, got %s", StateAwaitingHumanReview, contract.State)
	}
	if strings.TrimSpace(reviewer) == "" {
		return errors.New("human review requires reviewer")
	}
	capture := captureHash(*contract)
	if capture == "" {
		return errors.New("capture_set_hash is required before human review")
	}
	candidate := cloneContract(*contract)
	candidate.State = decision
	contractHash, err := ContentHashChecked(candidate)
	if err != nil {
		return fmt.Errorf("human review canonical contract hash: %w", err)
	}
	candidate.Review = &HumanReview{
		Decision:       decision,
		ContractHash:   contractHash,
		CaptureSetHash: capture,
		Reviewer:       strings.TrimSpace(reviewer),
		IssueTypes:     append([]string(nil), issues...),
		Comment:        strings.TrimSpace(comment),
		Revision:       1,
	}
	if err := Validate(candidate); err != nil {
		return err
	}
	*contract = candidate
	return nil
}

func ContentHashChecked(contract Contract) (string, error) {
	contract = cloneContract(contract)
	Normalize(&contract)
	contract.State = ""
	contract.Review = nil
	return canonicalHash(contract)
}

// ContentHash is the compatibility form used by already-validated callers.
// Invalid/non-canonical payloads return an empty, non-authorizable value rather
// than collapsing onto the SHA-256 of partial encoder output.
func ContentHash(contract Contract) string {
	hash, _ := ContentHashChecked(contract)
	return hash
}

// ProposalHash identifies the reviewable asset or interaction payload without
// capture evidence or its recursively derived embedded hash. Capture manifests
// can bind this value before capture_set_hash exists, and recapturing identical
// geometry does not manufacture a different proposal identity.
func ProposalHashChecked(contract Contract) (string, error) {
	contract = cloneContract(contract)
	Normalize(&contract)
	contract.State = ""
	contract.Review = nil
	contract.Technical = nil
	if contract.Asset != nil {
		contract.Asset.GeometryHash = ""
		contract.Asset.CaptureSetHash = ""
	}
	if contract.Interaction != nil {
		contract.Interaction.InteractionHash = ""
		contract.Interaction.CaptureSetHash = ""
	}
	payload := struct {
		Domain          string                `json:"domain"`
		ContractVersion int                   `json:"contract_version"`
		ContractType    string                `json:"contract_type"`
		Asset           *AssetSpatialContract `json:"asset,omitempty"`
		Interaction     *InteractionContract  `json:"interaction,omitempty"`
	}{
		Domain: "unity-ctx-spatial-proposal-v1", ContractVersion: contract.ContractVersion,
		ContractType: contract.ContractType, Asset: contract.Asset, Interaction: contract.Interaction,
	}
	return canonicalHash(payload)
}

// ProposalHash is the compatibility form used by already-validated callers.
// Invalid/non-canonical payloads fail closed with an empty value.
func ProposalHash(contract Contract) string {
	hash, _ := ProposalHashChecked(contract)
	return hash
}

func cloneContract(contract Contract) Contract {
	copy := contract
	if contract.Asset != nil {
		asset := *contract.Asset
		asset.CollisionProxies = append([]OBB(nil), contract.Asset.CollisionProxies...)
		asset.ClearanceProxies = append([]OBB(nil), contract.Asset.ClearanceProxies...)
		asset.Frames = append([]ContactFrame(nil), contract.Asset.Frames...)
		asset.Contacts = append([]ContactRequirement(nil), contract.Asset.Contacts...)
		copy.Asset = &asset
	}
	if contract.Interaction != nil {
		interaction := *contract.Interaction
		copy.Interaction = &interaction
	}
	if contract.Technical != nil {
		technical := *contract.Technical
		copy.Technical = &technical
	}
	if contract.Review != nil {
		review := *contract.Review
		review.IssueTypes = append([]string(nil), contract.Review.IssueTypes...)
		if contract.Review.Authorization != nil {
			authorization := *contract.Review.Authorization
			review.Authorization = &authorization
		}
		copy.Review = &review
	}
	return copy
}

func Diff(currentPath, draftPath string) (DiffResult, error) {
	draft, err := Load(draftPath)
	if err != nil {
		return DiffResult{}, err
	}
	contractHash, err := ContentHashChecked(draft)
	if err != nil {
		return DiffResult{}, err
	}
	proposalHash, err := ProposalHashChecked(draft)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{
		Status: "OK", Current: currentPath, Draft: draftPath,
		ContractType: draft.ContractType, ContractHash: contractHash, ProposalHash: proposalHash,
	}
	if draft.Asset != nil {
		result.AssetGUID = draft.Asset.AssetGUID
		result.GeometryHash = draft.Asset.GeometryHash
	}
	if draft.Interaction != nil {
		result.SubjectGUID = draft.Interaction.SubjectGUID
		result.TargetKey = draft.Interaction.TargetKey
	}
	currentHash, current, err := currentBaseline(currentPath, draft)
	if err != nil {
		return DiffResult{}, err
	}
	result.CurrentHash = currentHash
	if current == nil {
		result.Status = "NEW"
		result.Changed = true
		result.Fields = []string{"contract"}
		return result, nil
	}
	result.Fields = changedFields(*current, draft)
	result.Changed = len(result.Fields) > 0
	if !result.Changed {
		result.Status = "UNCHANGED"
	}
	return result, nil
}

func Apply(currentPath, draftPath string, write bool) (ApplyResult, error) {
	if write {
		return ApplyResult{}, errors.New("spatial apply --write is unavailable in the public CLI; use the authorized local review bridge")
	}
	result, _, err := prepareApply(currentPath, draftPath)
	return result, err
}

// ApplyAuthorized writes an approved draft only after the trusted local bridge
// re-verifies the exact human approval evidence stored with the review. The
// public CLI exposes only Apply's read-only dry run.
func ApplyAuthorized(currentPath, draftPath string, verifier ApprovalVerifier) (ApplyResult, error) {
	return ApplyResult{}, errors.New("separate authorized apply is disabled; use atomic ApproveAndApplyAuthorized with a fresh action-scoped grant")
}

// ApproveAndApplyAuthorized is the production bridge boundary. It verifies an
// AwaitingHumanReview draft, binds a signed one-shot grant to the exact content,
// capture, reviewer and canonical destination, consumes that grant, and writes
// the Approved contract as one logical operation. Grant evidence is never
// persisted in the contract.
func ApproveAndApplyAuthorized(projectRoot, currentPath, draftPath, expectedCurrentHash, reviewer string, evidence ApprovalEvidence, verifier ApprovalVerifier, consumer ApprovalGrantConsumer) (ApplyResult, error) {
	return ApproveAndApplyAuthorizedWithGeometry(projectRoot, currentPath, draftPath, expectedCurrentHash, reviewer, evidence, ApprovalGeometryBindings{}, verifier, consumer)
}

// ApproveAndApplyAuthorizedWithGeometry is the interaction-aware production
// boundary. The caller must resolve these hashes from ledger-authorized asset
// contracts; accepting hashes copied from the interaction caller would make a
// stale pose appear current without another human review.
func ApproveAndApplyAuthorizedWithGeometry(projectRoot, currentPath, draftPath, expectedCurrentHash, reviewer string, evidence ApprovalEvidence, geometry ApprovalGeometryBindings, verifier ApprovalVerifier, consumer ApprovalGrantConsumer) (ApplyResult, error) {
	if verifier == nil {
		return ApplyResult{}, errors.New("atomic spatial approval requires a bridge approval verifier")
	}
	if consumer == nil {
		return ApplyResult{}, errors.New("atomic spatial approval requires a consume-once grant ledger")
	}
	draft, err := Load(draftPath)
	if err != nil {
		return ApplyResult{}, err
	}
	if draft.State != StateAwaitingHumanReview {
		return ApplyResult{}, fmt.Errorf("atomic spatial approval requires %s draft, got %s", StateAwaitingHumanReview, draft.State)
	}
	geometry.SubjectGeometryHash = strings.ToLower(strings.TrimSpace(geometry.SubjectGeometryHash))
	geometry.TargetGeometryHash = strings.ToLower(strings.TrimSpace(geometry.TargetGeometryHash))
	switch draft.ContractType {
	case TypeAsset:
		if geometry.SubjectGeometryHash != "" || geometry.TargetGeometryHash != "" || len(geometry.DependencyDestinations) != 0 || geometry.RevalidateCurrent != nil {
			return ApplyResult{}, errors.New("asset approval must not carry interaction geometry bindings")
		}
	case TypeInteraction:
		if _, _, err := InteractionAssetGUIDs(draft); err != nil {
			return ApplyResult{}, err
		}
		if !sha256Hex(geometry.SubjectGeometryHash) || !sha256Hex(geometry.TargetGeometryHash) {
			return ApplyResult{}, errors.New("SUPPORT_CONTRACT_STALE: interaction approval requires ledger-authorized subject and target geometry hashes")
		}
		if len(geometry.DependencyDestinations) != 2 || geometry.RevalidateCurrent == nil {
			return ApplyResult{}, errors.New("SUPPORT_CONTRACT_STALE: interaction approval requires dependency locks and in-lock geometry revalidation")
		}
		for _, dependency := range geometry.DependencyDestinations {
			if !filepath.IsAbs(strings.TrimSpace(dependency)) {
				return ApplyResult{}, errors.New("interaction approval dependency destinations must be absolute")
			}
		}
	default:
		return ApplyResult{}, errors.New("spatial approval requires an asset or interaction contract")
	}
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return ApplyResult{}, errors.New("human review requires reviewer")
	}
	if err := validateCanonicalDestination(projectRoot, currentPath, draft); err != nil {
		return ApplyResult{}, err
	}
	expectedCurrentHash, err = normalizeCurrentHash(expectedCurrentHash)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := verifyCurrentBaseline(currentPath, draft, expectedCurrentHash); err != nil {
		return ApplyResult{}, err
	}

	evidence.Authority = strings.TrimSpace(evidence.Authority)
	evidence.Nonce = strings.TrimSpace(evidence.Nonce)
	evidence.Proof = strings.TrimSpace(evidence.Proof)
	if evidence.Authority == "" || evidence.Nonce == "" || evidence.Proof == "" {
		return ApplyResult{}, errors.New("human approval evidence requires authority, nonce, and proof")
	}
	destination, err := filepath.Abs(currentPath)
	if err != nil {
		return ApplyResult{}, err
	}
	contractHash, err := ContentHashChecked(draft)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("atomic spatial approval canonical contract hash: %w", err)
	}
	verification := ApprovalVerification{
		Action:                 ApprovalActionApproveApply,
		ContractHash:           contractHash,
		CurrentHash:            expectedCurrentHash,
		CaptureSetHash:         captureHash(draft),
		Reviewer:               reviewer,
		Destination:            filepath.Clean(destination),
		SubjectGeometryHash:    geometry.SubjectGeometryHash,
		TargetGeometryHash:     geometry.TargetGeometryHash,
		DependencyDestinations: append([]string(nil), geometry.DependencyDestinations...),
		Evidence:               evidence,
	}
	if err := verifier.VerifyApproval(verification); err != nil {
		return ApplyResult{}, fmt.Errorf("human approval authorization failed: %w", err)
	}

	approved := cloneContract(draft)
	if err := recordReview(&approved, StateApproved, reviewer, nil, ""); err != nil {
		return ApplyResult{}, err
	}
	result, err := prepareApprovedWrite(projectRoot, currentPath, draftPath, approved)
	if err != nil {
		return ApplyResult{}, err
	}
	var applied ApplyResult
	if err := consumer.ConsumeApprovalGrant(verification, func() error {
		// The grant is already consumed when this callback runs. Re-check the
		// exact diff baseline immediately before mutation so a concurrent edit
		// cannot be silently overwritten or retried with the same grant.
		if geometry.RevalidateCurrent != nil {
			if geometryErr := geometry.RevalidateCurrent(); geometryErr != nil {
				return geometryErr
			}
		}
		if baselineErr := verifyCurrentBaseline(currentPath, draft, expectedCurrentHash); baselineErr != nil {
			return baselineErr
		}
		if !result.Changed {
			result.Status = "UNCHANGED"
			applied = result
			return nil
		}
		var writeErr error
		applied, writeErr = writeAppliedContract(result, projectRoot, currentPath, approved, expectedCurrentHash)
		return writeErr
	}); err != nil {
		return ApplyResult{}, fmt.Errorf("atomic spatial approval failed: %w", err)
	}
	return applied, nil
}

func normalizeCurrentHash(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == CurrentHashAbsent {
		return value, nil
	}
	if len(value) != sha256.Size*2 {
		return "", errors.New("current_hash must be a SHA-256 value or the absent sentinel")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", errors.New("current_hash must be a SHA-256 value or the absent sentinel")
	}
	return value, nil
}

func sha256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// currentBaseline returns a raw-file digest so formatting-only or review-only
// edits are still protected by the approval compare-and-swap. A missing file
// is represented by CurrentHashAbsent rather than an empty, unsigned value.
func currentBaseline(path string, expectedIdentity Contract) (string, *Contract, error) {
	before, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return CurrentHashAbsent, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	if !before.Mode().IsRegular() {
		return "", nil, errors.New("current spatial contract is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return "", nil, errors.New("current spatial contract changed while reading its baseline")
	}
	current, err := Decode(data)
	if err != nil {
		return "", nil, err
	}
	if !sameIdentity(current, expectedIdentity) {
		return "", nil, errors.New("spatial apply current and draft contract identities do not match")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), &current, nil
}

func verifyCurrentBaseline(path string, expectedIdentity Contract, expectedHash string) error {
	actual, _, err := currentBaseline(path, expectedIdentity)
	if err != nil {
		return fmt.Errorf("APPLY_SOURCE_CHANGED inspect current contract: %w", err)
	}
	if actual != expectedHash {
		return fmt.Errorf("APPLY_SOURCE_CHANGED current_hash mismatch got=%s want=%s", actual, expectedHash)
	}
	return nil
}

func prepareApply(currentPath, draftPath string) (ApplyResult, Contract, error) {
	draft, err := Load(draftPath)
	if err != nil {
		return ApplyResult{}, Contract{}, err
	}
	if draft.State != StateApproved {
		return ApplyResult{}, Contract{}, errors.New("spatial apply requires an Approved human-reviewed draft")
	}
	projectRoot, err := inferProjectRoot(currentPath)
	if err != nil {
		return ApplyResult{}, Contract{}, err
	}
	if err := validateApplyIdentity(projectRoot, currentPath, draft); err != nil {
		return ApplyResult{}, Contract{}, err
	}
	diff, err := Diff(currentPath, draftPath)
	if err != nil {
		return ApplyResult{}, Contract{}, err
	}
	result := ApplyResult{Status: "DRY_RUN", Current: currentPath, Draft: draftPath, ContractHash: diff.ContractHash, Changed: diff.Changed, Verified: true}
	return result, draft, nil
}

func prepareApprovedWrite(projectRoot, currentPath, draftPath string, approved Contract) (ApplyResult, error) {
	if err := validateCanonicalDestination(projectRoot, currentPath, approved); err != nil {
		return ApplyResult{}, err
	}
	changed := true
	status := "WRITE"
	if current, err := Load(currentPath); err == nil {
		changed = len(changedFields(current, approved)) != 0
		if !changed {
			status = "UNCHANGED"
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ApplyResult{}, err
	}
	contractHash, err := ContentHashChecked(approved)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Status: status, Current: currentPath, Draft: draftPath, ContractHash: contractHash, Changed: changed, Verified: true}, nil
}

func validateApplyIdentity(projectRoot, currentPath string, draft Contract) error {
	if err := validateCanonicalDestination(projectRoot, currentPath, draft); err != nil {
		return err
	}
	current, err := Load(currentPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !sameIdentity(current, draft) {
		return errors.New("spatial apply current and draft contract identities do not match")
	}
	return nil
}

func validateCanonicalDestination(projectRoot, path string, contract Contract) error {
	expected, err := CanonicalContractPath(projectRoot, contract)
	if err != nil {
		return err
	}
	actual, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !sameFilesystemPath(expected, actual) {
		return fmt.Errorf("spatial apply destination does not match canonical contract identity: got=%s want=%s", filepath.Clean(actual), filepath.Clean(expected))
	}
	if err := validateDestinationAncestor(projectRoot, actual); err != nil {
		return err
	}
	return nil
}

// CanonicalContractPath returns the sole tracked destination for a contract.
// Interaction target and relation values are UTF-8 hex encoded so filenames
// are lossless, separator-safe, and collision-free on case-insensitive filesystems.
func CanonicalContractPath(projectRoot string, contract Contract) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(projectRoot))
	if err != nil || strings.TrimSpace(projectRoot) == "" {
		return "", errors.New("spatial apply requires an explicit project root")
	}
	var relative string
	switch contract.ContractType {
	case TypeAsset:
		if contract.Asset == nil || !guidPattern.MatchString(contract.Asset.AssetGUID) {
			return "", errors.New("spatial apply requires a valid asset contract identity")
		}
		relative = filepath.Join("Assets", "SpatialContracts", "Assets", strings.ToLower(contract.Asset.AssetGUID)+".spatial.json")
	case TypeInteraction:
		if contract.Interaction == nil || !guidPattern.MatchString(contract.Interaction.SubjectGUID) || contract.Interaction.TargetKey == "" || contract.Interaction.Relation == "" {
			return "", errors.New("spatial apply requires a valid interaction contract identity")
		}
		if len([]byte(contract.Interaction.TargetKey)) > 80 {
			return "", errors.New("spatial apply interaction target_key exceeds the canonical filename limit")
		}
		name := strings.ToLower(contract.Interaction.SubjectGUID) + "__" +
			hex.EncodeToString([]byte(contract.Interaction.TargetKey)) + "__" +
			hex.EncodeToString([]byte(contract.Interaction.Relation)) + ".interaction.json"
		relative = filepath.Join("Assets", "SpatialContracts", "Interactions", name)
	default:
		return "", errors.New("spatial apply requires an asset or interaction contract")
	}
	expected := filepath.Clean(filepath.Join(root, relative))
	if !pathWithin(root, expected) {
		return "", errors.New("canonical spatial contract destination escapes the project root")
	}
	return expected, nil
}

// InteractionAssetGUIDs narrows v1 approval to the only dependency shape for
// which both geometry revisions can currently be proven: SupportedBy between
// two asset contracts.
func InteractionAssetGUIDs(contract Contract) (string, string, error) {
	if contract.ContractType != TypeInteraction || contract.Interaction == nil || contract.Interaction.Relation != "SupportedBy" {
		return "", "", errors.New("interaction approval currently supports only SupportedBy asset relationships")
	}
	subject := strings.ToLower(strings.TrimSpace(contract.Interaction.SubjectGUID))
	targetKey := strings.TrimSpace(contract.Interaction.TargetKey)
	if !guidPattern.MatchString(subject) || !strings.HasPrefix(targetKey, "asset:") {
		return "", "", errors.New("interaction approval currently requires subject_guid and target_key asset:<guid>")
	}
	target := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(targetKey, "asset:")))
	if !guidPattern.MatchString(target) {
		return "", "", errors.New("interaction approval target_key must be asset:<guid>")
	}
	return subject, target, nil
}

// ProjectRootForCanonicalContractPath recovers a project root only when path
// is the exact canonical destination for the validated contract identity.
func ProjectRootForCanonicalContractPath(path string, contract Contract) (string, error) {
	root, err := inferProjectRoot(path)
	if err != nil {
		return "", err
	}
	expected, err := CanonicalContractPath(root, contract)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil || !sameFilesystemPath(expected, absolute) {
		return "", errors.New("spatial contract path does not match its canonical contract identity")
	}
	return root, nil
}

func inferProjectRoot(contractPath string) (string, error) {
	absolute, err := filepath.Abs(contractPath)
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(absolute)
	for {
		assets := filepath.Join(directory, "Assets")
		if pathWithin(assets, absolute) {
			relative, relErr := filepath.Rel(assets, absolute)
			parts := strings.Split(filepath.ToSlash(relative), "/")
			if relErr == nil && len(parts) == 3 && strings.EqualFold(parts[0], "SpatialContracts") &&
				(strings.EqualFold(parts[1], "Assets") || strings.EqualFold(parts[1], "Interactions")) {
				return directory, nil
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return "", errors.New("spatial apply destination must be under the explicit project's Assets/SpatialContracts tree")
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

func sameFilesystemPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func validateDestinationAncestor(projectRoot, destination string) error {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, filepath.Clean(destination))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("spatial apply destination escapes the project root")
	}
	current := root
	parts := strings.Split(relative, string(filepath.Separator))
	for index := -1; index < len(parts); index++ {
		if index >= 0 {
			current = filepath.Join(current, parts[index])
		}
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) {
			// Once a component does not exist, later components cannot contain a
			// pre-existing reparse point. MkdirAll will create ordinary directories.
			break
		}
		if statErr != nil {
			return fmt.Errorf("inspect spatial apply destination component: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("spatial apply destination contains a symlink or junction under the project root")
		}
	}
	return nil
}

func sameIdentity(current, draft Contract) bool {
	if current.ContractType != draft.ContractType {
		return false
	}
	switch draft.ContractType {
	case TypeAsset:
		return current.Asset != nil && draft.Asset != nil && current.Asset.AssetGUID == draft.Asset.AssetGUID
	case TypeInteraction:
		return current.Interaction != nil && draft.Interaction != nil &&
			current.Interaction.SubjectGUID == draft.Interaction.SubjectGUID &&
			current.Interaction.TargetKey == draft.Interaction.TargetKey &&
			current.Interaction.Relation == draft.Interaction.Relation
	default:
		return false
	}
}

type appliedWriteHooks struct {
	afterEvacuate func(currentPath, backupPath string) error
	beforePublish func(currentPath string) error
	syncDirectory func(string) error
}

func writeAppliedContract(result ApplyResult, projectRoot, currentPath string, draft Contract, expectedCurrentHash string) (ApplyResult, error) {
	return writeAppliedContractWithHooks(result, projectRoot, currentPath, draft, expectedCurrentHash, appliedWriteHooks{})
}

// writeAppliedContractWithHooks performs a raw-file compare-and-swap. The
// signed baseline is checked only after the old file has been atomically moved
// out of the destination, and the approved bytes are published with a hard
// link so an uncooperative workspace writer can never be overwritten. Hooks
// exist only so tests can deterministically exercise the two race windows.
func writeAppliedContractWithHooks(result ApplyResult, projectRoot, currentPath string, draft Contract, expectedCurrentHash string, hooks appliedWriteHooks) (ApplyResult, error) {
	data, err := encode(draft)
	if err != nil {
		return ApplyResult{}, err
	}
	directory := filepath.Dir(currentPath)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return ApplyResult{}, err
	}
	if err := validateCanonicalDestination(projectRoot, currentPath, draft); err != nil {
		return ApplyResult{}, err
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil || !directoryInfo.IsDir() {
		return ApplyResult{}, fmt.Errorf("inspect spatial apply destination directory: %w", err)
	}

	temp, err := os.CreateTemp(directory, ".spatial-contract-*.tmp")
	if err != nil {
		return ApplyResult{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return ApplyResult{}, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return ApplyResult{}, err
	}
	if err := temp.Close(); err != nil {
		return ApplyResult{}, err
	}
	tempInfo, err := os.Lstat(tempPath)
	if err != nil || !tempInfo.Mode().IsRegular() {
		return ApplyResult{}, errors.New("spatial apply staged contract is not a regular file")
	}
	syncDirectory := hooks.syncDirectory
	if syncDirectory == nil {
		syncDirectory = syncSpatialDirectory
	}
	if err := syncDirectory(directory); err != nil {
		return ApplyResult{}, fmt.Errorf("sync spatial apply staged contract directory: %w", err)
	}

	restoreBackup := func(backup string) error {
		if backup == "" {
			return nil
		}
		if err := os.Link(backup, currentPath); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return errors.New("destination was replaced while restoring the signed baseline")
			}
			return fmt.Errorf("restore signed spatial baseline without replacement: %w", err)
		}
		if err := syncDirectory(directory); err != nil {
			return fmt.Errorf("sync restored spatial baseline: %w", err)
		}
		return nil
	}

	backup := ""
	if expectedCurrentHash != CurrentHashAbsent {
		reserved, reserveErr := os.CreateTemp(directory, "."+filepath.Base(currentPath)+".bak-*")
		if reserveErr != nil {
			return ApplyResult{}, reserveErr
		}
		backup = reserved.Name()
		if closeErr := reserved.Close(); closeErr != nil {
			_ = os.Remove(backup)
			return ApplyResult{}, closeErr
		}
		if removeErr := os.Remove(backup); removeErr != nil {
			return ApplyResult{}, removeErr
		}
		if err := os.Rename(currentPath, backup); err != nil {
			return ApplyResult{}, fmt.Errorf("APPLY_SOURCE_CHANGED claim current contract: %w", err)
		}
		result.Backup = backup
		if err := syncDirectory(directory); err != nil {
			restoreErr := restoreBackup(backup)
			return ApplyResult{}, fmt.Errorf("spatial apply backup durability is uncertain (backup=%s restore=%v): %w", backup, restoreErr, err)
		}
		actual, _, baselineErr := currentBaseline(backup, draft)
		if baselineErr != nil || actual != expectedCurrentHash {
			restoreErr := restoreBackup(backup)
			if baselineErr != nil {
				return ApplyResult{}, fmt.Errorf("APPLY_SOURCE_CHANGED claimed current contract is invalid (backup=%s restore=%v): %w", backup, restoreErr, baselineErr)
			}
			return ApplyResult{}, fmt.Errorf("APPLY_SOURCE_CHANGED claimed current_hash mismatch got=%s want=%s backup=%s restore=%v", actual, expectedCurrentHash, backup, restoreErr)
		}
		if hooks.afterEvacuate != nil {
			if err := hooks.afterEvacuate(currentPath, backup); err != nil {
				restoreErr := restoreBackup(backup)
				return ApplyResult{}, fmt.Errorf("spatial apply test hook failed (restore=%v): %w", restoreErr, err)
			}
		}
	} else if _, err := os.Lstat(currentPath); err == nil {
		return ApplyResult{}, errors.New("APPLY_SOURCE_CHANGED expected an absent current contract")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ApplyResult{}, fmt.Errorf("APPLY_SOURCE_CHANGED inspect absent current contract: %w", err)
	}

	if hooks.beforePublish != nil {
		if err := hooks.beforePublish(currentPath); err != nil {
			restoreErr := restoreBackup(backup)
			return ApplyResult{}, fmt.Errorf("spatial apply test hook failed (restore=%v): %w", restoreErr, err)
		}
	}
	if err := validateCanonicalDestination(projectRoot, currentPath, draft); err != nil {
		restoreErr := restoreBackup(backup)
		return ApplyResult{}, fmt.Errorf("spatial apply destination changed before publish (backup=%s restore=%v): %w", backup, restoreErr, err)
	}
	currentDirectoryInfo, err := os.Stat(directory)
	if err != nil || !os.SameFile(directoryInfo, currentDirectoryInfo) {
		restoreErr := restoreBackup(backup)
		return ApplyResult{}, fmt.Errorf("spatial apply destination directory changed before publish (backup=%s restore=%v)", backup, restoreErr)
	}
	if err := os.Link(tempPath, currentPath); err != nil {
		restoreErr := restoreBackup(backup)
		if errors.Is(err, fs.ErrExist) {
			return ApplyResult{}, fmt.Errorf("APPLY_SOURCE_CHANGED destination was created before publish (backup=%s restore=%v)", backup, restoreErr)
		}
		return ApplyResult{}, fmt.Errorf("publish spatial contract without replacement (backup=%s restore=%v): %w", backup, restoreErr, err)
	}
	if err := syncDirectory(directory); err != nil {
		return ApplyResult{}, fmt.Errorf("spatial apply was published but directory durability is uncertain (backup=%s): %w", backup, err)
	}

	publishedInfo, err := os.Lstat(currentPath)
	if err != nil || !publishedInfo.Mode().IsRegular() || !os.SameFile(tempInfo, publishedInfo) {
		return ApplyResult{}, fmt.Errorf("spatial apply verification failed: published file identity changed (backup=%s)", backup)
	}
	publishedData, err := os.ReadFile(currentPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("spatial apply verification failed (backup=%s): %w", backup, err)
	}
	if !bytes.Equal(publishedData, data) {
		return ApplyResult{}, fmt.Errorf("spatial apply verification failed: published bytes changed (backup=%s)", backup)
	}
	verified, verifyErr := Decode(publishedData)
	verifiedHash := ""
	if verifyErr == nil {
		verifiedHash, verifyErr = ContentHashChecked(verified)
	}
	if verifyErr != nil || verifiedHash != result.ContractHash {
		if verifyErr != nil {
			return ApplyResult{}, fmt.Errorf("spatial apply verification failed (backup=%s): %w", backup, verifyErr)
		}
		return ApplyResult{}, fmt.Errorf("spatial apply verification failed: contract hash mismatch (backup=%s)", backup)
	}
	if err := validateCanonicalDestination(projectRoot, currentPath, verified); err != nil {
		return ApplyResult{}, fmt.Errorf("spatial apply destination changed after publish (backup=%s): %w", backup, err)
	}
	if currentDirectoryInfo, err = os.Stat(directory); err != nil || !os.SameFile(directoryInfo, currentDirectoryInfo) {
		return ApplyResult{}, fmt.Errorf("spatial apply destination directory changed after publish (backup=%s)", backup)
	}
	result.Status = "WRITE"
	result.Written = true
	return result, nil
}

func syncSpatialDirectory(path string) error {
	return durablefs.SyncDirectory(path)
}

func OverlayApprovedAssets(manifest *bounds.Manifest, root string) (int, error) {
	return OverlayApprovedAssetsWithPolicy(manifest, root, OverlayPolicy{})
}

// OverlayApprovedAssetsWithPolicy overlays only contracts whose approval is
// verified by the local bridge's external ledger. Legacy tracked approval JSON
// is deliberately not treated as authority and must be reviewed again.
func OverlayApprovedAssetsWithPolicy(manifest *bounds.Manifest, root string, policy OverlayPolicy) (int, error) {
	if manifest == nil || strings.TrimSpace(root) == "" {
		return 0, nil
	}
	contracts := make(map[string]Contract)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".spatial.json") {
			return nil
		}
		contract, err := Load(path)
		if err != nil {
			return err
		}
		if contract.ContractType == TypeAsset && contract.State == StateApproved {
			if _, exists := contracts[contract.Asset.AssetPath]; exists {
				return fmt.Errorf("duplicate approved spatial contracts for asset path %s", contract.Asset.AssetPath)
			}
			if policy.Verifier == nil {
				return fmt.Errorf("approved spatial contract %s requires external approval-ledger verification", path)
			} else {
				verification, err := ApprovedVerification(contract, path)
				if err != nil {
					return err
				}
				if err := policy.Verifier.VerifyApprovedContract(verification); err != nil {
					return fmt.Errorf("approved spatial contract %s is not authorized for consumption: %w", path, err)
				}
			}
			contracts[contract.Asset.AssetPath] = contract
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	applied := 0
	for i := range manifest.Prefabs {
		prefab := &manifest.Prefabs[i]
		contract, ok := contracts[prefab.Path]
		if !ok {
			continue
		}
		asset := contract.Asset
		if prefab.GUID == "" || !strings.EqualFold(prefab.GUID, asset.AssetGUID) {
			continue
		}
		if prefab.Spatial == nil || prefab.Spatial.DependencyHash == "" || asset.DependencyHash != prefab.Spatial.DependencyHash {
			continue
		}
		profile := &bounds.SpatialProfile{
			Forward:        bounds.Vec3(asset.Forward),
			Up:             bounds.Vec3(asset.Up),
			PivotOffset:    bounds.Vec3(asset.PivotOffset),
			Source:         "spatial-contract",
			Confidence:     1,
			Reviewed:       true,
			DependencyHash: asset.DependencyHash,
		}
		for _, proxy := range asset.CollisionProxies {
			profile.OBBs = append(profile.OBBs, bounds.OBB{ID: proxy.ID, Center: bounds.Vec3(proxy.Center), Size: bounds.Vec3(proxy.Size), Rotation: bounds.Quat(proxy.Rotation)})
		}
		for _, frame := range asset.Frames {
			converted := &bounds.ContactFrame{ID: frame.ID, Point: bounds.Vec3(frame.Point), Normal: bounds.Vec3(frame.Normal), Tangent: bounds.Vec3(frame.Tangent), Size: frame.Size}
			profile.Frames = append(profile.Frames, *converted)
			switch frame.ID {
			case "bottom":
				profile.BottomContact = converted
			case "back":
				profile.BackContact = converted
			case "top":
				profile.TopContact = converted
			}
		}
		for _, requirement := range asset.Contacts {
			profile.Contacts = append(profile.Contacts, bounds.ContactRequirement{
				ID: requirement.ID, Kind: requirement.Kind, FrameID: requirement.FrameID,
				Target: requirement.Target, MinimumGap: requirement.MinimumGap,
				MaximumGap: requirement.MaximumGap, MaximumPenetration: requirement.MaximumPenetration,
				MinimumSupport: requirement.MinimumSupport, DirectionAlignment: requirement.DirectionAlignment,
			})
		}
		prefab.Spatial = profile
		applied++
	}
	return applied, nil
}

func validateAsset(asset AssetSpatialContract) error {
	if !guidPattern.MatchString(asset.AssetGUID) {
		return errors.New("invalid spatial contract: asset.asset_guid must be 32 hexadecimal characters")
	}
	if !strings.HasPrefix(asset.AssetPath, "Assets/") {
		return errors.New("invalid spatial contract: asset.asset_path must be under Assets/")
	}
	if strings.TrimSpace(asset.DependencyHash) == "" {
		return errors.New("invalid spatial contract: asset.dependency_hash is required")
	}
	if asset.Units != "meter" {
		return errors.New("invalid spatial contract: asset.units must be meter")
	}
	if !unitVector(asset.Forward) || !unitVector(asset.Up) || math.Abs(dot(asset.Forward, asset.Up)) > 0.001 {
		return errors.New("invalid spatial contract: asset forward/up must be normalized and orthogonal")
	}
	if !finiteVec(asset.PivotOffset) {
		return errors.New("invalid spatial contract: asset pivot_offset must be finite")
	}
	if asset.Revision < 1 || len(asset.CollisionProxies) == 0 || len(asset.Frames) == 0 || len(asset.Contacts) == 0 {
		return errors.New("invalid spatial contract: asset requires revision, collision proxies, frames, and contacts")
	}
	if err := validateOBBs(asset.CollisionProxies, "collision_proxies"); err != nil {
		return err
	}
	if err := validateOBBs(asset.ClearanceProxies, "clearance_proxies"); err != nil {
		return err
	}
	frameIDs := map[string]bool{}
	for _, frame := range asset.Frames {
		if strings.TrimSpace(frame.ID) == "" || frameIDs[frame.ID] {
			return errors.New("invalid spatial contract: frame IDs must be non-empty and unique")
		}
		frameIDs[frame.ID] = true
		if !finiteVec(frame.Point) || !unitVector(frame.Normal) || !unitVector(frame.Tangent) || math.Abs(dot(frame.Normal, frame.Tangent)) > 0.001 || !finite(frame.Size[0]) || !finite(frame.Size[1]) || frame.Size[0] <= 0 || frame.Size[1] <= 0 {
			return fmt.Errorf("invalid spatial contract: frame %q has invalid basis or size", frame.ID)
		}
	}
	contactIDs := map[string]bool{}
	for _, contact := range asset.Contacts {
		if strings.TrimSpace(contact.ID) == "" || contactIDs[contact.ID] {
			return errors.New("invalid spatial contract: contact IDs must be non-empty and unique")
		}
		contactIDs[contact.ID] = true
		if !frameIDs[contact.FrameID] {
			return fmt.Errorf("invalid spatial contract: contact %q references missing frame %q", contact.ID, contact.FrameID)
		}
		if !validContactKind(contact.Kind) || strings.TrimSpace(contact.Target) == "" {
			return fmt.Errorf("invalid spatial contract: contact %q has unsupported kind or target", contact.ID)
		}
		if !finite(contact.MinimumGap) || !finite(contact.MaximumGap) || !finite(contact.MaximumPenetration) || !finite(contact.MinimumSupport) || !finite(contact.DirectionAlignment) || contact.MinimumGap < 0 || contact.MaximumGap < contact.MinimumGap || contact.MaximumPenetration < 0 || contact.MinimumSupport < 0 || contact.MinimumSupport > 1 || contact.DirectionAlignment < 0 || contact.DirectionAlignment > 1 {
			return fmt.Errorf("invalid spatial contract: contact %q has invalid tolerances", contact.ID)
		}
	}
	expectedHash, err := assetHashChecked(asset)
	if err != nil {
		return fmt.Errorf("invalid spatial contract: canonical asset hash: %w", err)
	}
	if !sha256Hex(asset.GeometryHash) || asset.GeometryHash != expectedHash {
		return errors.New("invalid spatial contract: asset.geometry_hash does not match geometry")
	}
	if strings.TrimSpace(asset.CaptureSetHash) == "" {
		return errors.New("invalid spatial contract: asset.capture_set_hash is required")
	}
	return nil
}

func validateInteraction(interaction InteractionContract) error {
	if !guidPattern.MatchString(interaction.SubjectGUID) || strings.TrimSpace(interaction.TargetKey) == "" {
		return errors.New("invalid spatial contract: interaction subject_guid and target_key are required")
	}
	if interaction.Relation != "SupportedBy" {
		return errors.New("invalid spatial contract: v1 interaction relation must be SupportedBy")
	}
	if strings.TrimSpace(interaction.SubjectFrame) == "" || strings.TrimSpace(interaction.TargetFrame) == "" || !unitQuat(interaction.RelativeRotation) {
		return errors.New("invalid spatial contract: interaction frames and normalized relative rotation are required")
	}
	if !finiteVec(interaction.RelativePosition) {
		return errors.New("invalid spatial contract: interaction relative_position must be finite")
	}
	for _, value := range interaction.PositionTolerance {
		if value < 0 || !finite(value) {
			return errors.New("invalid spatial contract: interaction position_tolerance must be finite and >= 0")
		}
	}
	if !finite(interaction.AngleTolerance) || interaction.AngleTolerance < 0 || interaction.AngleTolerance > 180 || interaction.CollisionPolicy == "" || interaction.Revision < 1 || interaction.CaptureSetHash == "" {
		return errors.New("invalid spatial contract: interaction tolerances, collision_policy, revision, and capture_set_hash are required")
	}
	expectedHash, err := interactionHashChecked(interaction)
	if err != nil {
		return fmt.Errorf("invalid spatial contract: canonical interaction hash: %w", err)
	}
	if !sha256Hex(interaction.InteractionHash) || interaction.InteractionHash != expectedHash {
		return errors.New("invalid spatial contract: interaction.interaction_hash does not match interaction")
	}
	return nil
}

func encode(contract Contract) ([]byte, error) {
	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func assetHash(asset AssetSpatialContract) string {
	hash, _ := assetHashChecked(asset)
	return hash
}

func assetHashChecked(asset AssetSpatialContract) (string, error) {
	asset.GeometryHash = ""
	return canonicalHash(asset)
}

func interactionHash(interaction InteractionContract) string {
	hash, _ := interactionHashChecked(interaction)
	return hash
}

func interactionHashChecked(interaction InteractionContract) (string, error) {
	interaction.InteractionHash = ""
	return canonicalHash(interaction)
}

func captureHash(contract Contract) string {
	if contract.Asset != nil {
		return contract.Asset.CaptureSetHash
	}
	if contract.Interaction != nil {
		return contract.Interaction.CaptureSetHash
	}
	return ""
}

func changedFields(current, draft Contract) []string {
	fields := make([]string, 0, 5)
	if current.ContractType != draft.ContractType {
		fields = append(fields, "contract_type")
	}
	if current.State != draft.State {
		fields = append(fields, "state")
	}
	if !reflect.DeepEqual(current.Asset, draft.Asset) {
		fields = append(fields, "asset")
	}
	if !reflect.DeepEqual(current.Interaction, draft.Interaction) {
		fields = append(fields, "interaction")
	}
	if !reflect.DeepEqual(current.Technical, draft.Technical) {
		fields = append(fields, "technical")
	}
	if !reflect.DeepEqual(current.Review, draft.Review) {
		fields = append(fields, "review")
	}
	return fields
}

func validState(state string) bool {
	switch state {
	case StateDraft, StateTechnicalPassed, StateAwaitingHumanReview, StateApproved, StateTechnicalFailed, StateRevisionRequested, StateUnableToJudge, StateStale:
		return true
	default:
		return false
	}
}

func validContactKind(kind string) bool {
	switch kind {
	case "WallMounted", "WallBacked", "FloorSupported", "CeilingMounted", "SupportedBy":
		return true
	default:
		return false
	}
}

func validateOBBs(boxes []OBB, field string) error {
	seen := map[string]bool{}
	for _, box := range boxes {
		if strings.TrimSpace(box.ID) == "" || seen[box.ID] {
			return fmt.Errorf("invalid spatial contract: %s IDs must be non-empty and unique", field)
		}
		seen[box.ID] = true
		if !finiteVec(box.Center) {
			return fmt.Errorf("invalid spatial contract: %s[%s] center must be finite", field, box.ID)
		}
		for _, value := range box.Size {
			if value <= 0 || !finite(value) {
				return fmt.Errorf("invalid spatial contract: %s[%s] size must be finite and > 0", field, box.ID)
			}
		}
		if !unitQuat(box.Rotation) {
			return fmt.Errorf("invalid spatial contract: %s[%s] rotation must be normalized", field, box.ID)
		}
	}
	return nil
}

func normalizeOBBs(boxes []OBB) {
	for i := range boxes {
		normalizeVec(&boxes[i].Center)
		normalizeVec(&boxes[i].Size)
		normalizeQuat(&boxes[i].Rotation)
	}
}

func normalizeVec(value *Vec3) {
	for i := range value {
		value[i] = round(value[i])
	}
}

func normalizeQuat(value *Quat) {
	for i := range value {
		value[i] = round(value[i])
	}
}

func round(value float64) float64 {
	// Unity stores Spatial Contract geometry as System.Single. Convert first so
	// values such as 0.5500005 normalize identically before the shared six-place
	// MidpointRounding.AwayFromZero step.
	value = float64(float32(value))
	return math.Round(value*1_000_000) / 1_000_000
}

func canonicalHash(value any) (string, error) {
	data, err := marshalCanonical(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func marshalCanonical(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	// SpatialContractHashUtility writes '<', '>' and '&' literally. Go's
	// default HTML escaping would otherwise produce a different C#/Go hash.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return restoreUnityLineSeparators(bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})), nil
}

// utf16OrdinalLess mirrors C# StringComparer.Ordinal. Comparing Go strings
// directly uses Unicode code-point order, which differs from UTF-16 code-unit
// order when a supplementary-plane ID is compared with a BMP ID.
func utf16OrdinalLess(left, right string) bool {
	a := utf16.Encode([]rune(left))
	b := utf16.Encode([]rune(right))
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for index := 0; index < limit; index++ {
		if a[index] != b[index] {
			return a[index] < b[index]
		}
	}
	return len(a) < len(b)
}

// encoding/json always escapes U+2028 and U+2029, even with HTML escaping
// disabled. Unity's canonical writer emits those characters literally. Only
// replace an actual JSON Unicode escape (an odd-length backslash run), never a
// user's literal "\\u2028" or "\\u2029" text.
func restoreUnityLineSeparators(data []byte) []byte {
	result := make([]byte, 0, len(data))
	inString := false
	for index := 0; index < len(data); {
		if data[index] == '"' {
			inString = !inString
			result = append(result, data[index])
			index++
			continue
		}
		if !inString || data[index] != '\\' {
			result = append(result, data[index])
			index++
			continue
		}

		runEnd := index
		for runEnd < len(data) && data[runEnd] == '\\' {
			runEnd++
		}
		runLength := runEnd - index
		if runLength%2 == 1 && runEnd+5 <= len(data) && data[runEnd] == 'u' {
			escape := data[runEnd : runEnd+5]
			if bytes.Equal(escape, []byte("u2028")) || bytes.Equal(escape, []byte("u2029")) {
				result = append(result, data[index:runEnd-1]...)
				if escape[4] == '8' {
					result = append(result, []byte("\u2028")...)
				} else {
					result = append(result, []byte("\u2029")...)
				}
				index = runEnd + 5
				continue
			}
		}
		result = append(result, data[index:runEnd]...)
		if runLength%2 == 1 && runEnd < len(data) {
			// The next byte is escaped JSON string content. Copy it here so an
			// escaped quote is not mistaken for the end of the string.
			result = append(result, data[runEnd])
			index = runEnd + 1
		} else {
			index = runEnd
		}
	}
	return result
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func finiteVec(value Vec3) bool {
	return finite(value[0]) && finite(value[1]) && finite(value[2])
}

func unitVector(value Vec3) bool {
	length := dot(value, value)
	return finite(length) && math.Abs(length-1) <= 0.001
}

func dot(a, b Vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func unitQuat(value Quat) bool {
	length := value[0]*value[0] + value[1]*value[1] + value[2]*value[2] + value[3]*value[3]
	return finite(length) && math.Abs(length-1) <= 0.001
}
