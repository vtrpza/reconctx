package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/scope"
)

type recordBuilder struct {
	context    Context
	records    model.RecordSet
	assets     map[string]int
	endpoints  map[string]int
	parameters map[string]int
}

func newRecordBuilder(context Context) *recordBuilder {
	return &recordBuilder{
		context: context, assets: map[string]int{}, endpoints: map[string]int{}, parameters: map[string]int{},
	}
}

func (builder *recordBuilder) decision(rawURL string) model.ScopeDecision {
	if builder.context.Scope == nil {
		return model.ScopeDecision{Classification: string(scope.Unknown), Reason: "scope evaluator is unavailable"}
	}
	decision := builder.context.Scope.EvaluateURL(rawURL)
	return model.ScopeDecision{Classification: string(decision.Classification), RuleID: decision.RuleID, Reason: decision.Reason}
}

func (builder *recordBuilder) addEndpoint(method *string, rawURL string) (*model.Endpoint, canonical.URL, error) {
	value, err := canonical.CanonicalizeURL(rawURL)
	if err != nil {
		return nil, canonical.URL{}, err
	}
	endpointID, err := canonical.EndpointID(method, value.CanonicalRouteURL)
	if err != nil {
		return nil, canonical.URL{}, err
	}
	asset := builder.addAsset(value)
	if index, ok := builder.endpoints[endpointID]; ok {
		return &builder.records.Endpoints[index], value, nil
	}
	endpoint := model.Endpoint{
		SchemaVersion: model.SchemaVersion, RecordType: "endpoint", ID: endpointID, RunID: builder.context.RunID,
		OriginAssetID: asset.ID, CanonicalRouteURL: value.CanonicalRouteURL, Scheme: value.Scheme, Host: value.Host,
		Port: value.Port, Path: value.Path, Method: method, MethodKnown: method != nil, Scope: builder.decision(rawURL),
		ObservationIDs: []string{}, EvidenceIDs: []string{},
	}
	builder.endpoints[endpointID] = len(builder.records.Endpoints)
	builder.records.Endpoints = append(builder.records.Endpoints, endpoint)
	return &builder.records.Endpoints[len(builder.records.Endpoints)-1], value, nil
}

func (builder *recordBuilder) addAsset(value canonical.URL) *model.Asset {
	assetID := stableID("asset", "origin", value.Origin)
	if index, ok := builder.assets[assetID]; ok {
		return &builder.records.Assets[index]
	}
	asset := model.Asset{
		SchemaVersion: model.SchemaVersion, RecordType: "asset", ID: assetID, RunID: builder.context.RunID,
		AssetKind: "origin", CanonicalValue: value.Origin, DisplayValue: value.Origin, Scope: builder.decision(value.Origin + "/"),
		ObservationIDs: []string{}, EvidenceIDs: []string{},
	}
	builder.assets[assetID] = len(builder.records.Assets)
	builder.records.Assets = append(builder.records.Assets, asset)
	return &builder.records.Assets[len(builder.records.Assets)-1]
}

func (builder *recordBuilder) addParameter(endpoint *model.Endpoint, name, location, discoveryKind string) (*model.Parameter, error) {
	parameterID, err := canonical.ParameterID(endpoint.ID, location, name)
	if err != nil {
		return nil, err
	}
	if index, ok := builder.parameters[parameterID]; ok {
		parameter := &builder.records.Parameters[index]
		parameter.DiscoveryKinds = appendUnique(parameter.DiscoveryKinds, discoveryKind)
		return parameter, nil
	}
	parameter := model.Parameter{
		SchemaVersion: model.SchemaVersion, RecordType: "parameter", ID: parameterID, RunID: builder.context.RunID,
		EndpointID: endpoint.ID, Name: name, Location: location, DiscoveryKinds: []string{discoveryKind},
		ObservationIDs: []string{}, EvidenceIDs: []string{},
	}
	builder.parameters[parameterID] = len(builder.records.Parameters)
	builder.records.Parameters = append(builder.records.Parameters, parameter)
	return &builder.records.Parameters[len(builder.records.Parameters)-1], nil
}

func (builder *recordBuilder) addEvidence(artifact model.Artifact, locator model.Locator, decision model.ScopeDecision) model.Evidence {
	locatorJSON, _ := canonical.Marshal(locator)
	evidence := model.Evidence{
		SchemaVersion: model.SchemaVersion, RecordType: "evidence",
		ID:    stableID("ev", builder.context.ToolExecutionID, artifact.SHA256, string(locatorJSON)),
		RunID: builder.context.RunID, ToolExecutionID: builder.context.ToolExecutionID, Artifact: artifact, Locator: locator,
		RedactionStatus: "not_needed", Scope: decision,
	}
	builder.records.Evidence = append(builder.records.Evidence, evidence)
	return evidence
}

func (builder *recordBuilder) addObservation(observationType, semanticState string, subject entity, observedAt *string, evidenceIDs []string, details any, decision model.ScopeDecision) model.Observation {
	components := []string{builder.context.ToolExecutionID, observationType, subject.entityID()}
	components = append(components, evidenceIDs...)
	observation := model.Observation{
		SchemaVersion: model.SchemaVersion, RecordType: "observation", ID: stableID("obs", components...),
		RunID: builder.context.RunID, ToolExecutionID: builder.context.ToolExecutionID, AuthContextID: builder.context.AuthContextID,
		ObservationType: observationType, SemanticState: semanticState, Subject: model.EntityRef{RecordType: subject.entityType(), ID: subject.entityID()},
		Scope: decision, ObservedAt: observedAt, EvidenceIDs: slices.Clone(evidenceIDs), Details: details,
	}
	builder.records.Observations = append(builder.records.Observations, observation)
	builder.link(subject, observation.ID, evidenceIDs)
	for _, evidenceID := range evidenceIDs {
		builder.records.Relationships = append(builder.records.Relationships, model.Relationship{
			SchemaVersion: model.SchemaVersion, RecordType: "relationship", RunID: builder.context.RunID,
			ID: stableID("rel", evidenceID, "evidence_for", observation.ID), RelationshipType: "evidence_for",
			From: model.EntityRef{RecordType: "evidence", ID: evidenceID}, To: model.EntityRef{RecordType: "observation", ID: observation.ID},
			EvidenceIDs: []string{evidenceID}, Attributes: map[string]any{},
		})
	}
	return observation
}

type entity interface {
	entityType() string
	entityID() string
}

type endpointEntity struct{ endpoint *model.Endpoint }

func (entity endpointEntity) entityType() string { return "endpoint" }
func (entity endpointEntity) entityID() string   { return entity.endpoint.ID }

type parameterEntity struct{ parameter *model.Parameter }

func (entity parameterEntity) entityType() string { return "parameter" }
func (entity parameterEntity) entityID() string   { return entity.parameter.ID }

func (builder *recordBuilder) link(subject entity, observationID string, evidenceIDs []string) {
	switch entity := subject.(type) {
	case endpointEntity:
		builder.linkEndpoint(entity.endpoint, observationID, evidenceIDs)
	case parameterEntity:
		entity.parameter.ObservationIDs = appendUnique(entity.parameter.ObservationIDs, observationID)
		entity.parameter.EvidenceIDs = appendUnique(entity.parameter.EvidenceIDs, evidenceIDs...)
		if index, ok := builder.endpoints[entity.parameter.EndpointID]; ok {
			builder.linkEndpoint(&builder.records.Endpoints[index], observationID, evidenceIDs)
		}
	}
}

func (builder *recordBuilder) linkEndpoint(endpoint *model.Endpoint, observationID string, evidenceIDs []string) {
	endpoint.ObservationIDs = appendUnique(endpoint.ObservationIDs, observationID)
	endpoint.EvidenceIDs = appendUnique(endpoint.EvidenceIDs, evidenceIDs...)
	if index, ok := builder.assets[endpoint.OriginAssetID]; ok {
		asset := &builder.records.Assets[index]
		asset.ObservationIDs = appendUnique(asset.ObservationIDs, observationID)
		asset.EvidenceIDs = appendUnique(asset.EvidenceIDs, evidenceIDs...)
	}
}

func (builder *recordBuilder) finish() model.RecordSet {
	for index := range builder.records.Parameters {
		parameter := &builder.records.Parameters[index]
		builder.records.Relationships = append(builder.records.Relationships, model.Relationship{
			SchemaVersion: model.SchemaVersion, RecordType: "relationship", RunID: builder.context.RunID,
			ID: stableID("rel", parameter.EndpointID, "has_parameter", parameter.ID), RelationshipType: "has_parameter",
			From: model.EntityRef{RecordType: "endpoint", ID: parameter.EndpointID}, To: model.EntityRef{RecordType: "parameter", ID: parameter.ID},
			EvidenceIDs: slices.Clone(parameter.EvidenceIDs), Attributes: map[string]any{},
		})
	}
	builder.records.Sort()
	return builder.records
}

func stableID(prefix string, components ...string) string {
	material := strings.Join(append([]string{"reconctx-" + prefix + "-v0"}, components...), "\x00")
	digest := sha256.Sum256([]byte(material))
	return prefix + "_sha256_" + hex.EncodeToString(digest[:])
}

func appendUnique(values []string, candidates ...string) []string {
	for _, candidate := range candidates {
		if !slices.Contains(values, candidate) {
			values = append(values, candidate)
		}
	}
	return values
}

func jsonPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func queryPairs(pairs []canonical.QueryPair) []model.QueryPair {
	result := make([]model.QueryPair, len(pairs))
	for index, pair := range pairs {
		result[index] = model.QueryPair{Index: pair.Index, RawName: pair.RawName, RawValue: pair.RawValue, Name: pair.Name, Value: pair.Value, HasEquals: pair.HasEquals}
	}
	return result
}

func requireNonEmpty(value, label string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", label)
	}
	return nil
}
