package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"

	"mpd-scrobbler/client"
	"mpd-scrobbler/scrobble"
)

const (
	// polling interval
	sleepTime = 5 * time.Second
)

var (
	config   = flag.String("config", "./config.toml", "path to config file")
	dbPath   = flag.String("db", "./scrobble.db", "path to database for caching")
	host     = flag.String("host", "127.0.0.1", "mpd connection address")
	port     = flag.String("port", "6600", "mpd connection port")
	pass     = flag.String("pass", "", "mpd password")
	duration = flag.Bool("duration", true, "should we send tracks durations?")

	submitTime = flag.Int(
		"submittime",
		client.SubmitTime,
		"time after which track is submitted, in seconds")
	submitPercentage = flag.Int(
		"submitpercentage",
		client.SubmitPercentage,
		"fraction of track after which it is submitted, in percents")
	submitMinDuration = flag.Int(
		"submitminduration",
		client.SubmitMinDuration,
		"minimum submittable track duration, in seconds")
	titleHack = flag.Bool(
		"titlehack",
		client.TitleHack,
		"attempt to extract artist from title if it's not specified")
)

func catchInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	s := <-c
	log.Printf("caught %s: shutting down", s)
}

func init() {
	log.SetFlags(log.Lshortfile)
}

func main() {
	flag.Parse()

	c, err := client.Dial("tcp", *host+":"+*port, *pass)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	c.SubmitTime = *submitTime
	c.SubmitPercentage = *submitPercentage
	c.SubmitMinDuration = *submitMinDuration
	c.TitleHack = *titleHack

	db, err := scrobble.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var conf map[string]map[string]string
	if _, err := toml.DecodeFile(*config, &conf); err != nil {
		log.Fatal(err)
	}

	apis := []scrobble.Scrobbler{}
	for k, v := range conf {
		api, err := scrobble.New(db, k, v["key"], v["secret"], v["username"], v["password"], v["uri"])
		if err != nil {
			log.Fatal(k, " ", err)
		}

		apis = append(apis, api)
	}

	toSubmit := make(chan client.Song)
	nowPlaying := make(chan client.Song)

	quitchan := make(chan struct{})

	var wg sync.WaitGroup

	wg.Add(1)

	go c.Watch(sleepTime, toSubmit, nowPlaying)

	go func() {
		for {
			select {
			case s := <-nowPlaying:
				if !*duration {
					s.Duration = 0
				}
				for _, api := range apis {
					err := api.NowPlaying(
						s.Title,
						s.Artist,
						s.Album,
						s.AlbumArtist,
						s.TrackNumber,
						s.Duration)
					if err != nil {
						log.Printf("[%s] err(NowPlaying): %s\n", api.Name(), err)
					}
				}

			case s := <-toSubmit:
				for _, api := range apis {
					err := api.Scrobble(
						s.Title,
						s.Artist,
						s.Album,
						s.AlbumArtist,
						s.TrackNumber,
						s.Duration,
						s.Start)
					if err != nil {
						log.Printf("[%s] err(Scrobble): %s\n", api.Name(), err)
					}
				}

			case <-quitchan:
				wg.Done()
				return
			}
		}
	}()

	catchInterrupt()

	close(quitchan)
	wg.Wait()
}
