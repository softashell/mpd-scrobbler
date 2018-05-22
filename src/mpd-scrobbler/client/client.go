package client

import (
	"log"
	"regexp"
	"strconv"
	"time"

	"mpd-scrobbler/client/mpd"
)

const (
	// only submit if played for submitTime second or submitPercentage of length
	submitTime        = 240 // 4 minutes
	submitPercentage  = 50  // 50%
	submitMinDuration = 30  // 30 seconds
)

type Client struct {
	client            *mpd.Client
	song              mpd.Song
	pos               mpd.Pos
	start             int // stats curtime
	starttime         time.Time
	submitted         bool
	quit              chan struct{}
	SubmitTime        int
	SubmitPercentage  int
	SubmitMinDuration int
}

func Dial(network, addr string) (*Client, error) {
	c, err := mpd.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	client := &Client{
		client:            c,
		song:              mpd.Song{},
		pos:               mpd.Pos{},
		start:             0,
		starttime:         time.Now(),
		submitted:         false,
		quit:              make(chan struct{}),
		SubmitTime:        submitTime,
		SubmitPercentage:  submitPercentage,
		SubmitMinDuration: submitMinDuration,
	}

	go client.keepalive()
	return client, nil
}

func (c *Client) keepalive() {
	for {
		select {
		case <-time.After(30 * time.Second):
			if err := c.client.Ping(); err != nil {
				// reopen connection?
			}
		case <-c.quit:
			break
		}
	}
}

func songsEqual(a, b mpd.Song) bool {
	return a.File == b.File &&
		a.Title == b.Title &&
		a.Artist == b.Artist &&
		a.Album == b.Album &&
		a.AlbumArtist == b.AlbumArtist
}

func (c *Client) Close() error {
	close(c.quit)
	return c.client.Close()
}

func (c *Client) Song() Song {
	tracknum, err := strconv.ParseUint(c.song.Track, 10, 32)
	if err != nil {
		tracknum = 0
	}
	durationf, err := strconv.ParseFloat(c.song.Duration, 64)
	if err != nil || durationf < 0.0 {
		durationf = 0.0
	}
	return Song{
		Title:       c.song.Title,
		Album:       c.song.Album,
		Artist:      c.song.Artist,
		AlbumArtist: c.song.AlbumArtist,
		TrackNumber: uint32(tracknum),
		Duration:    uint32(durationf + 0.5),
		Start:       c.starttime,
	}
}

func (c *Client) Watch(interval time.Duration, toSubmit chan<- Song, nowPlaying chan<- Song) {
	r := regexp.MustCompile("(.+) - (.+)")
	for _ = range time.Tick(interval) {
		pos, playing, err := c.client.CurrentPos()
		if !playing {
			continue
		}
		if err != nil {
			log.Println("err(CurrentPos):", err)
			continue
		}

		playtime, err := c.client.PlayTime()
		if err != nil {
			log.Println("err(PlayTime):", err)
			continue
		}

		song, err := c.client.CurrentSong()
		if err != nil {
			log.Println("err(CurrentSong):", err)
			continue
		}

		if song.Album == "" && song.Title != "" {
			matches := r.FindStringSubmatch(song.Title)
			if matches != nil {
				song.Artist = matches[1]
				song.Title = matches[2]
			}
		}

		// new song
		if !songsEqual(song, c.song) {
			c.song = song
			c.pos = pos
			c.start = playtime
			c.starttime = time.Now().UTC()

			c.submitted = false
			nowPlaying <- c.Song()
		}

		// new playtime
		if pos != c.pos {
			c.pos = pos
			if c.canSubmit(playtime) {
				c.submitted = true
				toSubmit <- c.Song()
			}
		}
	}
}

func (c *Client) canSubmit(playtime int) bool {
	if c.submitted ||
		c.pos.Length < c.SubmitMinDuration ||
		c.song.Title == "" || c.song.Artist == "" {
		return false
	}
	if c.pos.Length > 0 {
		return playtime-c.start >= submitTime ||
			float64(playtime-c.start) >= (float64(c.pos.Length)*submitPercentage)/100
	}
	return playtime-c.start >= submitTime
}
