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

func TestVerifierBindsActionDestinationContractAndCurrentHash(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authorityRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(authorityRoot, "local-review.pub"), []byte(base64.RawURLEncoding.EncodeToString(publicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	verification := signedVerification(t, privateKey, now)
	verifier := Verifier{AuthorityRoot: authorityRoot, Now: func() time.Time { return now }}
	if err := verifier.VerifyApproval(verification); err != nil {
		t.Fatalf("VerifyApproval() error = %v", err)
	}

	mutations := map[string]func(*spatialcontract.ApprovalVerification){
		"action": func(value *spatialcontract.ApprovalVerification) { value.Action = spatialcontract.ApprovalActionReview },
		"destination": func(value *spatialcontract.ApprovalVerification) {
			value.Destination = filepath.Join(filepath.Dir(value.Destination), "other.spatial.json")
		},
		"hash": func(value *spatialcontract.ApprovalVerification) { value.ContractHash = strings.Repeat("b", 64) },
		"current hash": func(value *spatialcontract.ApprovalVerification) {
			value.CurrentHash = strings.Repeat("c", 64)
		},
		"geometry hashes": func(value *spatialcontract.ApprovalVerification) {
			value.SubjectGeometryHash = strings.Repeat("d", 64)
			value.TargetGeometryHash = strings.Repeat("e", 64)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := verification
			mutate(&candidate)
			if err := verifier.VerifyApproval(candidate); err == nil {
				t.Fatal("mutated grant binding was accepted")
			}
		})
	}
}

func TestLedgerConsumesOnceAndCommitsOnlyAfterSuccessfulWrite(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	applied := false
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err != nil || !applied {
		t.Fatalf("ConsumeApprovalGrant() applied=%v err=%v", applied, err)
	}
	record := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset,
		ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: verification.Destination,
	}
	if err := ledger.VerifyApprovedContract(record); err != nil {
		t.Fatalf("VerifyApprovedContract() error = %v", err)
	}
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err == nil || !strings.Contains(err.Error(), "already been consumed") {
		t.Fatalf("reused nonce error = %v", err)
	}

	failedLedger := &Ledger{Root: t.TempDir(), AuthorityRoot: ledger.AuthorityRoot, Now: func() time.Time { return now }}
	failed := verification
	failed.Evidence.Nonce = "fedcba9876543210fedcba9876543210"
	failed.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(failed)))
	if err := failedLedger.ConsumeApprovalGrant(failed, func() error { return errors.New("disk write failed") }); err == nil || !strings.Contains(err.Error(), "disk write failed") {
		t.Fatalf("failed callback error = %v", err)
	}
	failedRecord := record
	failedRecord.ContractHash = failed.ContractHash
	if err := failedLedger.VerifyApprovedContract(failedRecord); err == nil || !strings.Contains(err.Error(), "no record") {
		t.Fatalf("failed write left an approval record: %v", err)
	}
	if err := failedLedger.ConsumeApprovalGrant(failed, func() error { return nil }); err == nil || !strings.Contains(err.Error(), "already been consumed") {
		t.Fatalf("failed write nonce was reusable: %v", err)
	}
}

func TestLedgerRejectsForgedAndTamperedApprovalReceipts(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset,
		ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: verification.Destination,
	}
	recordPath := filepath.Join(ledger.Root, "approvals", approvalRecordKey(approvalRecordFromVerification(verification))+".json")
	original, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}

	var record approvalRecord
	if err := json.Unmarshal(original, &record); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*approvalRecord){
		"authority":    func(value *approvalRecord) { value.Authority = "forged-review" },
		"nonce":        func(value *approvalRecord) { value.Nonce = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
		"expiry":       func(value *approvalRecord) { value.ExpiresUnix++ },
		"current hash": func(value *approvalRecord) { value.CurrentHash = strings.Repeat("c", 64) },
		"proof": func(value *approvalRecord) {
			value.Proof = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := record
			mutate(&candidate)
			encoded, marshalErr := json.Marshal(candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if writeErr := os.WriteFile(recordPath, append(encoded, '\n'), 0o600); writeErr != nil {
				t.Fatal(writeErr)
			}
			if verifyErr := ledger.VerifyApprovedContract(lookup); verifyErr == nil {
				t.Fatal("tampered approval receipt was accepted")
			}
			if restoreErr := os.WriteFile(recordPath, original, 0o600); restoreErr != nil {
				t.Fatal(restoreErr)
			}
		})
	}
}

func TestLedgerRejectsUnsignedLegacyRecordAndAllowsFreshV2ReReview(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset,
		ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: verification.Destination,
	}
	legacy := approvalRecord{
		Version: 1, Action: spatialcontract.ApprovalActionApproveApply, Authority: verification.Evidence.Authority,
		ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: canonicalPathKey(verification.Destination),
	}
	approvalDir := filepath.Join(ledger.Root, "approvals")
	if err := os.MkdirAll(approvalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyData, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(approvalDir, approvalRecordKey(legacy)+".json"), append(legacyData, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ledger.VerifyApprovedContract(lookup); err == nil || !strings.Contains(err.Error(), "no record") {
		t.Fatalf("unsigned legacy receipt was not rejected: %v", err)
	}
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatalf("fresh v2 re-review could not supersede legacy receipt: %v", err)
	}
	if err := ledger.VerifyApprovedContract(lookup); err != nil {
		t.Fatalf("fresh signed receipt was rejected: %v", err)
	}
}

func TestLedgerVerifiesSignedReceiptAfterGrantExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	ledger.Now = func() time.Time { return now.Add(24 * time.Hour) }
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset,
		ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: verification.Destination,
	}
	if err := ledger.VerifyApprovedContract(lookup); err != nil {
		t.Fatalf("durable signed receipt was incorrectly treated as a fresh grant: %v", err)
	}
}

func TestLedgerReceiptSurvivesRestartAndRejectsAuthorityKeyMismatch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset,
		ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: verification.Destination,
	}
	restarted := &Ledger{Root: ledger.Root, AuthorityRoot: ledger.AuthorityRoot}
	if err := restarted.VerifyApprovedContract(lookup); err != nil {
		t.Fatalf("signed approval did not survive a new ledger instance: %v", err)
	}

	wrongPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger.AuthorityRoot, "local-review.pub"), []byte(base64.RawURLEncoding.EncodeToString(wrongPublicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restarted.VerifyApprovedContract(lookup); err == nil || !strings.Contains(err.Error(), "signature is invalid") {
		t.Fatalf("receipt signed by a different authority key was accepted: %v", err)
	}
}

func TestLedgerConsumeRejectsUnverifiedGrant(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	verification.Evidence.Proof = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	applied := false
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err == nil || !strings.Contains(err.Error(), "signature is invalid") {
		t.Fatalf("unverified grant error = %v", err)
	}
	if applied {
		t.Fatal("unverified grant reached the apply callback")
	}
}

func TestLedgerRecoversOnlySafelyStaleDestinationLock(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	lockPath := writeDestinationLockFixture(t, ledger, verification.Destination, strings.Repeat("f", 64), time.Now().Add(-staleDestinationLockAge-time.Minute))
	applied := false
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err != nil || !applied {
		t.Fatalf("stale destination lock recovery applied=%v err=%v", applied, err)
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination lock remained after successful recovery: %v", err)
	}
}

func TestLedgerRefusesFreshDestinationLockWithoutConsumingGrant(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	lockPath := writeDestinationLockFixture(t, ledger, verification.Destination, strings.Repeat("f", 64), time.Now())
	applied := false
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err == nil || !strings.Contains(err.Error(), "already being committed") {
		t.Fatalf("fresh destination lock error = %v", err)
	}
	if applied {
		t.Fatal("fresh destination lock allowed the apply callback")
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err != nil || !applied {
		t.Fatalf("lock refusal consumed the grant: applied=%v err=%v", applied, err)
	}
}

func TestLedgerTreatsFreshIncompleteDestinationLockAsActiveWriter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	destinationHash := digest(canonicalPathKey(verification.Destination))
	lockDir := filepath.Join(ledger.Root, "locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(lockDir, destinationHash+".lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	applied := false
	err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "already being committed") {
		t.Fatalf("fresh incomplete destination lock error = %v", err)
	}
	if applied {
		t.Fatal("fresh incomplete destination lock allowed the apply callback")
	}
}

func TestDefaultRootsIgnoreCallerControlledCacheEnvironment(t *testing.T) {
	poisoned := filepath.Join(t.TempDir(), "caller-controlled")
	t.Setenv("LOCALAPPDATA", poisoned)
	t.Setenv("XDG_CACHE_HOME", poisoned)
	authorityRoot, err := DefaultAuthorityRoot()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := DefaultLedger()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(filepath.Clean(authorityRoot), filepath.Clean(poisoned)) || strings.HasPrefix(filepath.Clean(ledger.Root), filepath.Clean(poisoned)) {
		t.Fatalf("default review roots followed caller-controlled environment: authority=%s ledger=%s", authorityRoot, ledger.Root)
	}
	if !samePath(ledger.AuthorityRoot, authorityRoot) {
		t.Fatalf("default ledger did not carry the secure authority root: got=%s want=%s", ledger.AuthorityRoot, authorityRoot)
	}
}

func signedTestLedger(t *testing.T, now time.Time) (*Ledger, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authorityRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(authorityRoot, "local-review.pub"), []byte(base64.RawURLEncoding.EncodeToString(publicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Ledger{Root: t.TempDir(), AuthorityRoot: authorityRoot, Now: func() time.Time { return now }}, privateKey
}

func writeDestinationLockFixture(t *testing.T, ledger *Ledger, destination, nonceHash string, created time.Time) string {
	t.Helper()
	destinationHash := digest(canonicalPathKey(destination))
	lockDir := filepath.Join(ledger.Root, "locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(lockDir, destinationHash+".lock")
	data, err := json.Marshal(destinationLockRecord{
		Version: ledgerVersion, DestinationHash: destinationHash, NonceHash: nonceHash, CreatedUnix: created.Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, created, created); err != nil {
		t.Fatal(err)
	}
	return path
}

func signedVerification(t *testing.T, privateKey ed25519.PrivateKey, now time.Time) spatialcontract.ApprovalVerification {
	t.Helper()
	value := spatialcontract.ApprovalVerification{
		Action:         spatialcontract.ApprovalActionApproveApply,
		ContractHash:   strings.Repeat("a", 64),
		CurrentHash:    spatialcontract.CurrentHashAbsent,
		CaptureSetHash: "capture-1",
		Reviewer:       "local-user",
		Destination:    filepath.Join(t.TempDir(), "Assets", "SpatialContracts", "Assets", "a.spatial.json"),
		Evidence: spatialcontract.ApprovalEvidence{
			Authority: "local-review", Nonce: "0123456789abcdef0123456789abcdef", ExpiresUnix: now.Add(5 * time.Minute).Unix(),
		},
	}
	value.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(value)))
	return value
}
