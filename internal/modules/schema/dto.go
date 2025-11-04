package schema

// Payload is the full response for the Cityscape schema endpoint.
type Payload struct {
	Tables    []TableDTO    `json:"tables"`
	Relations []RelationDTO `json:"relations"`
	Metrics   Metrics       `json:"metrics"`
}

// Metrics describe freshness and truncation state for the payload.
type Metrics struct {
	LastRefreshed string `json:"lastRefreshed"`
	Truncated     bool   `json:"truncated"`
}

// TableDTO represents aggregated metadata for a single table.
type TableDTO struct {
	Schema      string     `json:"schema"`
	Name        string     `json:"name"`
	Cluster     string     `json:"cluster"`
	RowCount    int64      `json:"rowCount"`
	DataSizeMB  float64    `json:"dataSizeMB"`
	IndexSizeMB float64    `json:"indexSizeMB"`
	TotalSizeMB float64    `json:"totalSizeMB"`
	Indexes     []IndexDTO `json:"indexes"`
}

// IndexDTO captures index-level details derived from INFORMATION_SCHEMA.
type IndexDTO struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	SizeMB  float64  `json:"sizeMB"`
}

// RelationDTO expresses a foreign-key relationship between two tables.
type RelationDTO struct {
	Schema           string `json:"schema"`
	Constraint       string `json:"constraint"`
	FromTable        string `json:"fromTable"`
	FromColumn       string `json:"fromColumn"`
	ToTable          string `json:"toTable"`
	ToColumn         string `json:"toColumn"`
	ReferencedSchema string `json:"referencedSchema"`
}
