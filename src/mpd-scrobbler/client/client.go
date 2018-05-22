package client

import (
	"log"
	"regexp"
	"time"

	"mpd-scrobbler/client/mpd"
)

const (
	// only submit if played for submitTime second or submitPercentage of length
	submitTime       = 30
	submitPercentage = 50
)

type Client struct {
	client    *mpd.Client
	song      mpd.Song
	pos       mpd.Pos
	start     int // stats curtime
	starttime time.Time
	submitted bool
	quit      chan struct{}
}

func Dial(network, addr string) (*Client, error) {
	c, err := mpd.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	client := &Client{
		client:    c,
		song:      mpd.Song{},
		pos:       mpd.Pos{},
		start:     0,
		starttime: time.Now(),
		submitted: false,
		quit:      make(chan struct{}),
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

func (c *Client) Close() error {
	close(c.quit)
	return c.client.Close()
}

func (c *Client) Song() Song {
	return Song{
		Album:       c.song.Album,
		Artist:      c.song.Artist,
		AlbumArtist: c.song.AlbumArtist,
		Title:       c.song.Title,
		Start:       c.starttime,
	}
}

func (c *Client) Watch(interval time.Duration, toSubmit chan Song, nowPlaying chan Song) {
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
		if song != c.song {
			c.song = song
			c.pos = pos
			c.start = playtime
			c.starttime = time.Now().UTC()

			c.submitted = false
			nowPlaying <- c.Song()
		}

		// still playing
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
	if c.submitted || c.song.Artist == "" || c.song.Title == "" {
		return false
	}
	if c.pos.Length > 0 {
		return playtime-c.start >= submitTime ||
			playtime-c.start >= c.pos.Length/(100/submitPercentage)
	}
	return playtime-c.start >= submitTime
}
