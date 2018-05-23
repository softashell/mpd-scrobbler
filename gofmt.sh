#!/bin/sh
GOPATH="$PWD" exec goimports -local 'mpd-scrobbler/' -w src/mpd-scrobbler
