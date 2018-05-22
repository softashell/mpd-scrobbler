#!/bin/sh

echo "Downloading dependencies..." >&2
go get -d -v

for file in *.go
do
	echo "Building: $file" >&2
	go build "$file"
done
