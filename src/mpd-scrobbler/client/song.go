package client

import "time"

type Song struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	TrackNumber uint32
	Duration    uint32
	Start       time.Time
}
