package client

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/softashell/mpd-scrobbler/client/mpd"
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
	start             int // playtime when current track started
	playtime          int // last playtime
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
	if e != nil && c != nil {
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
		net:               net,
		addr:              addr,
		pass:              pass,
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
		case <-time.After(30 * time.Second):
			c.lock.Lock()

			err = c.client.Ping()
			if err != nil {
				log.Println("ping failed:", err)
			}

			c.lock.Unlock()

		case <-time.After(1 * time.Second):
			c.lock.Lock()
			closed := c.client.Closed
			c.lock.Unlock()

			if closed {
				log.Println("detected closed socket, reconnecting")

				cc, err := newClient(c.net, c.addr, c.pass)
				if err != nil {
					log.Println("reconnection fail:", err)
					time.Sleep(5 * time.Second)
				} else {
					c.lock.Lock()

					c.client.Close()
					c.client = cc

					log.Println("successfully reconnected")

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
	var (
		err       error
		tracknumu uint64
		tracknum  int32
		durationf float64
	)

	trnum := c.song.Track
	// handle `num/num` format
	if i := strings.IndexByte(c.song.Track, '/'); i >= 0 {
		trnum = c.song.Track[:i]
	}
	if trnum != "" {
		tracknumu, err = strconv.ParseUint(trnum, 10, 32)
		tracknum = int32(tracknumu)
		if err != nil || tracknum < 0 {
			tracknum = -1
		}
	} else {
		tracknum = -1
	}
	durationf, err = strconv.ParseFloat(c.song.Duration, 64)
	if err != nil || durationf < 0.0 {
		durationf = 0.0
	}
	return Song{
		Title:       c.song.Title,
		Album:       c.song.Album,
		Artist:      c.song.Artist,
		AlbumArtist: c.song.AlbumArtist,
		TrackNumber: tracknum,
		Duration:    uint32(durationf + 0.5),
		Start:       c.starttime,
	}
}

func (c *Client) flushCurrent(toSubmit chan<- Song) {
	if c.canSubmit() {
		c.submitted = true
		toSubmit <- c.Song()
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
		// quit signal check
		select {
		case <-c.quit:
			return
		default:
		}

		c.lock.Lock()

		pos, playing, err = c.client.CurrentPos()
		if !playing {
			goto nocurrent
		}
		if err != nil {
			log.Println("err(CurrentPos):", err)
			goto nocurrent
		}

		playtime, err = c.client.PlayTime()
		if err != nil {
			log.Println("err(PlayTime):", err)
			goto nocurrent
		}

		song, err = c.client.CurrentSong()
		if err != nil {
			log.Println("err(CurrentSong):", err)
			goto nocurrent
		}

		c.lock.Unlock()

		if song.Artist == "" && song.AlbumArtist != "" {
			song.Artist = song.AlbumArtist
		}
		if song.Artist == "" && song.Title != "" && c.TitleHack {
			matches := r.FindStringSubmatch(song.Title)
			if matches != nil {
				song.Artist = matches[1]
				song.Title = matches[2]
			}
		}

		// new song
		if !songsEqual(song, c.song) {
			c.flushCurrent(toSubmit)

			c.song = song
			c.pos = pos
			c.start = playtime
			c.starttime = time.Now().UTC()

			c.submitted = false
			nowPlaying <- c.Song()
		} else if playtime < c.playtime {
			// server was prolly restarted. it normally cannot go back in time.
			// shift start playtime
			c.start -= c.playtime - playtime
		}

		c.playtime = playtime

		// more progress
		if pos != c.pos {
			if pos.Seconds < c.pos.Seconds {
				// new position is smaller. user seeked back or repeated track
				if c.submitted || c.canSubmit() {
					if !c.submitted {
						toSubmit <- c.Song()
					}
					// allow to relisten, if it's already submitted
					c.submitted = false
					c.starttime = time.Now().UTC()
					// reset start position, so that relisten will be calculated properly
					c.start = playtime
					// incase of relistens...
					nowPlaying <- c.Song()
				} else {
					// not yet submitted, so increase c.start by ammount of time jumped to past
					c.start += c.pos.Seconds - pos.Seconds
					// but don't make it worse than fresh listen
					if c.start > playtime {
						c.start = playtime
					}
				}
			}
			c.pos = pos
		}

		continue

	nocurrent:
		c.lock.Unlock()

		c.flushCurrent(toSubmit)
	}
}

func (c *Client) canSubmit() bool {
	// XXX live streams? c.pos.Length is prolly 0 on these
	if c.submitted ||
		c.pos.Length < c.SubmitMinDuration ||
		c.song.Title == "" || c.song.Artist == "" {
		return false
	}
	if c.pos.Length > 0 {
		return c.playtime-c.start >= c.SubmitTime ||
			float64(c.playtime-c.start) >= (float64(c.pos.Length)*float64(c.SubmitPercentage))/100
	}
	return c.playtime-c.start >= c.SubmitTime
}
