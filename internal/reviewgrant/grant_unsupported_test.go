//go:build !linux && !windows

package reviewgrant

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

func TestGrantConsumptionFailsBeforeCallbackOnUnsupportedOS(t *testing.T) {
	ledger := &Ledger{Root: t.TempDir()}
	called := false
	err := ledger.ConsumeApprovalGrant(spatialcontract.ApprovalVerification{
		Action: spatialcontract.ApprovalActionApproveApply, ContractHash: strings.Repeat("a", 64), CurrentHash: spatialcontract.CurrentHashAbsent,
		Evidence: spatialcontract.ApprovalEvidence{Nonce: "0123456789abcdef0123456789abcdef"},
	}, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported") || called {
		t.Fatalf("unsupported grant consumption called=%v err=%v", called, err)
	}
	if entries, readErr := os.ReadDir(ledger.Root); readErr != nil || len(entries) != 0 {
		t.Fatalf("unsupported grant consumption changed the ledger: entries=%d err=%v", len(entries), readErr)
	}
}

func TestExistingReceiptVerificationRemainsReadOnlyOnUnsupportedOS(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	authorityRoot := t.TempDir()
	authority := "local-review-v2"
	if err := os.WriteFile(filepath.Join(authorityRoot, authority+".pub"), []byte(base64.RawURLEncoding.EncodeToString(publicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	verification := spatialcontract.ApprovalVerification{
		Action: spatialcontract.ApprovalActionApproveApply, ContractHash: strings.Repeat("a", 64), CurrentHash: spatialcontract.CurrentHashAbsent,
		CaptureSetHash: "capture", Reviewer: "local-user", Destination: filepath.Join(t.TempDir(), "asset.spatial.json"),
		Evidence: spatialcontract.ApprovalEvidence{Authority: authority, Nonce: "0123456789abcdef0123456789abcdef", ExpiresUnix: time.Now().Add(time.Minute).Unix()},
	}
	verification.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(verification)))
	record := approvalRecordFromVerification(verification)
	record.AuthorityKeyHash = publicKeyHash(publicKey)
	writeUnsupportedFixture(t, filepath.Join(root, "approvals", approvalReceiptFilename(record)), record)
	writeUnsupportedFixture(t, filepath.Join(root, "authorities", authority+".json"), authorityPinRecord{Version: ledgerVersion, Authority: authority, KeyHash: record.AuthorityKeyHash})

	ledger := &Ledger{Root: root, AuthorityRoot: authorityRoot, syncDirectory: func(string) error {
		return errors.New("read-only receipt verification attempted a directory sync")
	}}
	receipt, err := ledger.VerifyApprovedContractReceipt(spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset, ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: verification.Destination,
	})
	if err != nil || receipt.Authority != authority {
		t.Fatalf("existing read-only receipt verification receipt=%+v err=%v", receipt, err)
	}
}

func writeUnsupportedFixture(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
