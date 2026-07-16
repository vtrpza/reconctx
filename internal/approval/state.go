package approval

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
)

const (
	CollectionPhase = "collection"
	ArjunPhase      = "arjun"
)

func QueueDigest(queue model.CandidateQueue) (string, error) {
	if err := validateQueue(queue); err != nil {
		return "", err
	}
	encoded, err := canonical.Marshal(queue)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func VerifyDecision(record model.ApprovalRecord, phase, digest, decision string) error {
	if record.Phase != phase || record.Decision != decision {
		return errors.New("approval phase or decision does not match transition")
	}
	if record.OperatorLabel == "" || record.CreatedAt == "" || record.Supersedes != "" {
		return errors.New("approval record is incomplete or pre-superseded")
	}
	if !validDigest(digest) || !validDigest(record.ApprovedDigest) || subtle.ConstantTimeCompare([]byte(digest), []byte(record.ApprovedDigest)) != 1 {
		return errors.New("approval digest does not match current behavior")
	}
	return nil
}

func AppendRecord(existing []model.ApprovalRecord, record model.ApprovalRecord) ([]model.ApprovalRecord, error) {
	if record.Phase == "" || record.OperatorLabel == "" || record.CreatedAt == "" || !validDigest(record.ApprovedDigest) || record.Supersedes != "" {
		return nil, errors.New("invalid approval record")
	}
	if record.Decision != "approve" && record.Decision != "skip" && record.Decision != "cancel" {
		return nil, errors.New("invalid approval decision")
	}
	result := append([]model.ApprovalRecord(nil), existing...)
	for index := len(result) - 1; index >= 0; index-- {
		if result[index].Phase == record.Phase {
			record.Supersedes = result[index].ApprovedDigest
			break
		}
	}
	return append(result, record), nil
}

func validateQueue(queue model.CandidateQueue) error {
	if queue.QueueVersion != "reconctx-candidate-queue/v0" || !validDigest(queue.PlanDigest) {
		return errors.New("invalid candidate queue identity")
	}
	if queue.MaxTargets < 0 || len(queue.Candidates) > queue.MaxTargets || queue.Limits.RatePerSecond <= 0 || queue.Limits.Concurrency <= 0 || queue.Limits.Parallelism <= 0 || queue.Limits.RequestTimeoutSeconds <= 0 || queue.Limits.ExecutionTimeoutSeconds <= queue.Limits.RequestTimeoutSeconds {
		return errors.New("invalid candidate queue limits")
	}
	for index, candidate := range queue.Candidates {
		url, err := canonical.CanonicalizeURL(candidate.URL)
		if err != nil || candidate.URL != url.CanonicalRouteURL {
			return fmt.Errorf("candidate %d URL is not canonical", index+1)
		}
		validMethodLocation := candidate.Method == "GET" && candidate.Location == "query" || candidate.Method == "POST" && (candidate.Location == "form" || candidate.Location == "json")
		if !validMethodLocation || !filepath.IsAbs(candidate.WordlistPath) || !validDigest(candidate.WordlistSHA256) || candidate.RequestBudget <= 0 || candidate.Scope.Classification != "in_scope" || candidate.Scope.RuleID == "" || candidate.Scope.Reason == "" {
			return fmt.Errorf("candidate %d behavior is invalid", index+1)
		}
		if len(candidate.Argv) == 0 || !filepath.IsAbs(candidate.Argv[0]) {
			return fmt.Errorf("candidate %d command is invalid", index+1)
		}
		for _, argument := range candidate.Argv {
			if strings.ContainsRune(argument, '\x00') {
				return fmt.Errorf("candidate %d command contains NUL", index+1)
			}
		}
	}
	return nil
}
