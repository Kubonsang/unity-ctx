package reviewgrant

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

func TestDestinationLockSerializesCompetingApprovalWrites(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger, privateKey := signedTestLedger(t, now)
	project := t.TempDir()
	first := concurrentAssetDraft("capture-first", 1)
	second := concurrentAssetDraft("capture-second", 2)
	firstDraft := filepath.Join(project, "Library", "SpatialDrafts", "first.json")
	secondDraft := filepath.Join(project, "Library", "SpatialDrafts", "second.json")
	if err := spatialcontract.Save(firstDraft, first); err != nil {
		t.Fatal(err)
	}
	if err := spatialcontract.Save(secondDraft, second); err != nil {
		t.Fatal(err)
	}
	current, err := spatialcontract.CanonicalContractPath(project, first)
	if err != nil {
		t.Fatal(err)
	}
	firstEvidence := signedConcurrentEvidence(privateKey, now, first, current, "11111111111111111111111111111111")
	secondEvidence := signedConcurrentEvidence(privateKey, now, second, current, "22222222222222222222222222222222")
	barrier := &approvalVerifierBarrier{
		underlying: Verifier{AuthorityRoot: ledger.AuthorityRoot, Now: func() time.Time { return now }},
		waitFor:    2,
		release:    make(chan struct{}),
	}

	errorsByDraft := make(chan error, 2)
	var workers sync.WaitGroup
	for _, input := range []struct {
		draft    string
		evidence spatialcontract.ApprovalEvidence
	}{{firstDraft, firstEvidence}, {secondDraft, secondEvidence}} {
		input := input
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, approveErr := spatialcontract.ApproveAndApplyAuthorized(project, current, input.draft, spatialcontract.CurrentHashAbsent, "local-user", input.evidence, barrier, ledger)
			errorsByDraft <- approveErr
		}()
	}
	workers.Wait()
	close(errorsByDraft)

	successes := 0
	failures := 0
	for approveErr := range errorsByDraft {
		if approveErr == nil {
			successes++
			continue
		}
		failures++
		if !strings.Contains(approveErr.Error(), "APPLY_SOURCE_CHANGED") && !strings.Contains(approveErr.Error(), "already being committed") {
			t.Fatalf("unexpected competing approval error: %v", approveErr)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("competing approvals successes=%d failures=%d, want exactly one each", successes, failures)
	}
	written, err := spatialcontract.Load(current)
	if err != nil {
		t.Fatal(err)
	}
	writtenHash := spatialcontract.ContentHash(written)
	if writtenHash != spatialcontract.ContentHash(first) && writtenHash != spatialcontract.ContentHash(second) {
		t.Fatalf("winner content hash %s did not match either signed draft", writtenHash)
	}
}

func concurrentAssetDraft(capture string, width float64) spatialcontract.Contract {
	contract := spatialcontract.Contract{
		ContractVersion: spatialcontract.ContractVersion, ContractType: spatialcontract.TypeAsset,
		State: spatialcontract.StateAwaitingHumanReview,
		Asset: &spatialcontract.AssetSpatialContract{
			AssetGUID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AssetPath: "Assets/Fixtures/shared.prefab",
			DependencyHash: "dependency-" + capture, Units: "meter", Forward: spatialcontract.Vec3{0, 0, 1}, Up: spatialcontract.Vec3{0, 1, 0},
			CollisionProxies: []spatialcontract.OBB{{ID: "body", Size: spatialcontract.Vec3{width, 1, 1}, Rotation: spatialcontract.Quat{0, 0, 0, 1}}},
			Frames:           []spatialcontract.ContactFrame{{ID: "bottom", Normal: spatialcontract.Vec3{0, -1, 0}, Tangent: spatialcontract.Vec3{1, 0, 0}, Size: [2]float64{width, 1}}},
			Contacts:         []spatialcontract.ContactRequirement{{ID: "floor", Kind: "FloorSupported", FrameID: "bottom", Target: "surface:floor", MaximumGap: .01, MinimumSupport: .6, DirectionAlignment: .95}},
			Revision:         1, CaptureSetHash: capture,
		},
		Technical: &spatialcontract.TechnicalEvidence{Passed: true, ErrorCount: 0, ReportHash: "report-" + capture},
	}
	spatialcontract.Normalize(&contract)
	return contract
}

func signedConcurrentEvidence(privateKey ed25519.PrivateKey, now time.Time, contract spatialcontract.Contract, destination, nonce string) spatialcontract.ApprovalEvidence {
	evidence := spatialcontract.ApprovalEvidence{Authority: "local-review", Nonce: nonce, ExpiresUnix: now.Add(5 * time.Minute).Unix()}
	verification := spatialcontract.ApprovalVerification{
		Action: spatialcontract.ApprovalActionApproveApply, ContractHash: spatialcontract.ContentHash(contract), CurrentHash: spatialcontract.CurrentHashAbsent,
		CaptureSetHash: contract.Asset.CaptureSetHash, Reviewer: "local-user", Destination: destination, Evidence: evidence,
	}
	evidence.Proof = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, SigningPayload(verification)))
	return evidence
}

type approvalVerifierBarrier struct {
	underlying spatialcontract.ApprovalVerifier
	waitFor    int
	mu         sync.Mutex
	arrived    int
	release    chan struct{}
}

func (barrier *approvalVerifierBarrier) VerifyApproval(value spatialcontract.ApprovalVerification) error {
	if err := barrier.underlying.VerifyApproval(value); err != nil {
		return err
	}
	barrier.mu.Lock()
	barrier.arrived++
	if barrier.arrived == barrier.waitFor {
		close(barrier.release)
	}
	barrier.mu.Unlock()
	select {
	case <-barrier.release:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("approval concurrency test barrier timed out")
	}
}
