package client

import (
	"log"
	"regexp"
	"strconv"
	"sync"
	"time"

	"mpd-scrobbler/client/mpd"
)

const (
	// only submit if played for submitTime second or submitPercentage of length
	SubmitTime        = 240 // 4 minutes
	SubmitPercentage  = 50  // 50%
	SubmitMinDuration = 30  // 30 seconds
	TitleHack         = false
)

type Client struct {
	client            *mpd.Client
	lock              sync.Mutex
	net               string
	addr              string
	pass              string
	song              mpd.Song
	pos               mpd.Pos
	start             int // stats curtime
	starttime         time.Time
	submitted         bool
	quit              chan struct{}
	SubmitTime        int
	SubmitPercentage  int
	SubmitMinDuration int
	TitleHack         bool
}

func newClient(net, addr, pass string) (c *mpd.Client, e error) {
	if pass == "" {
		c, e = mpd.Dial(net, addr)
	} else {
		c, e = mpd.DialAuthenticated(net, addr, pass)
	}
	if e != nil {
		c.Close()
		c = nil
	}
	return
}

func Dial(net, addr, pass string) (*Client, error) {
	c, err := newClient(net, addr, pass)
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
		SubmitTime:        SubmitTime,
		SubmitPercentage:  SubmitPercentage,
		SubmitMinDuration: SubmitMinDuration,
		TitleHack:         TitleHack,
	}

	go client.keepalive()
	return client, nil
}

func (c *Client) keepalive() {
	var err error
	for {
		select {
		case <-time.After(15 * time.Second):
			c.lock.Lock()
			err = c.client.Ping()
			c.lock.Unlock()

			if err != nil {
				log.Println("reconnecting because ping failed:", err)

				cc, err := newClient(c.net, c.addr, c.pass)
				if err != nil {
					log.Println("reconnection fail:", err)
				} else {
					c.lock.Lock()

					c.client.Close()
					c.client = cc

					c.lock.Unlock()
				}
			}

		case <-c.quit:
			return
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
	var song mpd.Song
	var pos mpd.Pos
	var playtime int
	var playing bool
	var err error

	r := regexp.MustCompile("(.+) - (.+)")

	for _ = range time.Tick(interval) {
		c.lock.Lock()

		pos, playing, err = c.client.CurrentPos()
		if !playing {
			goto nextr
		}
		if err != nil {
			log.Println("err(CurrentPos):", err)
			goto nextr
		}

		playtime, err = c.client.PlayTime()
		if err != nil {
			log.Println("err(PlayTime):", err)
			goto nextr
		}

		song, err = c.client.CurrentSong()
		if err != nil {
			log.Println("err(CurrentSong):", err)
			goto nextr
		}

		c.lock.Unlock()

		if song.Album == "" && song.Title != "" && c.TitleHack {
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

		continue

	nextr:
		c.lock.Unlock()
	}
}

func (c *Client) canSubmit(playtime int) bool {
	if c.submitted ||
		c.pos.Length < c.SubmitMinDuration ||
		c.song.Title == "" || c.song.Artist == "" {
		return false
	}
	if c.pos.Length > 0 {
		return playtime-c.start >= c.SubmitTime ||
			float64(playtime-c.start) >= (float64(c.pos.Length)*float64(c.SubmitPercentage))/100
	}
	return playtime-c.start >= c.SubmitTime
}
