package types

type OpenCPEProduct struct {
	Name           string `json:"name"`
	Title          string `json:"title"`
	Deprecated     bool   `json:"deprecated"`
	DeprecatedOver string `json:"deprecated_over"`
}
