package types

type AggregatorResult struct {
	Nist    []NistProduct    `json:"nist"`
	Opencpe []OpenCPEProduct `json:"opencpe"`
}
