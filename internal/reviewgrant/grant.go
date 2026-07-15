package reviewgrant

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Kubonsang/unity-ctx/internal/durablefs"
	"github.com/Kubonsang/unity-ctx/internal/spatialcontract"
)

const ledgerVersion = 2

const staleDestinationLockAge = 20 * time.Minute

var (
	authorityPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
	noncePattern     = regexp.MustCompile(`^[a-zA-Z0-9_-]{24,192}$`)
	errLockVanished  = errors.New("approval destination lock vanished before inspection")
)

// Verifier validates an action-scoped grant signed by an authority whose
// public key was registered outside the Unity project.
type Verifier struct {
	AuthorityRoot string
	Now           func() time.Time
}

// Ledger is both the consume-once nonce store and the durable external record
// used to authorize later consumption of an Approved contract.
type Ledger struct {
	Root          string
	AuthorityRoot string
	Now           func() time.Time
	syncDirectory func(string) error
}

type approvalRecord struct {
	Version             int    `json:"version"`
	Action              string `json:"action"`
	Authority           string `json:"authority"`
	AuthorityKeyHash    string `json:"authority_key_hash,omitempty"`
	Nonce               string `json:"nonce"`
	ExpiresUnix         int64  `json:"expires_unix"`
	ContractHash        string `json:"contract_hash"`
	CurrentHash         string `json:"current_hash"`
	CaptureSetHash      string `json:"capture_set_hash"`
	Reviewer            string `json:"reviewer"`
	ContractPath        string `json:"contract_path"`
	SubjectGeometryHash string `json:"subject_geometry_hash,omitempty"`
	TargetGeometryHash  string `json:"target_geometry_hash,omitempty"`
	Proof               string `json:"proof"`
}

// ApprovalReceipt is cryptographically verified provenance returned by the
// concrete ledger. Interaction consumers use its signed geometry bindings to
// reject stale relative poses after either asset geometry changes.
type ApprovalReceipt struct {
	Action              string `json:"action"`
	Authority           string `json:"authority"`
	ContractHash        string `json:"contract_hash"`
	CurrentHash         string `json:"current_hash"`
	CaptureSetHash      string `json:"capture_set_hash"`
	Reviewer            string `json:"reviewer"`
	ContractPath        string `json:"contract_path"`
	SubjectGeometryHash string `json:"subject_geometry_hash,omitempty"`
	TargetGeometryHash  string `json:"target_geometry_hash,omitempty"`
}

// ReceiptCommittedError reports a failure that happened only after the
// immutable approval receipt was durably committed. Callers can distinguish
// this from uncertain receipt publication without importing this package.
type ReceiptCommittedError struct {
	Err error
}

func (err *ReceiptCommittedError) Error() string {
	if err == nil || err.Err == nil {
		return "approval receipt was committed before a post-commit check failed"
	}
	return err.Err.Error()
}

func (err *ReceiptCommittedError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func (err *ReceiptCommittedError) ReceiptCommitted() bool { return true }

type consumedRecord struct {
	Version      int    `json:"version"`
	Action       string `json:"action"`
	Authority    string `json:"authority"`
	NonceHash    string `json:"nonce_hash"`
	ContractHash string `json:"contract_hash"`
	CurrentHash  string `json:"current_hash"`
}

type destinationLockRecord struct {
	Version         int    `json:"version"`
	DestinationHash string `json:"destination_hash"`
	NonceHash       string `json:"nonce_hash"`
	LeaseID         string `json:"lease_id"`
	CreatedUnix     int64  `json:"created_unix"`
}

type authorityPinRecord struct {
	Version   int    `json:"version"`
	Authority string `json:"authority"`
	KeyHash   string `json:"key_hash"`
}

type heldDestinationLock struct {
	path   string
	record destinationLockRecord
	info   os.FileInfo
}

type destinationLockSnapshot struct {
	record destinationLockRecord
	info   os.FileInfo
}

type destinationLockLease struct {
	locks         []heldDestinationLock
	syncDirectory func(string) error
}

func DefaultAuthorityRoot() (string, error) {
	root, err := localDataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "unity-ctx", "review-authorities"), nil
}

func DefaultLedger() (*Ledger, error) {
	root, err := localDataRoot()
	if err != nil {
		return nil, err
	}
	return &Ledger{
		Root:          filepath.Join(root, "unity-ctx", "review-ledger"),
		AuthorityRoot: filepath.Join(root, "unity-ctx", "review-authorities"),
	}, nil
}

func localDataRoot() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve local review identity: %w", err)
	}
	home := filepath.Clean(strings.TrimSpace(current.HomeDir))
	if home == "." || !filepath.IsAbs(home) {
		return "", errors.New("resolve local review data root: OS account home is invalid")
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, "AppData", "Local"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		return filepath.Join(home, ".local", "share"), nil
	}
}

func (verifier Verifier) VerifyApproval(value spatialcontract.ApprovalVerification) error {
	_, err := verifier.verifyApprovalKey(value, true)
	return err
}

func (verifier Verifier) verifyApprovalKey(value spatialcontract.ApprovalVerification, requireFresh bool) (ed25519.PublicKey, error) {
	if value.Action != spatialcontract.ApprovalActionApproveApply {
		return nil, fmt.Errorf("approval grant action must be %s", spatialcontract.ApprovalActionApproveApply)
	}
	if !authorityPattern.MatchString(value.Evidence.Authority) {
		return nil, errors.New("approval grant authority is invalid")
	}
	if !noncePattern.MatchString(value.Evidence.Nonce) {
		return nil, errors.New("approval grant nonce is invalid")
	}
	if value.Evidence.ExpiresUnix <= 0 {
		return nil, errors.New("approval grant expiry is required")
	}
	if requireFresh {
		now := time.Now()
		if verifier.Now != nil {
			now = verifier.Now()
		}
		expires := time.Unix(value.Evidence.ExpiresUnix, 0)
		if !expires.After(now) {
			return nil, errors.New("approval grant has expired")
		}
		if expires.After(now.Add(15 * time.Minute)) {
			return nil, errors.New("approval grant expiry exceeds the 15 minute limit")
		}
	}
	if !hashPattern(value.ContractHash) || !currentHashPattern(value.CurrentHash) || !geometryBindingPattern(value.SubjectGeometryHash, value.TargetGeometryHash) || strings.TrimSpace(value.CaptureSetHash) == "" || strings.TrimSpace(value.Reviewer) == "" || !filepath.IsAbs(value.Destination) {
		return nil, errors.New("approval grant binding is incomplete")
	}
	root := strings.TrimSpace(verifier.AuthorityRoot)
	if root == "" {
		var err error
		root, err = DefaultAuthorityRoot()
		if err != nil {
			return nil, err
		}
	}
	key, err := loadPublicKey(filepath.Join(root, value.Evidence.Authority+".pub"))
	if err != nil {
		return nil, err
	}
	signature, err := decodeSignature(value.Evidence.Proof)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(key, SigningPayload(value), signature) {
		return nil, errors.New("approval grant signature is invalid")
	}
	return key, nil
}

// SigningPayload is the stable UTF-8 message Node/C# bridges sign with their
// registered Ed25519 private key. Each field is length-prefixed to prevent
// delimiter ambiguity.
func SigningPayload(value spatialcontract.ApprovalVerification) []byte {
	fields := []string{
		"unity-ctx-review-grant-v2",
		value.Action,
		value.Evidence.Authority,
		value.Evidence.Nonce,
		strconv.FormatInt(value.Evidence.ExpiresUnix, 10),
		strings.ToLower(value.ContractHash),
		strings.ToLower(value.CurrentHash),
		value.CaptureSetHash,
		value.Reviewer,
		filepath.ToSlash(filepath.Clean(value.Destination)),
		strings.ToLower(strings.TrimSpace(value.SubjectGeometryHash)),
		strings.ToLower(strings.TrimSpace(value.TargetGeometryHash)),
	}
	var builder strings.Builder
	for _, field := range fields {
		builder.WriteString(strconv.Itoa(len([]byte(field))))
		builder.WriteByte(':')
		builder.WriteString(field)
	}
	return []byte(builder.String())
}

func (ledger *Ledger) ConsumeApprovalGrant(value spatialcontract.ApprovalVerification, apply func() error) error {
	if ledger == nil || strings.TrimSpace(ledger.Root) == "" {
		return errors.New("approval ledger root is required")
	}
	if value.Action != spatialcontract.ApprovalActionApproveApply || !noncePattern.MatchString(value.Evidence.Nonce) || !hashPattern(value.ContractHash) || !currentHashPattern(value.CurrentHash) || !geometryBindingPattern(value.SubjectGeometryHash, value.TargetGeometryHash) {
		return errors.New("approval grant cannot be recorded because its binding is invalid")
	}
	if apply == nil {
		return errors.New("approval grant apply callback is required")
	}
	if !durablefs.SecurePublicationSupported() {
		return errors.New("approval grant consumption is unsupported on this operating system because secure publication is unavailable")
	}
	authorityRoot, err := ledger.authorityRoot()
	if err != nil {
		return err
	}
	verifiedKey, err := (Verifier{AuthorityRoot: authorityRoot, Now: ledger.Now}).verifyApprovalKey(value, true)
	if err != nil {
		return fmt.Errorf("approval grant cannot be recorded: %w", err)
	}
	keyHash := publicKeyHash(verifiedKey)
	nonceHash := digest(value.Evidence.Authority + "\x00" + value.Evidence.Nonce)
	record := approvalRecordFromVerification(value)
	record.AuthorityKeyHash = keyHash
	if record.SubjectGeometryHash == "" {
		if len(value.DependencyDestinations) != 0 {
			return errors.New("asset approval grant cannot lock interaction dependencies")
		}
	} else if len(value.DependencyDestinations) != 2 {
		return errors.New("interaction approval grant requires exactly two dependency destinations")
	}
	for _, destination := range value.DependencyDestinations {
		if !filepath.IsAbs(strings.TrimSpace(destination)) {
			return errors.New("interaction approval dependency destinations must be absolute")
		}
	}
	// Pin an authority only after the signed request has passed the complete
	// dependency-shape validation. A malformed first request must not make an
	// otherwise unused authority ID permanently unusable.
	if err := ledger.ensureAuthorityPinned(value.Evidence.Authority, keyHash); err != nil {
		return fmt.Errorf("approval grant authority pin failed: %w", err)
	}
	lockDestinations := make([]string, 0, len(value.DependencyDestinations)+1)
	lockDestinations = append(lockDestinations, value.DependencyDestinations...)
	lockDestinations = append(lockDestinations, record.ContractPath)
	destinationLease, err := ledger.acquireDestinationLocks(lockDestinations, nonceHash)
	if err != nil {
		return err
	}
	defer destinationLease.Release()
	if err := destinationLease.AssertOwned(); err != nil {
		return err
	}
	approvalDir := filepath.Join(ledger.Root, "approvals")
	if err := mkdirAllDurably(approvalDir, 0o700, ledger.syncDirectoryFunc()); err != nil {
		return err
	}
	approvalPath := filepath.Join(approvalDir, approvalReceiptFilename(record))
	approvalExists, err := approvalRecordMatches(approvalPath, record)
	if err != nil {
		return err
	}
	consumedDir := filepath.Join(ledger.Root, "consumed")
	if err := mkdirAllDurably(consumedDir, 0o700, ledger.syncDirectoryFunc()); err != nil {
		return err
	}
	marker := filepath.Join(consumedDir, nonceHash+".json")
	markerRecord := consumedRecord{Version: ledgerVersion, Action: value.Action, Authority: value.Evidence.Authority, NonceHash: nonceHash, ContractHash: record.ContractHash, CurrentHash: strings.ToLower(value.CurrentHash)}
	if err := writeJSONExclusiveDurably(marker, markerRecord, 0o600, ledger.syncDirectoryFunc()); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("approval grant nonce has already been consumed")
		}
		return fmt.Errorf("approval grant nonce durability is uncertain and requires manual recovery: %w", err)
	}
	if err := destinationLease.AssertOwned(); err != nil {
		return err
	}
	// The nonce reservation remains consumed even when the write fails, but no
	// durable approval record may exist until the contract write and reload/hash
	// verification have completed successfully.
	if err := apply(); err != nil {
		return err
	}
	if err := destinationLease.AssertOwned(); err != nil {
		if approvalExists {
			return &ReceiptCommittedError{Err: err}
		}
		return err
	}
	if value.RevalidateApplied != nil {
		if err := value.RevalidateApplied(); err != nil {
			if approvalExists {
				return &ReceiptCommittedError{Err: err}
			}
			return err
		}
	}
	if !approvalExists {
		if err := writeRecordExclusiveOrEqual(approvalPath, record, ledger.syncDirectoryFunc()); err != nil {
			return fmt.Errorf("approval receipt durability is uncertain: %w", err)
		}
	}
	if err := destinationLease.AssertOwned(); err != nil {
		return &ReceiptCommittedError{Err: err}
	}
	if value.RevalidateApplied != nil {
		if err := value.RevalidateApplied(); err != nil {
			return &ReceiptCommittedError{Err: err}
		}
	}
	return nil
}

func (ledger *Ledger) VerifyApprovedContract(value spatialcontract.ApprovedContractVerification) error {
	_, err := ledger.VerifyApprovedContractReceipt(value)
	return err
}

// VerifyApprovedContractReceipt verifies both the tracked-contract binding and
// the original human authority signature, then returns only signed provenance.
func (ledger *Ledger) VerifyApprovedContractReceipt(value spatialcontract.ApprovedContractVerification) (ApprovalReceipt, error) {
	if ledger == nil || strings.TrimSpace(ledger.Root) == "" || !hashPattern(value.ContractHash) {
		return ApprovalReceipt{}, errors.New("approval ledger lookup is invalid")
	}
	switch value.ContractType {
	case spatialcontract.TypeAsset:
		if value.SubjectGeometryHash != "" || value.TargetGeometryHash != "" {
			return ApprovalReceipt{}, errors.New("asset approval lookup must not carry interaction geometry bindings")
		}
	case spatialcontract.TypeInteraction:
		if !geometryBindingPattern(value.SubjectGeometryHash, value.TargetGeometryHash) || value.SubjectGeometryHash == "" {
			return ApprovalReceipt{}, errors.New("SUPPORT_CONTRACT_STALE: interaction approval lookup requires current subject and target geometry hashes")
		}
	default:
		return ApprovalReceipt{}, errors.New("approval ledger lookup contract type is invalid")
	}
	expected := approvalRecord{
		Version: ledgerVersion, Action: spatialcontract.ApprovalActionApproveApply,
		ContractHash: strings.ToLower(value.ContractHash), CaptureSetHash: value.CaptureSetHash,
		Reviewer: value.Reviewer, ContractPath: signingDestination(value.ContractPath),
	}
	paths, err := approvalCandidatePaths(filepath.Join(ledger.Root, "approvals"), expected)
	if err != nil {
		return ApprovalReceipt{}, err
	}
	if len(paths) == 0 {
		return ApprovalReceipt{}, errors.New("approval ledger has no record for this contract hash")
	}
	authorityRoot, err := ledger.authorityRoot()
	if err != nil {
		return ApprovalReceipt{}, err
	}
	var firstErr error
	var matchingErr error
	geometryMismatch := false
	for _, path := range paths {
		record, readErr := readApprovalRecord(path)
		if readErr != nil {
			if firstErr == nil {
				firstErr = readErr
			}
			continue
		}
		if record.Version != ledgerVersion || record.Action != spatialcontract.ApprovalActionApproveApply || !authorityPattern.MatchString(record.Authority) ||
			!strings.EqualFold(record.ContractHash, value.ContractHash) || record.CaptureSetHash != value.CaptureSetHash ||
			record.Reviewer != value.Reviewer || !samePath(record.ContractPath, value.ContractPath) {
			if firstErr == nil {
				firstErr = errors.New("approval ledger record does not match the contract evidence")
			}
			continue
		}
		if value.ContractType == spatialcontract.TypeAsset && (record.SubjectGeometryHash != "" || record.TargetGeometryHash != "") {
			if firstErr == nil {
				firstErr = errors.New("approval ledger asset receipt contains unexpected interaction geometry bindings")
			}
			continue
		}
		if value.ContractType == spatialcontract.TypeInteraction &&
			(!strings.EqualFold(record.SubjectGeometryHash, value.SubjectGeometryHash) || !strings.EqualFold(record.TargetGeometryHash, value.TargetGeometryHash)) {
			geometryMismatch = true
			continue
		}
		// The grant is necessarily expired when many approved contracts are later
		// consumed. Verify every signed binding, but deliberately do not re-apply
		// the short-lived freshness window that was enforced when it was recorded.
		verifiedKey, verifyErr := (Verifier{AuthorityRoot: authorityRoot}).verifyApprovalKey(record.approvalVerification(), false)
		if verifyErr != nil {
			if matchingErr == nil {
				matchingErr = fmt.Errorf("approval ledger receipt signature is invalid: %w", verifyErr)
			}
			continue
		}
		keyHash := publicKeyHash(verifiedKey)
		if record.AuthorityKeyHash != "" && !strings.EqualFold(record.AuthorityKeyHash, keyHash) {
			if matchingErr == nil {
				matchingErr = errors.New("approval ledger receipt authority key fingerprint does not match")
			}
			continue
		}
		if pinErr := ledger.verifyAuthorityPinned(record.Authority, keyHash); pinErr != nil {
			if matchingErr == nil {
				matchingErr = fmt.Errorf("approval ledger authority pin is invalid: %w", pinErr)
			}
			continue
		}
		return record.receipt(), nil
	}
	if matchingErr != nil {
		return ApprovalReceipt{}, matchingErr
	}
	if geometryMismatch {
		return ApprovalReceipt{}, errors.New("SUPPORT_CONTRACT_STALE: approval ledger geometry bindings do not match current approved assets")
	}
	if firstErr != nil {
		return ApprovalReceipt{}, firstErr
	}
	return ApprovalReceipt{}, errors.New("approval ledger has no valid record for this contract hash")
}

func (ledger *Ledger) authorityRoot() (string, error) {
	root := ""
	if ledger != nil {
		root = strings.TrimSpace(ledger.AuthorityRoot)
	}
	if root != "" {
		return filepath.Clean(root), nil
	}
	return DefaultAuthorityRoot()
}

func publicKeyHash(key ed25519.PublicKey) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}

func (ledger *Ledger) ensureAuthorityPinned(authority, keyHash string) error {
	return ledger.authorityPin(authority, keyHash, true)
}

func (ledger *Ledger) verifyAuthorityPinned(authority, keyHash string) error {
	return ledger.authorityPin(authority, keyHash, false)
}

func (ledger *Ledger) authorityPin(authority, keyHash string, syncExisting bool) error {
	if !authorityPattern.MatchString(authority) || !hashPattern(keyHash) {
		return errors.New("authority pin binding is invalid")
	}
	directory := filepath.Join(ledger.Root, "authorities")
	path := filepath.Join(directory, authority+".json")
	record := authorityPinRecord{Version: ledgerVersion, Authority: authority, KeyHash: strings.ToLower(keyHash)}
	if existing, err := readAuthorityPin(path); err == nil {
		if existing != record {
			return errors.New("authority key changed; register a new versioned authority ID instead of replacing its key")
		}
		if syncExisting {
			return ledger.syncDirectoryFunc()(directory)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !syncExisting {
		return errors.New("approval ledger authority pin is missing; read-only verification cannot bootstrap authority")
	}
	if err := mkdirAllDurably(directory, 0o700, ledger.syncDirectoryFunc()); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeImmutableBytesDurably(path, data, 0o600, ledger.syncDirectoryFunc()); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	existing, err := readAuthorityPin(path)
	if err != nil {
		return err
	}
	if existing != record {
		return errors.New("authority key changed; register a new versioned authority ID instead of replacing its key")
	}
	return ledger.syncDirectoryFunc()(directory)
}

func readAuthorityPin(path string) (authorityPinRecord, error) {
	data, err := readStableRegularFile(path, 16*1024, "authority pin")
	if err != nil {
		return authorityPinRecord{}, err
	}
	var record authorityPinRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return authorityPinRecord{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return authorityPinRecord{}, errors.New("authority pin contains trailing JSON content")
	}
	if record.Version != ledgerVersion || !authorityPattern.MatchString(record.Authority) || !hashPattern(record.KeyHash) {
		return authorityPinRecord{}, errors.New("authority pin is invalid")
	}
	return record, nil
}

func (ledger *Ledger) acquireDestinationLock(destination, nonceHash string) (heldDestinationLock, error) {
	destinationHash := digest(canonicalPathKey(destination))
	lockDir := filepath.Join(ledger.Root, "locks")
	if err := mkdirAllDurably(lockDir, 0o700, ledger.syncDirectoryFunc()); err != nil {
		return heldDestinationLock{}, err
	}
	lockPath := filepath.Join(lockDir, destinationHash+".lock")
	leaseID, err := newLeaseID()
	if err != nil {
		return heldDestinationLock{}, err
	}
	record := destinationLockRecord{
		Version: ledgerVersion, DestinationHash: destinationHash, NonceHash: nonceHash,
		LeaseID: leaseID, CreatedUnix: time.Now().UTC().Unix(),
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := writeJSONExclusiveDurably(lockPath, record, 0o600, ledger.syncDirectoryFunc()); err != nil {
			if errors.Is(err, os.ErrExist) {
				inspectErr := refuseExistingDestinationLock(lockPath, destinationHash, time.Now())
				if errors.Is(inspectErr, errLockVanished) {
					continue
				}
				return heldDestinationLock{}, inspectErr
			}
			return heldDestinationLock{}, fmt.Errorf("approval destination lock durability is uncertain and requires manual recovery: %w", err)
		}
		snapshot, snapshotErr := readDestinationLockSnapshot(lockPath)
		if snapshotErr != nil || snapshot.record != record {
			return heldDestinationLock{}, errors.New("approval destination lock ownership could not be confirmed after acquisition; manual recovery is required")
		}
		return heldDestinationLock{path: lockPath, record: record, info: snapshot.info}, nil
	}
	return heldDestinationLock{}, errors.New("approval destination lock changed repeatedly while it was being acquired")
}

func (ledger *Ledger) acquireDestinationLocks(destinations []string, nonceHash string) (*destinationLockLease, error) {
	unique := make(map[string]string, len(destinations))
	keys := make([]string, 0, len(destinations))
	for _, destination := range destinations {
		if !filepath.IsAbs(strings.TrimSpace(destination)) {
			return nil, errors.New("approval destination lock path must be absolute")
		}
		key := canonicalPathKey(destination)
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = destination
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lease := &destinationLockLease{locks: make([]heldDestinationLock, 0, len(keys)), syncDirectory: ledger.syncDirectoryFunc()}
	for _, key := range keys {
		held, err := ledger.acquireDestinationLock(unique[key], nonceHash)
		if err != nil {
			lease.Release()
			return nil, err
		}
		lease.locks = append(lease.locks, held)
	}
	return lease, nil
}

func refuseExistingDestinationLock(lockPath, destinationHash string, now time.Time) error {
	info, err := os.Lstat(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errLockVanished
		}
		return err
	}
	record, err := readDestinationLock(lockPath)
	if err != nil {
		// os.O_CREATE|os.O_EXCL publishes the lock name before its JSON record is
		// fully written. A concurrent contender may observe that short window;
		// treat any fresh partial record as an active writer, not corruption.
		if now.Sub(info.ModTime()) <= staleDestinationLockAge {
			return fmt.Errorf("approval destination is already being committed lock=%s", lockPath)
		}
		return fmt.Errorf("approval destination lock is invalid and requires manual recovery lock=%s", lockPath)
	}
	if record.Version != ledgerVersion || record.DestinationHash != destinationHash || !hashPattern(record.NonceHash) || !hashPattern(record.LeaseID) || record.CreatedUnix <= 0 {
		return fmt.Errorf("approval destination lock is invalid and requires manual recovery lock=%s", lockPath)
	}
	if now.Sub(info.ModTime()) <= staleDestinationLockAge || now.Sub(time.Unix(record.CreatedUnix, 0)) <= staleDestinationLockAge {
		return fmt.Errorf("approval destination is already being committed lock=%s lease_id=%s", lockPath, record.LeaseID)
	}
	return fmt.Errorf("approval destination lock is old and requires manual recovery lock=%s lease_id=%s", lockPath, record.LeaseID)
}

func releaseDestinationLock(held heldDestinationLock, syncDirectory func(string) error) {
	// Never move a canonical lock that is already owned by a successor. There
	// is no portable compare-and-unlink primitive, so the quarantine check below
	// remains the second fail-closed fence for a same-user swap in the narrow
	// interval between this validation and Rename.
	current, err := readDestinationLockSnapshot(held.path)
	if err != nil || current.record != held.record || held.info == nil || !os.SameFile(current.info, held.info) {
		return
	}
	leaseID, err := newLeaseID()
	if err != nil {
		return
	}
	quarantine := held.path + ".release-" + leaseID[:16]
	if err := os.Rename(held.path, quarantine); err != nil {
		return
	}
	quarantined, readErr := readDestinationLockSnapshot(quarantine)
	if readErr == nil && quarantined.record == held.record && os.SameFile(quarantined.info, held.info) {
		if err := os.Remove(quarantine); err == nil && syncDirectory != nil {
			_ = syncDirectory(filepath.Dir(held.path))
		}
		return
	}
	// A successor may have replaced the path between the old owner's final
	// assertion and release. Restore that exact inode only if the path is still
	// absent; never delete or overwrite a newer lock.
	if err := os.Link(quarantine, held.path); err == nil {
		if removeErr := os.Remove(quarantine); removeErr == nil && syncDirectory != nil {
			_ = syncDirectory(filepath.Dir(held.path))
		}
	}
}

func (lease *destinationLockLease) AssertOwned() error {
	if lease == nil {
		return errors.New("approval destination lock lease is missing")
	}
	for _, held := range lease.locks {
		snapshot, err := readDestinationLockSnapshot(held.path)
		if err != nil || snapshot.record != held.record || held.info == nil || !os.SameFile(snapshot.info, held.info) {
			return errors.New("approval destination lock ownership was lost; manual recovery is required")
		}
	}
	return nil
}

func (lease *destinationLockLease) Release() {
	if lease == nil {
		return
	}
	for index := len(lease.locks) - 1; index >= 0; index-- {
		held := lease.locks[index]
		releaseDestinationLock(held, lease.syncDirectory)
	}
}

func readDestinationLock(path string) (destinationLockRecord, error) {
	snapshot, err := readDestinationLockSnapshot(path)
	return snapshot.record, err
}

func readDestinationLockSnapshot(path string) (destinationLockSnapshot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return destinationLockSnapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 16*1024 {
		return destinationLockSnapshot{}, errors.New("destination lock must be a regular JSON file no larger than 16 KiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return destinationLockSnapshot{}, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = file.Close()
		return destinationLockSnapshot{}, errors.New("destination lock changed before it was opened")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 16*1024+1))
	closeErr := file.Close()
	if readErr != nil {
		return destinationLockSnapshot{}, readErr
	}
	if closeErr != nil {
		return destinationLockSnapshot{}, closeErr
	}
	if len(data) > 16*1024 {
		return destinationLockSnapshot{}, errors.New("destination lock exceeds 16 KiB")
	}
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return destinationLockSnapshot{}, errors.New("destination lock changed while it was read")
	}
	var record destinationLockRecord
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return destinationLockSnapshot{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return destinationLockSnapshot{}, errors.New("destination lock contains trailing JSON content")
	}
	return destinationLockSnapshot{record: record, info: opened}, nil
}

func (ledger *Ledger) syncDirectoryFunc() func(string) error {
	if ledger != nil && ledger.syncDirectory != nil {
		return ledger.syncDirectory
	}
	return durablefs.SyncDirectory
}

func newLeaseID() (string, error) {
	value := make([]byte, sha256.Size)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create approval destination lease: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func writeJSONExclusiveDurably(path string, value any, mode os.FileMode, syncDirectory func(string) error) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeBytesExclusiveDurably(path, append(data, '\n'), mode, syncDirectory)
}

func writeBytesExclusiveDurably(path string, data []byte, mode os.FileMode, syncDirectory func(string) error) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	written := 0
	for written < len(data) {
		count, writeErr := file.Write(data[written:])
		written += count
		if writeErr != nil {
			_ = file.Close()
			return writeErr
		}
		if count == 0 {
			_ = file.Close()
			return io.ErrShortWrite
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if syncDirectory == nil {
		return errors.New("durable ledger write requires directory sync")
	}
	return syncDirectory(filepath.Dir(path))
}

func writeImmutableBytesDurably(path string, data []byte, mode os.FileMode, syncDirectory func(string) error) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".ledger-immutable-*.tmp")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	written := 0
	for written < len(data) {
		count, writeErr := file.Write(data[written:])
		written += count
		if writeErr != nil {
			_ = file.Close()
			return writeErr
		}
		if count == 0 {
			_ = file.Close()
			return io.ErrShortWrite
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Link(tempPath, path); err != nil {
		return err
	}
	if syncDirectory == nil {
		return errors.New("durable immutable write requires directory sync")
	}
	return syncDirectory(directory)
}

func mkdirAllDurably(path string, mode os.FileMode, syncDirectory func(string) error) error {
	return durablefs.EnsureDirectoryTree(path, mode, syncDirectory)
}

func approvalRecordFromVerification(value spatialcontract.ApprovalVerification) approvalRecord {
	return approvalRecord{
		Version:             ledgerVersion,
		Action:              value.Action,
		Authority:           value.Evidence.Authority,
		Nonce:               value.Evidence.Nonce,
		ExpiresUnix:         value.Evidence.ExpiresUnix,
		ContractHash:        strings.ToLower(value.ContractHash),
		CurrentHash:         strings.ToLower(value.CurrentHash),
		CaptureSetHash:      value.CaptureSetHash,
		Reviewer:            value.Reviewer,
		ContractPath:        signingDestination(value.Destination),
		SubjectGeometryHash: strings.ToLower(strings.TrimSpace(value.SubjectGeometryHash)),
		TargetGeometryHash:  strings.ToLower(strings.TrimSpace(value.TargetGeometryHash)),
		Proof:               value.Evidence.Proof,
	}
}

func (record approvalRecord) approvalVerification() spatialcontract.ApprovalVerification {
	return spatialcontract.ApprovalVerification{
		Action:              record.Action,
		ContractHash:        record.ContractHash,
		CurrentHash:         record.CurrentHash,
		CaptureSetHash:      record.CaptureSetHash,
		Reviewer:            record.Reviewer,
		Destination:         filepath.FromSlash(record.ContractPath),
		SubjectGeometryHash: record.SubjectGeometryHash,
		TargetGeometryHash:  record.TargetGeometryHash,
		Evidence: spatialcontract.ApprovalEvidence{
			Authority:   record.Authority,
			Nonce:       record.Nonce,
			ExpiresUnix: record.ExpiresUnix,
			Proof:       record.Proof,
		},
	}
}

func (record approvalRecord) receipt() ApprovalReceipt {
	return ApprovalReceipt{
		Action:              record.Action,
		Authority:           record.Authority,
		ContractHash:        record.ContractHash,
		CurrentHash:         record.CurrentHash,
		CaptureSetHash:      record.CaptureSetHash,
		Reviewer:            record.Reviewer,
		ContractPath:        record.ContractPath,
		SubjectGeometryHash: record.SubjectGeometryHash,
		TargetGeometryHash:  record.TargetGeometryHash,
	}
}

func loadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readStableRegularFile(path, 16*1024, "registered review authority")
	if err != nil {
		return nil, fmt.Errorf("load registered review authority: %w", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if block, _ := pem.Decode(data); block != nil {
		parsed, parseErr := x509.ParsePKIXPublicKey(block.Bytes)
		if parseErr != nil {
			return nil, fmt.Errorf("parse registered review authority: %w", parseErr)
		}
		key, ok := parsed.(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("registered review authority is not an Ed25519 public key")
		}
		return key, nil
	}
	for _, decode := range []func(string) ([]byte, error){base64.RawURLEncoding.DecodeString, base64.StdEncoding.DecodeString, hex.DecodeString} {
		if raw, decodeErr := decode(trimmed); decodeErr == nil && len(raw) == ed25519.PublicKeySize {
			return ed25519.PublicKey(raw), nil
		}
	}
	return nil, errors.New("registered review authority must be PEM, base64, base64url, or hex Ed25519 public key")
}

func decodeSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	for _, decode := range []func(string) ([]byte, error){base64.RawURLEncoding.DecodeString, base64.StdEncoding.DecodeString, hex.DecodeString} {
		if raw, err := decode(value); err == nil && len(raw) == ed25519.SignatureSize {
			return raw, nil
		}
	}
	return nil, errors.New("approval grant proof is not an Ed25519 signature")
}

func writeRecordExclusiveOrEqual(path string, record approvalRecord, syncDirectory func(string) error) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeImmutableBytesDurably(path, data, 0o600, syncDirectory); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	existing, readErr := readStableRegularFile(path, 64*1024, "approval ledger record")
	if readErr != nil || string(existing) != string(data) {
		return errors.New("approval ledger already contains a conflicting contract record")
	}
	if syncDirectory == nil {
		return errors.New("durable ledger write requires directory sync")
	}
	return syncDirectory(filepath.Dir(path))
}

func approvalRecordMatches(path string, record approvalRecord) (bool, error) {
	existing, err := readStableRegularFile(path, 64*1024, "approval ledger record")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	expected, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	expected = append(expected, '\n')
	if string(existing) != string(expected) {
		return false, errors.New("approval ledger already contains a conflicting contract record")
	}
	return true, nil
}

func approvalRecordKey(record approvalRecord) string {
	return digest(strings.Join([]string{strconv.Itoa(record.Version), record.ContractHash, record.CaptureSetHash, record.Reviewer, canonicalPathKey(record.ContractPath)}, "\x00"))
}

func approvalReceiptFilename(record approvalRecord) string {
	binding := approvalRecordKey(record)
	receiptID := digest(strings.Join([]string{
		record.Authority,
		record.AuthorityKeyHash,
		record.Nonce,
		record.Proof,
		strings.ToLower(record.SubjectGeometryHash),
		strings.ToLower(record.TargetGeometryHash),
	}, "\x00"))
	return binding + "." + receiptID + ".json"
}

func approvalCandidatePaths(directory string, expected approvalRecord) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	binding := approvalRecordKey(expected)
	legacyName := binding + ".json"
	prefix := binding + "."
	paths := make([]string, 0, 2)
	for _, entry := range entries {
		name := entry.Name()
		candidate := name == legacyName
		if !candidate && strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".json") {
			receiptID := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
			candidate = hashPattern(receiptID)
		}
		if !candidate {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		paths = append(paths, filepath.Join(directory, name))
		if len(paths) > 1024 {
			return nil, errors.New("approval ledger contains too many receipts for one contract binding")
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func readApprovalRecord(path string) (approvalRecord, error) {
	data, err := readStableRegularFile(path, 64*1024, "approval ledger record")
	if err != nil {
		return approvalRecord{}, err
	}
	var record approvalRecord
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return approvalRecord{}, fmt.Errorf("approval ledger record is invalid: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return approvalRecord{}, errors.New("approval ledger record is invalid: trailing JSON content")
	}
	return record, nil
}

func readStableRegularFile(path string, maximum int64, label string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if maximum <= 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("%s must be a non-empty regular file no larger than %d bytes", label, maximum)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("%s changed before it was opened", label)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maximum+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, maximum)
	}
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return nil, fmt.Errorf("%s changed while it was read", label)
	}
	return data, nil
}

func signingDestination(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}

func canonicalPathKey(value string) string {
	value = filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if filepath.Separator == '\\' {
		return strings.ToLower(value)
	}
	return value
}

func hashPattern(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func currentHashPattern(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), spatialcontract.CurrentHashAbsent) || hashPattern(value)
}

func geometryBindingPattern(subject, target string) bool {
	subject = strings.TrimSpace(subject)
	target = strings.TrimSpace(target)
	if subject == "" && target == "" {
		return true
	}
	return hashPattern(subject) && hashPattern(target)
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func samePath(left, right string) bool {
	left = filepath.Clean(filepath.FromSlash(left))
	right = filepath.Clean(filepath.FromSlash(right))
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}
