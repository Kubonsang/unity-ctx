package reviewgrant

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

// InteractionGeometryProvenance is resolved exclusively from canonical,
// ledger-authorized asset contracts. Contract hashes are audit evidence; the
// geometry hashes are the signed staleness gate for an interaction pose.
type InteractionGeometryProvenance struct {
	SubjectGUID         string `json:"subject_guid"`
	TargetGUID          string `json:"target_guid"`
	SubjectContractPath string `json:"subject_contract_path"`
	TargetContractPath  string `json:"target_contract_path"`
	SubjectContractHash string `json:"subject_contract_hash"`
	TargetContractHash  string `json:"target_contract_hash"`
	SubjectGeometryHash string `json:"subject_geometry_hash"`
	TargetGeometryHash  string `json:"target_geometry_hash"`
}

func (value InteractionGeometryProvenance) ApprovalBindings() spatialcontract.ApprovalGeometryBindings {
	return spatialcontract.ApprovalGeometryBindings{
		SubjectGeometryHash:    value.SubjectGeometryHash,
		TargetGeometryHash:     value.TargetGeometryHash,
		DependencyDestinations: []string{value.SubjectContractPath, value.TargetContractPath},
	}
}

// ResolveInteractionGeometry verifies each dependency against its own signed
// durable receipt before exposing geometry hashes to the interaction signer.
// Caller-provided "current" hashes are never accepted as authority.
func (ledger *Ledger) ResolveInteractionGeometry(projectRoot string, interaction spatialcontract.Contract) (InteractionGeometryProvenance, error) {
	if ledger == nil || strings.TrimSpace(ledger.Root) == "" {
		return InteractionGeometryProvenance{}, errors.New("approval ledger root is required")
	}
	subjectGUID, targetGUID, err := spatialcontract.InteractionAssetGUIDs(interaction)
	if err != nil {
		return InteractionGeometryProvenance{}, err
	}
	subject, subjectVerification, err := ledger.loadApprovedAsset(projectRoot, subjectGUID)
	if err != nil {
		return InteractionGeometryProvenance{}, fmt.Errorf("SUPPORT_CONTRACT_STALE: subject asset %s is not currently approved: %w", subjectGUID, err)
	}
	target, targetVerification, err := ledger.loadApprovedAsset(projectRoot, targetGUID)
	if err != nil {
		return InteractionGeometryProvenance{}, fmt.Errorf("SUPPORT_CONTRACT_STALE: target asset %s is not currently approved: %w", targetGUID, err)
	}
	return InteractionGeometryProvenance{
		SubjectGUID:         subjectGUID,
		TargetGUID:          targetGUID,
		SubjectContractPath: subjectVerification.ContractPath,
		TargetContractPath:  targetVerification.ContractPath,
		SubjectContractHash: subjectVerification.ContractHash,
		TargetContractHash:  targetVerification.ContractHash,
		SubjectGeometryHash: subject.Asset.GeometryHash,
		TargetGeometryHash:  target.Asset.GeometryHash,
	}, nil
}

func (ledger *Ledger) loadApprovedAsset(projectRoot, guid string) (spatialcontract.Contract, spatialcontract.ApprovedContractVerification, error) {
	identity := spatialcontract.Contract{
		ContractType: spatialcontract.TypeAsset,
		Asset:        &spatialcontract.AssetSpatialContract{AssetGUID: guid},
	}
	path, err := spatialcontract.CanonicalContractPath(projectRoot, identity)
	if err != nil {
		return spatialcontract.Contract{}, spatialcontract.ApprovedContractVerification{}, err
	}
	contract, err := spatialcontract.Load(filepath.Clean(path))
	if err != nil {
		return spatialcontract.Contract{}, spatialcontract.ApprovedContractVerification{}, err
	}
	if contract.ContractType != spatialcontract.TypeAsset || contract.Asset == nil || !strings.EqualFold(contract.Asset.AssetGUID, guid) {
		return spatialcontract.Contract{}, spatialcontract.ApprovedContractVerification{}, errors.New("canonical dependency is not the requested asset contract")
	}
	verification, err := spatialcontract.ApprovedVerification(contract, path)
	if err != nil {
		return spatialcontract.Contract{}, spatialcontract.ApprovedContractVerification{}, err
	}
	if _, err := ledger.VerifyApprovedContractReceipt(verification); err != nil {
		return spatialcontract.Contract{}, spatialcontract.ApprovedContractVerification{}, err
	}
	return contract, verification, nil
}
