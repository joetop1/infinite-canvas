package model

// MediaContract is the common, public descriptor used by forms and submissions.
// The compiled JSON Schema stays on the server; no credentials are published.
type MediaContract struct {
	Version               int                           `json:"version"`
	Status                string                        `json:"status"`
	Reason                string                        `json:"reason,omitempty"`
	Source                string                        `json:"source,omitempty"`
	Revision              string                        `json:"revision,omitempty"`
	ProviderVersion       string                        `json:"providerVersion,omitempty"`
	FetchedAt             string                        `json:"fetchedAt,omitempty"`
	Capability            string                        `json:"capability,omitempty"`
	Bindings              map[string]MediaContractField `json:"bindings,omitempty"`
	NativeAudio           bool                          `json:"nativeAudio,omitempty"`
	RequiresReference     bool                          `json:"requiresReference,omitempty"`
	MaxReferences         int                           `json:"maxReferences,omitempty"`
	ReferenceMinSeconds   float64                       `json:"referenceMinSeconds,omitempty"`
	ReferenceMaxSeconds   float64                       `json:"referenceMaxSeconds,omitempty"`
	ReferenceTotalSeconds float64                       `json:"referenceTotalSeconds,omitempty"`
}

type MediaContractField struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required,omitempty"`
	Default  any      `json:"default,omitempty"`
	Values   []any    `json:"values,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	MaxItems int      `json:"maxItems,omitempty"`
}
