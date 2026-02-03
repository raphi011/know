package models

// DecayedEntity represents an entity affected by decay.
type DecayedEntity struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	OldDecayWeight float64 `json:"old_decay_weight"`
	NewDecayWeight float64 `json:"new_decay_weight"`
	OldImportance  float64 `json:"old_importance"`
	NewImportance  float64 `json:"new_importance"`
}

// DecayResult is the result of a decay operation.
type DecayResult struct {
	Affected int             `json:"affected"`
	DryRun   bool            `json:"dry_run"`
	Entities []DecayedEntity `json:"entities"`
}

// SimilarPair represents two similar entities.
type SimilarPair struct {
	Entity1ID   string  `json:"entity1_id"`
	Entity1Name string  `json:"entity1_name"`
	Entity2ID   string  `json:"entity2_id"`
	Entity2Name string  `json:"entity2_name"`
	Similarity  float64 `json:"similarity"`
}

// SimilarPairsResult is the result of similar pairs detection.
type SimilarPairsResult struct {
	Pairs  []SimilarPair `json:"pairs"`
	Count  int           `json:"count"`
	DryRun bool          `json:"dry_run"` // Always true for similar (identify-only)
}
