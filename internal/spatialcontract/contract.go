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

	"github.com/Kubonsang/unity-ctx/internal/bounds"
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

type HumanReview struct {
	Decision       string   `json:"decision"`
	ContractHash   string   `json:"contract_hash"`
	CaptureSetHash string   `json:"capture_set_hash"`
	Reviewer       string   `json:"reviewer"`
	IssueTypes     []string `json:"issue_types,omitempty"`
	Comment        string   `json:"comment,omitempty"`
	Revision       int      `json:"revision"`
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
	Status       string   `json:"status"`
	Current      string   `json:"current"`
	Draft        string   `json:"draft"`
	ContractHash string   `json:"contract_hash"`
	Changed      bool     `json:"changed"`
	Fields       []string `json:"fields"`
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
		sort.Slice(a.CollisionProxies, func(i, j int) bool { return a.CollisionProxies[i].ID < a.CollisionProxies[j].ID })
		sort.Slice(a.ClearanceProxies, func(i, j int) bool { return a.ClearanceProxies[i].ID < a.ClearanceProxies[j].ID })
		sort.Slice(a.Frames, func(i, j int) bool { return a.Frames[i].ID < a.Frames[j].ID })
		sort.Slice(a.Contacts, func(i, j int) bool { return a.Contacts[i].ID < a.Contacts[j].ID })
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
	if contract.Review != nil {
		sort.Strings(contract.Review.IssueTypes)
		if contract.Review.Revision < 1 {
			contract.Review.Revision = 1
		}
	}
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
		if contract.Technical.Passed && contract.Technical.ErrorCount != 0 {
			return errors.New("invalid spatial contract: passed technical evidence requires error_count=0")
		}
	}
	if contract.State == StateApproved {
		if contract.Technical == nil || !contract.Technical.Passed || contract.Technical.ErrorCount != 0 {
			return errors.New("invalid spatial contract: Approved requires passed technical evidence with zero errors")
		}
		if contract.Review == nil || contract.Review.Decision != StateApproved {
			return errors.New("invalid spatial contract: Approved requires an Approved human review")
		}
		hash := ContentHash(contract)
		if contract.Review.ContractHash != hash {
			return fmt.Errorf("invalid spatial contract: review contract_hash is stale got=%s want=%s", contract.Review.ContractHash, hash)
		}
		if contract.Review.CaptureSetHash == "" || contract.Review.CaptureSetHash != captureHash(contract) {
			return errors.New("invalid spatial contract: review capture_set_hash is stale")
		}
		if strings.TrimSpace(contract.Review.Reviewer) == "" {
			return errors.New("invalid spatial contract: Approved requires review.reviewer")
		}
	}
	return nil
}

func Approve(contract *Contract, reviewer string) error {
	return Review(contract, StateApproved, reviewer, nil, "")
}

func Review(contract *Contract, decision, reviewer string, issues []string, comment string) error {
	if contract == nil {
		return errors.New("contract is nil")
	}
	Normalize(contract)
	if decision != StateApproved && decision != StateRevisionRequested && decision != StateUnableToJudge {
		return errors.New("human review decision must be Approved, RevisionRequested, or UnableToJudge")
	}
	if strings.TrimSpace(reviewer) == "" {
		return errors.New("human review requires reviewer")
	}
	if decision == StateApproved && (contract.Technical == nil || !contract.Technical.Passed || contract.Technical.ErrorCount != 0) {
		return errors.New("technical validation must pass before human approval")
	}
	capture := captureHash(*contract)
	if capture == "" {
		return errors.New("capture_set_hash is required before human approval")
	}
	contract.State = decision
	contract.Review = &HumanReview{
		Decision:       decision,
		ContractHash:   ContentHash(*contract),
		CaptureSetHash: capture,
		Reviewer:       strings.TrimSpace(reviewer),
		IssueTypes:     append([]string(nil), issues...),
		Comment:        strings.TrimSpace(comment),
		Revision:       1,
	}
	return Validate(*contract)
}

func ContentHash(contract Contract) string {
	Normalize(&contract)
	contract.State = ""
	contract.Review = nil
	data, _ := json.Marshal(contract)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func Diff(currentPath, draftPath string) (DiffResult, error) {
	draft, err := Load(draftPath)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{Status: "OK", Current: currentPath, Draft: draftPath, ContractHash: ContentHash(draft)}
	current, err := Load(currentPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			result.Status = "NEW"
			result.Changed = true
			result.Fields = []string{"contract"}
			return result, nil
		}
		return DiffResult{}, err
	}
	result.Fields = changedFields(current, draft)
	result.Changed = len(result.Fields) > 0
	if !result.Changed {
		result.Status = "UNCHANGED"
	}
	return result, nil
}

func Apply(currentPath, draftPath string, write bool) (ApplyResult, error) {
	diff, err := Diff(currentPath, draftPath)
	if err != nil {
		return ApplyResult{}, err
	}
	draft, err := Load(draftPath)
	if err != nil {
		return ApplyResult{}, err
	}
	if draft.State != StateApproved {
		return ApplyResult{}, errors.New("spatial apply requires an Approved human-reviewed draft")
	}
	result := ApplyResult{Status: "DRY_RUN", Current: currentPath, Draft: draftPath, ContractHash: diff.ContractHash, Changed: diff.Changed, Verified: true}
	if !write || !diff.Changed {
		return result, nil
	}
	data, err := encode(draft)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		return ApplyResult{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(currentPath), ".spatial-contract-*.tmp")
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
	backup := currentPath + ".bak"
	if _, err := os.Stat(currentPath); err == nil {
		_ = os.Remove(backup)
		if err := os.Rename(currentPath, backup); err != nil {
			return ApplyResult{}, err
		}
		result.Backup = backup
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ApplyResult{}, err
	}
	if err := os.Rename(tempPath, currentPath); err != nil {
		if result.Backup != "" {
			_ = os.Rename(backup, currentPath)
		}
		return ApplyResult{}, err
	}
	verified, verifyErr := Load(currentPath)
	if verifyErr != nil || ContentHash(verified) != result.ContractHash {
		_ = os.Remove(currentPath)
		if result.Backup != "" {
			_ = os.Rename(backup, currentPath)
		}
		if verifyErr != nil {
			return ApplyResult{}, fmt.Errorf("spatial apply verification failed: %w", verifyErr)
		}
		return ApplyResult{}, errors.New("spatial apply verification failed: contract hash mismatch")
	}
	result.Status = "WRITE"
	result.Written = true
	return result, nil
}

func OverlayApprovedAssets(manifest *bounds.Manifest, root string) (int, error) {
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
		if prefab.Spatial != nil && prefab.Spatial.DependencyHash != "" && asset.DependencyHash != prefab.Spatial.DependencyHash {
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
			switch frame.ID {
			case "bottom":
				profile.BottomContact = converted
			case "back":
				profile.BackContact = converted
			}
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
		if !unitVector(frame.Normal) || !unitVector(frame.Tangent) || math.Abs(dot(frame.Normal, frame.Tangent)) > 0.001 || frame.Size[0] <= 0 || frame.Size[1] <= 0 {
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
		if contact.MinimumGap < 0 || contact.MaximumGap < contact.MinimumGap || contact.MaximumPenetration < 0 || contact.MinimumSupport < 0 || contact.MinimumSupport > 1 || contact.DirectionAlignment < 0 || contact.DirectionAlignment > 1 {
			return fmt.Errorf("invalid spatial contract: contact %q has invalid tolerances", contact.ID)
		}
	}
	if asset.GeometryHash != assetHash(asset) {
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
	for _, value := range interaction.PositionTolerance {
		if value < 0 || !finite(value) {
			return errors.New("invalid spatial contract: interaction position_tolerance must be finite and >= 0")
		}
	}
	if interaction.AngleTolerance < 0 || interaction.AngleTolerance > 180 || interaction.CollisionPolicy == "" || interaction.Revision < 1 || interaction.CaptureSetHash == "" {
		return errors.New("invalid spatial contract: interaction tolerances, collision_policy, revision, and capture_set_hash are required")
	}
	if interaction.InteractionHash != interactionHash(interaction) {
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
	asset.GeometryHash = ""
	data, _ := json.Marshal(asset)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func interactionHash(interaction InteractionContract) string {
	interaction.InteractionHash = ""
	data, _ := json.Marshal(interaction)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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
	return math.Round(value*1_000_000) / 1_000_000
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func unitVector(value Vec3) bool {
	length := dot(value, value)
	return finite(length) && math.Abs(length-1) <= 0.001
}

func dot(a, b Vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func unitQuat(value Quat) bool {
	length := value[0]*value[0] + value[1]*value[1] + value[2]*value[2] + value[3]*value[3]
	return finite(length) && math.Abs(length-1) <= 0.001
}
