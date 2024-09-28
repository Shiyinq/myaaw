#!/bin/bash

mkdir -p docs/swagger

swag init -g ./cmd/server/main.go --parseDependency --parseInternal --output docs/swagger