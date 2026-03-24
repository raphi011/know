package models

// DevResetDBResponse is the API response for the dev reset-db endpoint.
type DevResetDBResponse struct {
	Preserved PreservedCounts `json:"preserved"`
}

// PreservedCounts holds the number of identity records preserved across a schema reset.
type PreservedCounts struct {
	Users        int `json:"users"`
	Vaults       int `json:"vaults"`
	VaultMembers int `json:"vault_members"`
	Tokens       int `json:"tokens"`
}
