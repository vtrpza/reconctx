package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	fileintegrity "github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
)

func validateRecords(runID string, records model.RecordSet, rawFiles map[string][]byte, rawPolicy string) error {
	ids := map[string]string{}
	add := func(kind, id, recordRunID string) error {
		if id == "" || recordRunID != "" && recordRunID != runID {
			return fmt.Errorf("invalid %s identity %q", kind, id)
		}
		if previous, exists := ids[id]; exists {
			return fmt.Errorf("duplicate ID %s (%s and %s)", id, previous, kind)
		}
		ids[id] = kind
		return nil
	}
	for _, record := range records.Runs {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "run" || record.ID != runID || record.CanonicalizationPolicy != "url-canonicalization/v0" {
			return errors.New("invalid run record")
		}
		if err := add("run", record.ID, ""); err != nil {
			return err
		}
	}
	for _, record := range records.ToolExecutions {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "tool_execution" {
			return errors.New("invalid tool execution record")
		}
		if err := add("tool_execution", record.ID, record.RunID); err != nil {
			return err
		}
	}
	for _, record := range records.Assets {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "asset" {
			return errors.New("invalid asset record")
		}
		if err := add("asset", record.ID, record.RunID); err != nil {
			return err
		}
	}
	for _, record := range records.Endpoints {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "endpoint" {
			return errors.New("invalid endpoint record")
		}
		if err := add("endpoint", record.ID, record.RunID); err != nil {
			return err
		}
	}
	for _, record := range records.Parameters {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "parameter" {
			return errors.New("invalid parameter record")
		}
		if err := add("parameter", record.ID, record.RunID); err != nil {
			return err
		}
	}
	for _, record := range records.Observations {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "observation" {
			return errors.New("invalid observation record")
		}
		if err := add("observation", record.ID, record.RunID); err != nil {
			return err
		}
	}
	for _, record := range records.Evidence {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "evidence" {
			return errors.New("invalid evidence record")
		}
		if err := add("evidence", record.ID, record.RunID); err != nil {
			return err
		}
	}
	for _, record := range records.Relationships {
		if record.SchemaVersion != model.SchemaVersion || record.RecordType != "relationship" {
			return errors.New("invalid relationship record")
		}
		if err := add("relationship", record.ID, record.RunID); err != nil {
			return err
		}
	}

	require := func(id, kind string) error {
		if ids[id] != kind {
			return fmt.Errorf("unresolved %s reference %q", kind, id)
		}
		return nil
	}
	requireEvidence := func(values []string) error {
		if len(values) == 0 {
			return errors.New("material record has no Evidence IDs")
		}
		for _, id := range values {
			if err := require(id, "evidence"); err != nil {
				return err
			}
		}
		return nil
	}
	evidenceOwners := make(map[string]string, len(records.Evidence))
	for _, record := range records.Evidence {
		evidenceOwners[record.ID] = record.ToolExecutionID
	}
	requireOwnedEvidence := func(values []string, executionID string) error {
		for _, id := range values {
			if err := require(id, "evidence"); err != nil {
				return err
			}
			if executionID != "" && evidenceOwners[id] != executionID {
				return fmt.Errorf("diagnostic Evidence %q belongs to another tool execution", id)
			}
		}
		return nil
	}
	validateDiagnostics := func(values []model.Diagnostic, executionID string) error {
		for _, diagnostic := range values {
			if err := requireOwnedEvidence(diagnostic.EvidenceIDs, executionID); err != nil {
				return err
			}
		}
		return nil
	}
	for _, record := range records.Runs {
		if len(record.ToolExecutionIDs) != len(records.ToolExecutions) {
			return errors.New("run ToolExecution IDs do not cover every execution")
		}
		listed := make(map[string]bool, len(record.ToolExecutionIDs))
		for _, id := range record.ToolExecutionIDs {
			if err := require(id, "tool_execution"); err != nil {
				return err
			}
			listed[id] = true
		}
		for _, execution := range records.ToolExecutions {
			if !listed[execution.ID] {
				return fmt.Errorf("run omits tool execution %q", execution.ID)
			}
		}
		if err := validateDiagnostics(record.Warnings, ""); err != nil {
			return err
		}
		if err := validateDiagnostics(record.Gaps, ""); err != nil {
			return err
		}
	}
	rawReferenced := make(map[string]bool, len(rawFiles))
	executionArtifacts := make(map[string]map[string][]model.ArtifactSummary, len(records.ToolExecutions))
	for _, record := range records.ToolExecutions {
		for _, provider := range record.ProviderStatus {
			if err := requireOwnedEvidence(provider.EvidenceIDs, record.ID); err != nil {
				return err
			}
		}
		if err := validateDiagnostics(record.Warnings, record.ID); err != nil {
			return err
		}
		if err := validateDiagnostics(record.Gaps, record.ID); err != nil {
			return err
		}
		byPath := make(map[string][]model.ArtifactSummary, len(record.Artifacts))
		for _, artifact := range record.Artifacts {
			if err := validateArtifactSummary(record.ID, artifact, rawFiles, rawPolicy); err != nil {
				return err
			}
			if artifact.Present {
				rawReferenced[artifact.Path] = true
			}
			byPath[artifact.Path] = append(byPath[artifact.Path], artifact)
		}
		executionArtifacts[record.ID] = byPath
	}
	for _, record := range records.Assets {
		if len(record.ObservationIDs) == 0 {
			return fmt.Errorf("asset %s has no Observation IDs", record.ID)
		}
		for _, id := range record.ObservationIDs {
			if err := require(id, "observation"); err != nil {
				return err
			}
		}
		if err := requireEvidence(record.EvidenceIDs); err != nil {
			return err
		}
	}
	for _, record := range records.Endpoints {
		if len(record.ObservationIDs) == 0 {
			return fmt.Errorf("endpoint %s has no Observation IDs", record.ID)
		}
		if err := require(record.OriginAssetID, "asset"); err != nil {
			return err
		}
		for _, id := range record.ObservationIDs {
			if err := require(id, "observation"); err != nil {
				return err
			}
		}
		if err := requireEvidence(record.EvidenceIDs); err != nil {
			return err
		}
	}
	for _, record := range records.Parameters {
		if len(record.ObservationIDs) == 0 {
			return fmt.Errorf("parameter %s has no Observation IDs", record.ID)
		}
		if err := require(record.EndpointID, "endpoint"); err != nil {
			return err
		}
		for _, id := range record.ObservationIDs {
			if err := require(id, "observation"); err != nil {
				return err
			}
		}
		if err := requireEvidence(record.EvidenceIDs); err != nil {
			return err
		}
	}
	for _, record := range records.Observations {
		if err := require(record.ToolExecutionID, "tool_execution"); err != nil {
			return err
		}
		if err := require(record.Subject.ID, record.Subject.RecordType); err != nil {
			return err
		}
		if err := requireEvidence(record.EvidenceIDs); err != nil {
			return err
		}
		if err := requireOwnedEvidence(record.EvidenceIDs, record.ToolExecutionID); err != nil {
			return err
		}
	}
	for _, record := range records.Evidence {
		if err := require(record.ToolExecutionID, "tool_execution"); err != nil {
			return err
		}
		if err := fileintegrity.ValidateRelativePath(record.Artifact.Path); err != nil || !strings.HasPrefix(record.Artifact.Path, "raw/") {
			return fmt.Errorf("evidence %s has unsafe artifact path", record.ID)
		}
		if record.Artifact.Role == "" || record.Artifact.MediaType == "" || record.Artifact.SizeBytes < 0 || !validSHA256(record.Artifact.SHA256) {
			return fmt.Errorf("evidence %s has invalid artifact metadata", record.ID)
		}
		content, embedded := rawFiles[record.Artifact.Path]
		if record.RedactionStatus != "withheld" && rawPolicy == "embedded_sanitized" && !embedded {
			return fmt.Errorf("evidence %s artifact is not embedded", record.ID)
		}
		if embedded {
			if !record.Artifact.Sanitized {
				return fmt.Errorf("evidence %s artifact is not marked sanitized", record.ID)
			}
			if err := validateArtifactBytes(record.Artifact.Path, record.Artifact.SHA256, record.Artifact.SizeBytes, content); err != nil {
				return fmt.Errorf("evidence %s: %w", record.ID, err)
			}
			if err := validateLocator(record.Locator, content); err != nil {
				return fmt.Errorf("evidence %s locator: %w", record.ID, err)
			}
			rawReferenced[record.Artifact.Path] = true
		} else if _, err := json.Marshal(record.Locator); err != nil {
			return fmt.Errorf("evidence %s locator: %w", record.ID, err)
		}
		if record.RedactionStatus != "withheld" && !matchesArtifactSummary(record.Artifact, executionArtifacts[record.ToolExecutionID][record.Artifact.Path]) {
			return fmt.Errorf("evidence %s artifact does not match its tool execution summary", record.ID)
		}
	}
	for _, record := range records.Relationships {
		if err := require(record.From.ID, record.From.RecordType); err != nil {
			return err
		}
		if err := require(record.To.ID, record.To.RecordType); err != nil {
			return err
		}
		if err := requireEvidence(record.EvidenceIDs); err != nil {
			return err
		}
	}
	for name := range rawFiles {
		if err := fileintegrity.ValidateRelativePath(name); err != nil || !strings.HasPrefix(name, "raw/") {
			return fmt.Errorf("unsafe raw artifact %q", name)
		}
		if !rawReferenced[name] {
			return fmt.Errorf("raw artifact %s has no normalized artifact summary or Evidence record", name)
		}
	}
	return nil
}

func validateCandidateReferences(rows []json.RawMessage, records model.RecordSet) error {
	endpoints := make(map[string]model.Endpoint, len(records.Endpoints))
	for _, record := range records.Endpoints {
		endpoints[record.ID] = record
	}
	observations := make(map[string]bool, len(records.Observations))
	for _, record := range records.Observations {
		observations[record.ID] = true
	}
	evidence := make(map[string]bool, len(records.Evidence))
	for _, record := range records.Evidence {
		evidence[record.ID] = true
	}
	executions := make(map[string]bool, len(records.ToolExecutions))
	for _, record := range records.ToolExecutions {
		executions[record.ID] = true
	}
	type references struct {
		CandidateID        string   `json:"candidate_id"`
		QueueDigest        string   `json:"queue_digest"`
		EndpointID         string   `json:"endpoint_id"`
		SelectedURL        string   `json:"selected_url"`
		CanonicalRouteURL  string   `json:"canonical_route_url"`
		ObservationIDs     []string `json:"observation_ids"`
		EvidenceIDs        []string `json:"evidence_ids"`
		SourceExecutionIDs []string `json:"source_execution_ids"`
	}
	seen := make(map[string]bool, len(rows))
	queueDigest := ""
	for index, row := range rows {
		var candidate references
		if err := json.Unmarshal(row, &candidate); err != nil {
			return fmt.Errorf("decode row %d: %w", index+1, err)
		}
		if seen[candidate.CandidateID] {
			return fmt.Errorf("row %d repeats candidate ID %q", index+1, candidate.CandidateID)
		}
		seen[candidate.CandidateID] = true
		if queueDigest == "" {
			queueDigest = candidate.QueueDigest
		} else if candidate.QueueDigest != queueDigest {
			return fmt.Errorf("row %d has a different queue digest", index+1)
		}
		endpoint, ok := endpoints[candidate.EndpointID]
		if !ok {
			return fmt.Errorf("row %d has unresolved endpoint reference %q", index+1, candidate.EndpointID)
		}
		if candidate.SelectedURL != endpoint.CanonicalRouteURL || candidate.CanonicalRouteURL != endpoint.CanonicalRouteURL {
			return fmt.Errorf("row %d URL differs from endpoint %q", index+1, candidate.EndpointID)
		}
		for _, id := range candidate.ObservationIDs {
			if !observations[id] {
				return fmt.Errorf("row %d has unresolved observation reference %q", index+1, id)
			}
		}
		for _, id := range candidate.EvidenceIDs {
			if !evidence[id] {
				return fmt.Errorf("row %d has unresolved evidence reference %q", index+1, id)
			}
		}
		for _, id := range candidate.SourceExecutionIDs {
			if !executions[id] {
				return fmt.Errorf("row %d has unresolved tool execution reference %q", index+1, id)
			}
		}
	}
	return nil
}

func validateArtifactSummary(executionID string, artifact model.ArtifactSummary, rawFiles map[string][]byte, rawPolicy string) error {
	if fileintegrity.ValidateRelativePath(artifact.Path) != nil || !strings.HasPrefix(artifact.Path, "raw/") || artifact.Role == "" || artifact.MediaType == "" {
		return fmt.Errorf("tool execution %s has an invalid artifact summary", executionID)
	}
	content, embedded := rawFiles[artifact.Path]
	if !artifact.Present {
		if artifact.SHA256 != nil || artifact.SizeBytes != nil || embedded {
			return fmt.Errorf("tool execution %s absent artifact %s has bytes or metadata", executionID, artifact.Path)
		}
		return nil
	}
	if artifact.SHA256 == nil || artifact.SizeBytes == nil {
		return fmt.Errorf("tool execution %s present artifact %s lacks digest or size", executionID, artifact.Path)
	}
	if *artifact.SizeBytes < 0 || !validSHA256(*artifact.SHA256) {
		return fmt.Errorf("tool execution %s artifact %s has invalid digest or size", executionID, artifact.Path)
	}
	if rawPolicy == "embedded_sanitized" && !embedded {
		return fmt.Errorf("tool execution %s artifact %s is not embedded", executionID, artifact.Path)
	}
	if embedded {
		if err := validateArtifactBytes(artifact.Path, *artifact.SHA256, *artifact.SizeBytes, content); err != nil {
			return fmt.Errorf("tool execution %s: %w", executionID, err)
		}
	}
	return nil
}

func validateArtifactBytes(name, digest string, size int64, content []byte) error {
	if size != int64(len(content)) {
		return fmt.Errorf("artifact %s size mismatch", name)
	}
	if !validSHA256(digest) {
		return fmt.Errorf("artifact %s has invalid SHA-256", name)
	}
	actual := sha256.Sum256(content)
	if digest != hex.EncodeToString(actual[:]) {
		return fmt.Errorf("artifact %s digest mismatch", name)
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func matchesArtifactSummary(artifact model.Artifact, summaries []model.ArtifactSummary) bool {
	for _, summary := range summaries {
		if summary.Present && summary.SHA256 != nil && summary.SizeBytes != nil &&
			summary.Role == artifact.Role && summary.Path == artifact.Path && *summary.SHA256 == artifact.SHA256 &&
			*summary.SizeBytes == artifact.SizeBytes && summary.MediaType == artifact.MediaType {
			return true
		}
	}
	return false
}

func validateLocator(locator model.Locator, content []byte) error {
	if _, err := json.Marshal(locator); err != nil {
		return err
	}
	switch locator.Kind {
	case "whole_artifact":
		return nil
	case "line_range":
		lines := bytes.Count(content, []byte{'\n'})
		if len(content) > 0 && content[len(content)-1] != '\n' {
			lines++
		}
		if locator.LineEnd > lines {
			return fmt.Errorf("line %d exceeds artifact line count %d", locator.LineEnd, lines)
		}
		return nil
	case "byte_range":
		if locator.ByteEndExclusive > int64(len(content)) {
			return fmt.Errorf("byte %d exceeds artifact size %d", locator.ByteEndExclusive, len(content))
		}
		return nil
	case "json_pointer":
		var document any
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.UseNumber()
		if err := decoder.Decode(&document); err != nil {
			return fmt.Errorf("artifact is not JSON: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err == nil {
				return errors.New("artifact contains multiple JSON values")
			}
			return fmt.Errorf("artifact is not JSON: %w", err)
		}
		return resolveJSONPointer(document, locator.Pointer)
	default:
		return fmt.Errorf("unsupported locator kind %q", locator.Kind)
	}
}

func resolveJSONPointer(value any, pointer string) error {
	if pointer == "" {
		return nil
	}
	for _, encoded := range strings.Split(pointer[1:], "/") {
		token, err := decodePointerToken(encoded)
		if err != nil {
			return err
		}
		switch node := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = node[token]
			if !ok {
				return fmt.Errorf("JSON pointer token %q does not exist", token)
			}
		case []any:
			if token == "" || len(token) > 1 && token[0] == '0' {
				return fmt.Errorf("JSON pointer array index %q is invalid", token)
			}
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(node) {
				return fmt.Errorf("JSON pointer array index %q is out of range", token)
			}
			value = node[index]
		default:
			return fmt.Errorf("JSON pointer token %q traverses a scalar", token)
		}
	}
	return nil
}

func decodePointerToken(value string) (string, error) {
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			continue
		}
		if index+1 >= len(value) || value[index+1] != '0' && value[index+1] != '1' {
			return "", errors.New("JSON pointer contains an invalid escape")
		}
		index++
	}
	return strings.ReplaceAll(strings.ReplaceAll(value, "~1", "/"), "~0", "~"), nil
}

func packageInventory(bundle Package) ([]manifestFile, error) {
	names := make([]string, 0, len(bundle))
	for name := range bundle {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]manifestFile, 0, len(names))
	for _, name := range names {
		content := bundle[name]
		digest := sha256.Sum256(content)
		role, mediaType := "normalized", "application/x-ndjson"
		switch {
		case name == "CONTEXT.md":
			role, mediaType = "context", "text/markdown"
		case name == "README.md":
			role, mediaType = "documentation", "text/markdown"
		case strings.HasPrefix(name, "raw/"):
			role, mediaType = "raw", mediaTypeFor(name)
		}
		files = append(files, manifestFile{Path: name, Role: role, SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(content)), MediaType: mediaType})
	}
	return files, nil
}

func mediaTypeFor(name string) string {
	switch {
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	case strings.HasSuffix(name, ".jsonl"):
		return "application/x-ndjson"
	case strings.HasSuffix(name, ".md"):
		return "text/markdown"
	default:
		return "text/plain"
	}
}

func checksumsForPackage(bundle Package, names []string) ([]byte, error) {
	sort.Strings(names)
	var output strings.Builder
	for _, name := range names {
		content, ok := bundle[name]
		if !ok {
			return nil, fmt.Errorf("missing package file %s", name)
		}
		digest := sha256.Sum256(content)
		fmt.Fprintf(&output, "%s  %s\n", hex.EncodeToString(digest[:]), name)
	}
	return []byte(output.String()), nil
}
