#!/bin/sh
GOPATH="$PWD" exec goimports -local 'mpd-scrobbler/' -w *.go src/mpd-scrobbler
