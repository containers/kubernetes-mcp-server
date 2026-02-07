package storage

// StorageSummaryInput defines the input parameters for the storage summary tool
type StorageSummaryInput struct {
	// No input parameters required for the initial implementation
}

// StorageSummaryOutput defines the output structure for the storage summary tool
type StorageSummaryOutput struct {
	Summary interface{} `json:"summary"`
}

// Made with Bob
