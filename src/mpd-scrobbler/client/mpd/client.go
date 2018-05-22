// Copyright 2009 The GoMPD Authors. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

// Package mpd provides the client side interface to MPD (Music Player Daemon).
// The protocol reference can be found at http://www.musicpd.org/doc/protocol/index.html
package mpd

import (
	"fmt"
	"net/textproto"
	"strconv"
	"strings"
)

// Quote quotes strings in the format understood by MPD.
// See: http://git.musicpd.org/cgit/master/mpd.git/tree/src/util/Tokenizer.cxx
func quote(s string) string {
	q := make([]byte, 2+2*len(s))
	i := 0
	q[i], i = '"', i+1
	for _, c := range []byte(s) {
		if c == '"' {
			q[i], i = '\\', i+1
			q[i], i = '"', i+1
		} else {
			q[i], i = c, i+1
		}
	}
	q[i], i = '"', i+1
	return string(q[:i])
}

// Client represents a client connection to a MPD server.
type Client struct {
	text *textproto.Conn
}

// Attrs is a set of attributes returned by MPD.
type Attrs map[string]string

// Dial connects to MPD listening on address addr (e.g. "127.0.0.1:6600")
// on network network (e.g. "tcp").
func Dial(network, addr string) (c *Client, err error) {
	text, err := textproto.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	line, err := text.ReadLine()
	if err != nil {
		return nil, err
	}
	if line[0:6] != "OK MPD" {
		return nil, textproto.ProtocolError("no greeting")
	}
	return &Client{text: text}, nil
}

// DialAuthenticated connects to MPD listening on address addr (e.g. "127.0.0.1:6600")
// on network network (e.g. "tcp"). It then authenticates with MPD
// using the plaintext password password if it's not empty.
func DialAuthenticated(network, addr, password string) (c *Client, err error) {
	c, err = Dial(network, addr)
	if err == nil && len(password) > 0 {
		err = c.Command("password %s", password).OK()
	}
	return c, err
}

// We are reimplemeting Cmd() and PrintfLine() from textproto here, because
// the original functions append CR-LF to the end of commands. This behavior
// violates the MPD protocol: Commands must be terminated by '\n'.
func (c *Client) cmd(format string, args ...interface{}) (uint, error) {
	id := c.text.Next()
	c.text.StartRequest(id)
	defer c.text.EndRequest(id)
	if err := c.printfLine(format, args...); err != nil {
		return 0, err
	}
	return id, nil
}

func (c *Client) printfLine(format string, args ...interface{}) error {
	fmt.Fprintf(c.text.W, format, args...)
	c.text.W.WriteByte('\n')
	return c.text.W.Flush()
}

// Close terminates the connection with MPD.
func (c *Client) Close() (err error) {
	if c.text != nil {
		c.printfLine("close")
		err = c.text.Close()
		c.text = nil
	}
	return
}

// Ping sends a no-op message to MPD. It's useful for keeping the connection alive.
func (c *Client) Ping() error {
	return c.Command("ping").OK()
}

func (c *Client) readList(key string) (list []string, err error) {
	list = []string{}
	key += ": "
	for {
		line, err := c.text.ReadLine()
		if err != nil {
			return nil, err
		}
		if line == "OK" {
			break
		}
		if !strings.HasPrefix(line, key) {
			return nil, textproto.ProtocolError("unexpected: " + line)
		}
		list = append(list, line[len(key):])
	}
	return
}

func (c *Client) readAttrs(terminator string) (attrs Attrs, err error) {
	attrs = make(Attrs)
	for {
		line, err := c.text.ReadLine()
		if err != nil {
			return nil, err
		}
		if line == terminator {
			break
		}
		z := strings.Index(line, ": ")
		if z < 0 {
			return nil, textproto.ProtocolError("can't parse line: " + line)
		}
		key := line[0:z]
		attrs[key] = line[z+2:]
	}
	return
}

type Song struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Track       string // tracknumber
	File        string // filename
	Duration    string // in seconds, float pt value
}

type Pos struct {
	Percent float64
	Seconds int // how much we listened of it
	Length  int // total track length
}

// CurrentSong returns information about the current song in the playlist.
func (c *Client) CurrentSong() (Song, error) {
	s, err := c.Command("currentsong").Attrs()
	if err != nil {
		return Song{}, nil
	}

	return Song{
		Title:       s["Title"],
		Artist:      s["Artist"],
		Album:       s["Album"],
		AlbumArtist: s["AlbumArtist"],
		Track:       s["Track"],
		File:        s["file"],
		Duration:    s["duration"],
	}, nil
}

// Status returns information about the current status of MPD.
func (c *Client) Status() (Attrs, error) {
	return c.Command("status").Attrs()
}

func (c *Client) CurrentPos() (pos Pos, playing bool, err error) {
	var st Attrs
	st, err = c.Status()
	if err != nil {
		return
	}

	if st["volume"] == "-1" || st["state"] != "play" {
		playing = false
		return
	}
	playing = true

	parts := strings.Split(st["time"], ":")
	pos.Seconds, err = strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	pos.Length, err = strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	pos.Percent = float64(pos.Seconds) * 100 / float64(pos.Length)
	return
}

// Stats displays statistics (number of artists, songs, playtime, etc)
func (c *Client) Stats() (Attrs, error) {
	return c.Command("stats").Attrs()
}

func (c *Client) PlayTime() (int, error) {
	s, err := c.Stats()
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(s["playtime"])
}

func (c *Client) readOKLine(terminator string) (err error) {
	line, err := c.text.ReadLine()
	if err != nil {
		return
	}
	if line == terminator {
		return nil
	}
	return textproto.ProtocolError("unexpected response: " + line)
}

func (c *Client) idle(subsystems ...string) ([]string, error) {
	return c.Command("idle %s", Quoted(strings.Join(subsystems, " "))).Strings("changed")
}

func (c *Client) noIdle() (err error) {
	id, err := c.cmd("noidle")
	if err == nil {
		c.text.StartResponse(id)
		c.text.EndResponse(id)
	}
	return
}
