package api

import _ "embed"

//go:embed ../../openapi.yaml
var openapiSpec []byte

func loadOpenAPI() ([]byte, error) {
	return openapiSpec, nil
}
