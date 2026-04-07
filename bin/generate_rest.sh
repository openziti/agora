#!/bin/bash

set -euo pipefail

echo "...generating api server and client"
rm -f internal/api/*.go
go generate ./...
