package main

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -package=ollama_client -generate=types,client,spec -o=./internal/ollama_client/api.gen.go openapi.yaml
