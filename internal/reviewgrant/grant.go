package reviewgrant

import (
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

type heldDestinationLock struct {
	path   string
	record destinationLockRecord
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
		return filepath.Join(home, "Library", "Caches"), nil
	default:
		return filepath.Join(home, ".cache"), nil
	}
}

func (verifier Verifier) VerifyApproval(value spatialcontract.ApprovalVerification) error {
	return verifier.verifyApproval(value, true)
}

func (verifier Verifier) verifyApproval(value spatialcontract.ApprovalVerification, requireFresh bool) error {
	if value.Action != spatialcontract.ApprovalActionApproveApply {
		return fmt.Errorf("approval grant action must be %s", spatialcontract.ApprovalActionApproveApply)
	}
	if !authorityPattern.MatchString(value.Evidence.Authority) {
		return errors.New("approval grant authority is invalid")
	}
	if !noncePattern.MatchString(value.Evidence.Nonce) {
		return errors.New("approval grant nonce is invalid")
	}
	if value.Evidence.ExpiresUnix <= 0 {
		return errors.New("approval grant expiry is required")
	}
	if requireFresh {
		now := time.Now()
		if verifier.Now != nil {
			now = verifier.Now()
		}
		expires := time.Unix(value.Evidence.ExpiresUnix, 0)
		if !expires.After(now) {
			return errors.New("approval grant has expired")
		}
		if expires.After(now.Add(15 * time.Minute)) {
			return errors.New("approval grant expiry exceeds the 15 minute limit")
		}
	}
	if !hashPattern(value.ContractHash) || !currentHashPattern(value.CurrentHash) || !geometryBindingPattern(value.SubjectGeometryHash, value.TargetGeometryHash) || strings.TrimSpace(value.CaptureSetHash) == "" || strings.TrimSpace(value.Reviewer) == "" || !filepath.IsAbs(value.Destination) {
		return errors.New("approval grant binding is incomplete")
	}
	root := strings.TrimSpace(verifier.AuthorityRoot)
	if root == "" {
		var err error
		root, err = DefaultAuthorityRoot()
		if err != nil {
			return err
		}
	}
	key, err := loadPublicKey(filepath.Join(root, value.Evidence.Authority+".pub"))
	if err != nil {
		return err
	}
	signature, err := decodeSignature(value.Evidence.Proof)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key, SigningPayload(value), signature) {
		return errors.New("approval grant signature is invalid")
	}
	return nil
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
	authorityRoot, err := ledger.authorityRoot()
	if err != nil {
		return err
	}
	if err := (Verifier{AuthorityRoot: authorityRoot, Now: ledger.Now}).VerifyApproval(value); err != nil {
		return fmt.Errorf("approval grant cannot be recorded: %w", err)
	}
	nonceHash := digest(value.Evidence.Authority + "\x00" + value.Evidence.Nonce)
	record := approvalRecordFromVerification(value)
	if record.SubjectGeometryHash == "" {
		if len(value.DependencyDestinations) != 0 {
			return errors.New("asset approval grant cannot lock interaction dependencies")
		}
	} else if len(value.DependencyDestinations) != 2 {
		return errors.New("interaction approval grant requires exactly two dependency destinations")
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
	if err := os.MkdirAll(approvalDir, 0o700); err != nil {
		return err
	}
	approvalPath := filepath.Join(approvalDir, approvalRecordKey(record)+".json")
	approvalExists, err := approvalRecordMatches(approvalPath, record)
	if err != nil {
		return err
	}
	consumedDir := filepath.Join(ledger.Root, "consumed")
	if err := os.MkdirAll(consumedDir, 0o700); err != nil {
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
		return err
	}
	if approvalExists {
		return nil
	}
	if err := writeRecordExclusiveOrEqual(approvalPath, record, ledger.syncDirectoryFunc()); err != nil {
		return fmt.Errorf("approval receipt durability is uncertain: %w", err)
	}
	return destinationLease.AssertOwned()
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
	path := filepath.Join(ledger.Root, "approvals", approvalRecordKey(expected)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ApprovalReceipt{}, errors.New("approval ledger has no record for this contract hash")
		}
		return ApprovalReceipt{}, err
	}
	var record approvalRecord
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return ApprovalReceipt{}, fmt.Errorf("approval ledger record is invalid: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ApprovalReceipt{}, errors.New("approval ledger record is invalid: trailing JSON content")
	}
	if record.Version != ledgerVersion || record.Action != spatialcontract.ApprovalActionApproveApply || !authorityPattern.MatchString(record.Authority) ||
		!strings.EqualFold(record.ContractHash, value.ContractHash) || record.CaptureSetHash != value.CaptureSetHash ||
		record.Reviewer != value.Reviewer || !samePath(record.ContractPath, value.ContractPath) {
		return ApprovalReceipt{}, errors.New("approval ledger record does not match the contract evidence")
	}
	if value.ContractType == spatialcontract.TypeAsset && (record.SubjectGeometryHash != "" || record.TargetGeometryHash != "") {
		return ApprovalReceipt{}, errors.New("approval ledger asset receipt contains unexpected interaction geometry bindings")
	}
	if value.ContractType == spatialcontract.TypeInteraction &&
		(!strings.EqualFold(record.SubjectGeometryHash, value.SubjectGeometryHash) || !strings.EqualFold(record.TargetGeometryHash, value.TargetGeometryHash)) {
		return ApprovalReceipt{}, errors.New("SUPPORT_CONTRACT_STALE: approval ledger geometry bindings do not match current approved assets")
	}
	authorityRoot, err := ledger.authorityRoot()
	if err != nil {
		return ApprovalReceipt{}, err
	}
	// The grant is necessarily expired when many approved contracts are later
	// consumed. Verify every signed binding, but deliberately do not re-apply
	// the short-lived freshness window that was enforced when it was recorded.
	if err := (Verifier{AuthorityRoot: authorityRoot}).verifyApproval(record.approvalVerification(), false); err != nil {
		return ApprovalReceipt{}, fmt.Errorf("approval ledger receipt signature is invalid: %w", err)
	}
	return record.receipt(), nil
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

func (ledger *Ledger) acquireDestinationLock(destination, nonceHash string) (heldDestinationLock, error) {
	destinationHash := digest(canonicalPathKey(destination))
	lockDir := filepath.Join(ledger.Root, "locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
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
	if err := writeJSONExclusiveDurably(lockPath, record, 0o600, ledger.syncDirectoryFunc()); err != nil {
		if errors.Is(err, os.ErrExist) {
			return heldDestinationLock{}, refuseExistingDestinationLock(lockPath, destinationHash, time.Now())
		}
		return heldDestinationLock{}, fmt.Errorf("approval destination lock durability is uncertain and requires manual recovery: %w", err)
	}
	return heldDestinationLock{path: lockPath, record: record}, nil
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
			return nil
		}
		return err
	}
	record, err := readDestinationLock(lockPath)
	if err != nil {
		// os.O_CREATE|os.O_EXCL publishes the lock name before its JSON record is
		// fully written. A concurrent contender may observe that short window;
		// treat any fresh partial record as an active writer, not corruption.
		if now.Sub(info.ModTime()) <= staleDestinationLockAge {
			return errors.New("approval destination is already being committed")
		}
		return errors.New("approval destination lock is invalid and requires manual recovery")
	}
	if record.Version != ledgerVersion || record.DestinationHash != destinationHash || !hashPattern(record.NonceHash) || !hashPattern(record.LeaseID) || record.CreatedUnix <= 0 {
		return errors.New("approval destination lock is invalid and requires manual recovery")
	}
	if now.Sub(info.ModTime()) <= staleDestinationLockAge || now.Sub(time.Unix(record.CreatedUnix, 0)) <= staleDestinationLockAge {
		return errors.New("approval destination is already being committed")
	}
	return errors.New("approval destination lock is old and requires manual recovery")
}

func releaseDestinationLock(path string, expected destinationLockRecord) {
	record, err := readDestinationLock(path)
	if err == nil && record == expected {
		_ = os.Remove(path)
	}
}

func (lease *destinationLockLease) AssertOwned() error {
	if lease == nil {
		return errors.New("approval destination lock lease is missing")
	}
	for _, held := range lease.locks {
		record, err := readDestinationLock(held.path)
		if err != nil || record != held.record {
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
		before, err := os.Lstat(held.path)
		releaseDestinationLock(held.path, held.record)
		if err == nil {
			if _, afterErr := os.Lstat(held.path); errors.Is(afterErr, os.ErrNotExist) && before.Mode().IsRegular() && lease.syncDirectory != nil {
				_ = lease.syncDirectory(filepath.Dir(held.path))
			}
		}
	}
}

func readDestinationLock(path string) (destinationLockRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return destinationLockRecord{}, err
	}
	var record destinationLockRecord
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return destinationLockRecord{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return destinationLockRecord{}, errors.New("destination lock contains trailing JSON content")
	}
	return record, nil
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
	data, err := os.ReadFile(path)
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
	if err := writeBytesExclusiveDurably(path, data, 0o600, syncDirectory); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	existing, readErr := os.ReadFile(path)
	if readErr != nil || string(existing) != string(data) {
		return errors.New("approval ledger already contains a conflicting contract record")
	}
	if syncDirectory == nil {
		return errors.New("durable ledger write requires directory sync")
	}
	return syncDirectory(filepath.Dir(path))
}

func approvalRecordMatches(path string, record approvalRecord) (bool, error) {
	existing, err := os.ReadFile(path)
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
