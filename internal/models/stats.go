package models

// LabelStat is a label name with its document count.
type LabelStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// MemberStat is a vault member with their role.
type MemberStat struct {
	Name string `json:"name"`
	Role string `json:"role"`
}
