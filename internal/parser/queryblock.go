package parser

// QueryFormat indicates how results should be rendered.
type QueryFormat int

const (
	FormatInvalid QueryFormat = iota // zero value, indicates unset
	FormatList                       // bullet list of links
	FormatTable                      // columnar table
	FormatTask                       // checkbox list
)

// String returns the format keyword.
func (f QueryFormat) String() string {
	switch f {
	case FormatList:
		return "LIST"
	case FormatTable:
		return "TABLE"
	case FormatTask:
		return "TASK"
	default:
		return "INVALID"
	}
}

// ConditionOp is a WHERE condition operator.
type ConditionOp int

const (
	OpInvalid ConditionOp = iota // zero value, indicates unset
	OpContain                    // labels CONTAIN "x" (label membership)
	OpEqual                      // field = "x" (exact match)
)

// String returns the operator keyword.
func (op ConditionOp) String() string {
	switch op {
	case OpContain:
		return "CONTAIN"
	case OpEqual:
		return "="
	default:
		return "INVALID"
	}
}

// Default query block values.
const (
	DefaultSortField = "path"
	DefaultLimit     = 50
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
