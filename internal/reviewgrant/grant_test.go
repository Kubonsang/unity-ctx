package reviewgrant

import (
	"bytes"
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
	recordForPath := approvalRecordFromVerification(verification)
	recordForPath.AuthorityKeyHash = publicKeyHash(privateKey.Public().(ed25519.PublicKey))
	recordPath := filepath.Join(ledger.Root, "approvals", approvalReceiptFilename(recordForPath))
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

func TestLedgerRequiresVersionedAuthorityIDAfterKeyRotation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, firstPrivateKey := signedTestLedger(t, now)
	first := signedVerification(t, firstPrivateKey, now)
	if err := ledger.ConsumeApprovalGrant(first, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	secondPublicKey, secondPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger.AuthorityRoot, "local-review.pub"), []byte(base64.RawURLEncoding.EncodeToString(secondPublicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Evidence.Nonce = "fedcba9876543210fedcba9876543210"
	second.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(secondPrivateKey, SigningPayload(second)))
	if err := ledger.ConsumeApprovalGrant(second, func() error { return nil }); err == nil || !strings.Contains(err.Error(), "versioned authority ID") {
		t.Fatalf("same authority ID accepted a replacement key: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ledger.AuthorityRoot, "local-review-v2.pub"), []byte(base64.RawURLEncoding.EncodeToString(secondPublicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	second.Evidence.Authority = "local-review-v2"
	second.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(secondPrivateKey, SigningPayload(second)))
	if err := ledger.ConsumeApprovalGrant(second, func() error { return nil }); err != nil {
		t.Fatalf("reapproval with a versioned authority ID failed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(ledger.Root, "approvals"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("rotated receipts did not coexist: entries=%d err=%v", len(entries), err)
	}
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset,
		ContractHash: second.ContractHash, CaptureSetHash: second.CaptureSetHash,
		Reviewer: second.Reviewer, ContractPath: second.Destination,
	}
	receipt, err := ledger.VerifyApprovedContractReceipt(lookup)
	if err != nil || receipt.Authority != "local-review-v2" {
		t.Fatalf("rotated receipt lookup failed: receipt=%+v err=%v", receipt, err)
	}
}

func TestLedgerSkipsInvalidSiblingWhenValidReceiptExists(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	record := approvalRecordFromVerification(verification)
	malformed := filepath.Join(ledger.Root, "approvals", approvalRecordKey(record)+"."+strings.Repeat("0", 64)+".json")
	if err := os.WriteFile(malformed, []byte("{not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeAsset,
		ContractHash: verification.ContractHash, CaptureSetHash: verification.CaptureSetHash,
		Reviewer: verification.Reviewer, ContractPath: verification.Destination,
	}
	if err := ledger.VerifyApprovedContract(lookup); err != nil {
		t.Fatalf("malformed sibling denied a valid receipt: %v", err)
	}
}

func TestInteractionCanBeReapprovedAfterGeometryChanges(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	first := signedVerification(t, privateKey, now)
	first.SubjectGeometryHash = strings.Repeat("b", 64)
	first.TargetGeometryHash = strings.Repeat("c", 64)
	first.DependencyDestinations = []string{filepath.Join(t.TempDir(), "subject.json"), filepath.Join(t.TempDir(), "target.json")}
	first.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(first)))
	if err := ledger.ConsumeApprovalGrant(first, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	second := first
	second.SubjectGeometryHash = strings.Repeat("d", 64)
	second.TargetGeometryHash = strings.Repeat("e", 64)
	second.Evidence.Nonce = "abcdef0123456789abcdef0123456789"
	second.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(second)))
	if err := ledger.ConsumeApprovalGrant(second, func() error { return nil }); err != nil {
		t.Fatalf("geometry-bound reapproval failed: %v", err)
	}
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeInteraction,
		ContractHash: second.ContractHash, CaptureSetHash: second.CaptureSetHash,
		Reviewer: second.Reviewer, ContractPath: second.Destination,
		SubjectGeometryHash: second.SubjectGeometryHash, TargetGeometryHash: second.TargetGeometryHash,
	}
	receipt, err := ledger.VerifyApprovedContractReceipt(lookup)
	if err != nil || receipt.SubjectGeometryHash != second.SubjectGeometryHash || receipt.TargetGeometryHash != second.TargetGeometryHash {
		t.Fatalf("latest geometry receipt=%+v err=%v", receipt, err)
	}
}

func TestLedgerPrioritizesMatchingGeometrySignatureError(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	old := signedVerification(t, privateKey, now)
	old.SubjectGeometryHash = strings.Repeat("b", 64)
	old.TargetGeometryHash = strings.Repeat("c", 64)
	old.DependencyDestinations = []string{filepath.Join(t.TempDir(), "subject.json"), filepath.Join(t.TempDir(), "target.json")}
	old.Evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(old)))
	if err := ledger.ConsumeApprovalGrant(old, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	matching := old
	matching.SubjectGeometryHash = strings.Repeat("d", 64)
	matching.TargetGeometryHash = strings.Repeat("e", 64)
	matching.Evidence.Nonce = "abcdef0123456789abcdef0123456789"
	matching.Evidence.Proof = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	record := approvalRecordFromVerification(matching)
	record.AuthorityKeyHash = publicKeyHash(privateKey.Public().(ed25519.PublicKey))
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ledger.Root, "approvals", approvalReceiptFilename(record))
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup := spatialcontract.ApprovedContractVerification{
		ContractType: spatialcontract.TypeInteraction,
		ContractHash: matching.ContractHash, CaptureSetHash: matching.CaptureSetHash,
		Reviewer: matching.Reviewer, ContractPath: matching.Destination,
		SubjectGeometryHash: matching.SubjectGeometryHash, TargetGeometryHash: matching.TargetGeometryHash,
	}
	if _, err := ledger.VerifyApprovedContractReceipt(lookup); err == nil || !strings.Contains(err.Error(), "signature is invalid") || strings.Contains(err.Error(), "SUPPORT_CONTRACT_STALE") {
		t.Fatalf("matching receipt error precedence = %v", err)
	}
}

func TestLedgerRevalidatesAppliedStateBeforeAndAfterReceipt(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	revalidations := 0
	verification.RevalidateApplied = func() error {
		revalidations++
		return nil
	}
	if err := ledger.ConsumeApprovalGrant(verification, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if revalidations != 2 {
		t.Fatalf("applied destination revalidation count=%d want=2", revalidations)
	}
}

func TestLedgerDoesNotWriteReceiptWhenAppliedStateChanged(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	verification.RevalidateApplied = func() error { return errors.New("APPLY_SOURCE_CHANGED injected") }
	applied := false
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err == nil || !strings.Contains(err.Error(), "APPLY_SOURCE_CHANGED") || !applied {
		t.Fatalf("changed applied state applied=%v err=%v", applied, err)
	}
	entries, err := os.ReadDir(filepath.Join(ledger.Root, "approvals"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("changed applied state wrote receipt: entries=%d err=%v", len(entries), err)
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

func TestLedgerNeverReclaimsAgedDestinationLock(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	verification := signedVerification(t, privateKey, now)
	lockPath := writeDestinationLockFixture(t, ledger, verification.Destination, strings.Repeat("f", 64), time.Now().Add(-staleDestinationLockAge-time.Minute))
	applied := false
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err == nil || !strings.Contains(err.Error(), "requires manual recovery") {
		t.Fatalf("aged destination lock error=%v", err)
	}
	if applied {
		t.Fatal("aged destination lock allowed the apply callback")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("aged destination lock was reclaimed: %v", err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil }); err != nil || !applied {
		t.Fatalf("manual lock recovery consumed the grant: applied=%v err=%v", applied, err)
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

func TestDestinationLockReaderRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.lock")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, 16*1024+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDestinationLock(path); err == nil || !strings.Contains(err.Error(), "16 KiB") {
		t.Fatalf("oversized destination lock error = %v", err)
	}
}

func TestLedgerRejectsLostLeaseBeforeReceipt(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	ledger.syncDirectory = func(string) error { return nil }
	verification := signedVerification(t, privateKey, now)
	destinationHash := digest(canonicalPathKey(verification.Destination))
	lockPath := filepath.Join(ledger.Root, "locks", destinationHash+".lock")
	applied := false
	err := ledger.ConsumeApprovalGrant(verification, func() error {
		applied = true
		successor := destinationLockRecord{
			Version: ledgerVersion, DestinationHash: destinationHash,
			NonceHash: digest("successor"), LeaseID: strings.Repeat("d", 64), CreatedUnix: time.Now().Unix(),
		}
		data, marshalErr := json.Marshal(successor)
		if marshalErr != nil {
			return marshalErr
		}
		if removeErr := os.Remove(lockPath); removeErr != nil {
			return removeErr
		}
		return os.WriteFile(lockPath, append(data, '\n'), 0o600)
	})
	if err == nil || !strings.Contains(err.Error(), "lock ownership was lost") || !applied {
		t.Fatalf("lost lease applied=%v err=%v", applied, err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("successor lock was removed by the old owner: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(ledger.Root, "approvals"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("lost lease created an approval receipt: entries=%d err=%v", len(entries), err)
	}
}

func TestReleaseDestinationLockDoesNotRemoveSuccessorWithSameNonce(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, _ := signedTestLedger(t, now)
	ledger.syncDirectory = func(string) error { return nil }
	held, err := ledger.acquireDestinationLock(filepath.Join(t.TempDir(), "contract.json"), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	successor := held.record
	successor.LeaseID = strings.Repeat("b", 64)
	data, err := json.Marshal(successor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(held.path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(held.path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	(&destinationLockLease{locks: []heldDestinationLock{held}, syncDirectory: ledger.syncDirectoryFunc()}).Release()
	got, err := readDestinationLock(held.path)
	if err != nil || got.LeaseID != successor.LeaseID {
		t.Fatalf("old owner removed successor lock: lease=%q err=%v", got.LeaseID, err)
	}
}

func TestLedgerDoesNotApplyWhenNonceDirectorySyncFails(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	ledger.syncDirectory = func(path string) error {
		if filepath.Base(path) == "consumed" {
			return errors.New("injected consumed directory sync failure")
		}
		return nil
	}
	verification := signedVerification(t, privateKey, now)
	applied := false
	err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "nonce durability is uncertain") || applied {
		t.Fatalf("nonce sync failure applied=%v err=%v", applied, err)
	}
	nonceHash := digest(verification.Evidence.Authority + "\x00" + verification.Evidence.Nonce)
	if _, err := os.Stat(filepath.Join(ledger.Root, "consumed", nonceHash+".json")); err != nil {
		t.Fatalf("uncertain nonce marker was removed: %v", err)
	}
}

func TestLedgerReportsUncertainReceiptDurability(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	ledger.syncDirectory = func(path string) error {
		if filepath.Base(path) == "approvals" {
			return errors.New("injected approvals directory sync failure")
		}
		return nil
	}
	verification := signedVerification(t, privateKey, now)
	applied := false
	err := ledger.ConsumeApprovalGrant(verification, func() error { applied = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "receipt durability is uncertain") || !applied {
		t.Fatalf("receipt sync failure applied=%v err=%v", applied, err)
	}
	entries, readErr := os.ReadDir(filepath.Join(ledger.Root, "approvals"))
	if readErr != nil || len(entries) != 1 {
		t.Fatalf("uncertain receipt was removed: entries=%d err=%v", len(entries), readErr)
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
		Version: ledgerVersion, DestinationHash: destinationHash, NonceHash: nonceHash,
		LeaseID: strings.Repeat("e", 64), CreatedUnix: created.Unix(),
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
