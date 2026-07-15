//go:build linux

package spatialcontract

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApprovalDoesNotRecreateDestinationParentAfterGrantConsumption(t *testing.T) {
	project := t.TempDir()
	contract := validAssetContract()
	draft := filepath.Join(project, "Library", "SpatialDrafts", "banner.json")
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	current, err := CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(current)
	consumer := &testOneShotConsumer{beforeApply: func() {
		if err := os.RemoveAll(parent); err != nil {
			t.Fatalf("remove guarded destination fixture: %v", err)
		}
	}}
	evidence := ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof"}
	result, err := ApproveAndApplyAuthorized(project, current, draft, CurrentHashAbsent, "local-user", evidence, testApprovalVerifier{}, consumer)
	if err == nil || result.Written || (!strings.Contains(err.Error(), "changed") && !errors.Is(err, os.ErrNotExist)) {
		t.Fatalf("removed destination parent result=%+v err=%v", result, err)
	}
	if !consumer.used["test-bridge:nonce-1"] {
		t.Fatal("fixture did not reach the post-consumption apply callback")
	}
	if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-consumption writer recreated the removed parent: %v", err)
	}
}

func TestApprovalReportsMovedBackupAfterPostWriteAncestorRename(t *testing.T) {
	project := t.TempDir()
	contract := validAssetContract()
	draft := filepath.Join(project, "Library", "SpatialDrafts", "banner.json")
	current, err := CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	if err := Save(current, contract); err != nil {
		t.Fatal(err)
	}
	baseline, _, err := currentBaseline(current, contract)
	if err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(project, "Assets")
	movedAssets := filepath.Join(project, "MovedAssets")
	consumer := renameAfterApplyConsumer{from: assets, to: movedAssets}
	evidence := ApprovalEvidence{Authority: "test-bridge", Nonce: "nonce-1", Proof: "valid-proof"}
	result, err := ApproveAndApplyAuthorized(project, current, draft, baseline, "local-user", evidence, testApprovalVerifier{}, consumer)
	if err == nil || !result.Written || result.Status != "WRITE_COMMITTED_UNRECEIPTED" || result.Backup == "" {
		t.Fatalf("ancestor rename result=%+v err=%v", result, err)
	}
	if !strings.HasPrefix(filepath.Clean(result.Backup), filepath.Clean(movedAssets)+string(filepath.Separator)) {
		t.Fatalf("backup path was not refreshed to the moved directory: %s", result.Backup)
	}
	if _, err := os.Stat(result.Backup); err != nil {
		t.Fatalf("refreshed backup is unavailable: %v", err)
	}
}

type renameAfterApplyConsumer struct {
	from string
	to   string
}

func (consumer renameAfterApplyConsumer) ConsumeApprovalGrant(value ApprovalVerification, apply func() error) error {
	if err := apply(); err != nil {
		return err
	}
	if err := os.Rename(consumer.from, consumer.to); err != nil {
		return err
	}
	if value.RevalidateApplied == nil {
		return errors.New("applied-state revalidation callback is missing")
	}
	return value.RevalidateApplied()
}
