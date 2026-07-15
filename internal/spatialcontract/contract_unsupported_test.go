//go:build !linux && !windows

package spatialcontract

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApprovalWriteFailsBeforeVerificationOnUnsupportedOS(t *testing.T) {
	project := t.TempDir()
	contract := validAssetContract()
	draft := filepath.Join(project, "Library", "draft.spatial.json")
	if err := Save(draft, contract); err != nil {
		t.Fatal(err)
	}
	current, err := CanonicalContractPath(project, contract)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &unsupportedApprovalVerifier{}
	consumer := &unsupportedApprovalConsumer{}
	_, err = ApproveAndApplyAuthorized(project, current, draft, CurrentHashAbsent, "local-user", ApprovalEvidence{
		Authority: "local-review", Nonce: "0123456789abcdef0123456789abcdef", Proof: "unused",
	}, verifier, consumer)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported approval write error = %v", err)
	}
	if verifier.called || consumer.called {
		t.Fatalf("unsupported approval write crossed the authority boundary: verifier=%v consumer=%v", verifier.called, consumer.called)
	}
	if _, statErr := os.Stat(current); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unsupported approval write changed the tracked contract: %v", statErr)
	}
}

type unsupportedApprovalVerifier struct{ called bool }

func (verifier *unsupportedApprovalVerifier) VerifyApproval(ApprovalVerification) error {
	verifier.called = true
	return nil
}

type unsupportedApprovalConsumer struct{ called bool }

func (consumer *unsupportedApprovalConsumer) ConsumeApprovalGrant(ApprovalVerification, func() error) error {
	consumer.called = true
	return nil
}
