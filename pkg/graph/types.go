package graph

type Model struct {
	Name       string
	Table      Table
	References []Reference
	Children   []Model
	FilterKeys []string
}

type Table struct {
	Name          string
	KeyField      string
	PriorityField string
	VersionField  string
}

type Reference struct {
	Model    string
	KeyField string
}

type Filter struct {
	Key      string
	Operator FilterOperator
	Values   []string
}

type FilterOperator string
