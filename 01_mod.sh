#!/usr/bin/env bash

[ -r .env ] && source .env
rm -f go.sum
go mod tidy
go generate
go fmt ./...
go get -u ./...
