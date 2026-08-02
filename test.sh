#!/bin/sh
set -e
echo "ZIMdex — starting dev server with samples/"
echo "Open http://localhost:3000 in your browser"
echo ""
go run main.go --folder=./samples --port=3000
