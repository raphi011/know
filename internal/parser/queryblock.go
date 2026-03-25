package parser

// QueryFormat indicates how results should be rendered.
type QueryFormat int

const (
	FormatList  QueryFormat = iota // bullet list of links
	FormatTable                    // columnar table
	FormatTask                     // checkbox list
)

// ConditionOp is a WHERE condition operator.
type ConditionOp int

const (
	OpContain  ConditionOp = iota // labels CONTAIN "x" (label membership)
	OpEqual                       // field = "x" (exact match)
	OpContains                    // field CONTAINS "x" (substring)
)

// Condition represents a parsed WHERE clause.
type Condition struct {
	Field string
	Op    ConditionOp
	Value string
}

// ShowField is a field with an optional display alias.
type ShowField struct {
	Name  string // field name (e.g. "title")
	Alias string // AS alias (e.g. "Name"), empty if none
}

// QueryBlock represents a parsed know query block.
type QueryBlock struct {
	Index      int         // byte offset of opening ``` in content
	RawQuery   string      // raw DSL text inside the fences
	Format     QueryFormat // LIST, TABLE, or TASK
	WithoutID  bool        // WITHOUT ID modifier
	Fields     []ShowField // fields from format line
	Folder     *string     // FROM folder
	Conditions []Condition // WHERE clauses
	SortField  string      // SORT field
	SortDesc   bool        // SORT DESC
	Limit      int         // LIMIT N
	Error      string      // parse error, empty if ok
}
