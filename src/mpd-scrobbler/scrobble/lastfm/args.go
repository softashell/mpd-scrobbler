package lastfm

import "strconv"

type Args interface {
	Format() map[string]string
}

type ScrobbleArgs struct {
	Artist      string
	Track       string
	Album       string
	AlbumArtist string
	TrackNumber uint32
	Duration    uint32
	Timestamp   int64
}

func (a ScrobbleArgs) Format() map[string]string {
	m := map[string]string{
		"track":       a.Track,
		"artist":      a.Artist,
		"album":       a.Album,
		"albumArtist": a.AlbumArtist,
		"timestamp":   strconv.FormatInt(a.Timestamp, 10),
	}
	if a.TrackNumber > 0 {
		m["trackNumber"] = strconv.FormatUint(uint64(a.TrackNumber), 10)
	}
	if a.Duration > 0 {
		m["duration"] = strconv.FormatUint(uint64(a.Duration), 10)
	}
	return m
}

type UpdateNowPlayingArgs struct {
	Artist      string
	Track       string
	Album       string
	AlbumArtist string
	TrackNumber uint32
	Duration    uint32
}

func (a UpdateNowPlayingArgs) Format() map[string]string {
	m := map[string]string{
		"track":       a.Track,
		"artist":      a.Artist,
		"album":       a.Album,
		"albumArtist": a.AlbumArtist,
	}
	if a.TrackNumber > 0 {
		m["trackNumber"] = strconv.FormatUint(uint64(a.TrackNumber), 10)
	}
	if a.Duration > 0 {
		m["duration"] = strconv.FormatUint(uint64(a.Duration), 10)
	}
	return m
}

type LoginArgs struct {
	Username string
	Password string
}

func (a LoginArgs) Format() map[string]string {
	return map[string]string{
		"username": a.Username,
		"password": a.Password,
	}
}
