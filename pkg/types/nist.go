package types

type Response struct {
	ResultsPerPage int           `json:"resultsPerPage"`
	StartIndex     int           `json:"startIndex"`
	TotalResults   int           `json:"totalResults"`
	Format         string        `json:"format"`
	Version        string        `json:"version"`
	Timestamp      string        `json:"timestamp"`
	Products       []NistProduct `json:"products"`
}

type NistProduct struct {
	CPE CPE `json:"cpe"`
}

type CPE struct {
	Deprecated   bool            `json:"deprecated"`
	CPEName      string          `json:"cpeName"`
	CPENameID    string          `json:"cpeNameId"`
	LastModified string          `json:"lastModified"`
	Created      string          `json:"created"`
	Titles       []Title         `json:"titles"`
	DeprecatedBy []DeprecatedCpe `json:"deprecatedBy"`
}

type Title struct {
	Title string `json:"title"`
	Lang  string `json:"lang"`
}

type DeprecatedCpe struct {
	CPEName   string `json:"cpeName"`
	CPENameID string `json:"cpeNameId"`
}
