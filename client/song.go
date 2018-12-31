package client

import "time"

type Song struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	TrackNumber int32
	Duration    uint32
	Start       time.Time
}
