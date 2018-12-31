package scrobble

import (
	"log"
	"time"

	"github.com/softashell/mpd-scrobbler/scrobble/lastfm"
)

type Scrobbler interface {
	Scrobble(title, artist, album, albumArtist string, trackNumber int32, duration uint32, timestamp time.Time) error
	NowPlaying(title, artist, album, albumArtist string, trackNumber int32, duration uint32) error
	Name() string
}

func New(db Database, name, apiKey, secret, username, password, uriBase string) (Scrobbler, error) {
	api := lastfm.New(apiKey, secret, uriBase)

	queue, err := db.Queue([]byte(name))
	if err != nil {
		return nil, err
	}

	scrobbler := &lastfmScrobbler{api, username, password, false, name}

	log.Printf("[%s] Emptying queue\n", name)
	for {
		track, err := queue.Dequeue()
		if err != nil {
			if err != QUEUE_EMPTY {
				log.Printf("[%s] Dequeue error: %v\n", name, err)
				//queue.Enqueue(track)
				//log.Printf("[%s] Queued: %s by %s\n", name, track.Title, track.Artist)
			}
			break
		}

		err = scrobbler.Scrobble(
			track.Title,
			track.Artist,
			track.Album,
			track.AlbumArtist,
			track.TrackNumber,
			track.Duration,
			track.Timestamp)
		if err != nil {
			log.Printf("[%s] Scrobble error: %v\n", name, err)
			queue.Enqueue(track)
			log.Printf("[%s] Queued: %s by %s\n", name, track.Title, track.Artist)
			break
		}
	}

	log.Printf("[%s] Emptying done\n", name)

	return &queuedScrobbler{scrobbler, queue}, nil
}

type queuedScrobbler struct {
	Scrobbler
	queue Queue
}

func (api *queuedScrobbler) Scrobble(title, artist, album, albumArtist string, trackNumber int32, duration uint32, timestamp time.Time) (err error) {
	track, err := Track{
		Title:       title,
		Artist:      artist,
		Album:       album,
		AlbumArtist: albumArtist,
		TrackNumber: trackNumber,
		Duration:    duration,
		Timestamp:   timestamp,
	}, nil
	for {
		err = api.Scrobbler.Scrobble(
			track.Title,
			track.Artist,
			track.Album,
			track.AlbumArtist,
			track.TrackNumber,
			track.Duration,
			track.Timestamp)
		if err != nil {
			break
		}
		track, err = api.queue.Dequeue()
		if err != nil {
			if err != QUEUE_EMPTY {
				log.Printf("[%s] Dequeue error: %v\n", api.Name(), err)
			}
			return nil
		}
	}

	if err != nil {
		log.Printf("[%s] Scrobble error: %v\n", api.Name(), err)
		api.queue.Enqueue(track)
		log.Printf("[%s] Queued: %s by %s\n", api.Name(), title, artist)
	}

	return err
}

type lastfmScrobbler struct {
	api      *lastfm.Api
	username string
	password string
	loggedIn bool
	name     string
}

func (api *lastfmScrobbler) Name() string {
	return api.name
}

func (api *lastfmScrobbler) login() error {
	if !api.loggedIn {
		err := api.api.Login(api.username, api.password)
		if err == nil {
			log.Printf("[%s] Connected", api.Name())
			api.loggedIn = true
		}
		return err
	}
	return nil
}

func (api *lastfmScrobbler) Scrobble(title, artist, album, albumArtist string, trackNumber int32, duration uint32, timestamp time.Time) error {
	if err := api.login(); err != nil {
		return err
	}

	err := api.api.Scrobble(lastfm.ScrobbleArgs{
		Track:       title,
		Artist:      artist,
		Album:       album,
		AlbumArtist: albumArtist,
		TrackNumber: trackNumber,
		Duration:    duration,
		Timestamp:   timestamp.Unix(),
	})

	if err == nil {
		log.Printf("[%s] Submitted: %s by %s\n", api.Name(), title, artist)
	}

	return err
}

func (api *lastfmScrobbler) NowPlaying(title, artist, album, albumArtist string, trackNumber int32, duration uint32) error {
	if err := api.login(); err != nil {
		return err
	}

	err := api.api.UpdateNowPlaying(lastfm.UpdateNowPlayingArgs{
		Track:       title,
		Artist:      artist,
		Album:       album,
		AlbumArtist: albumArtist,
		TrackNumber: trackNumber,
		Duration:    duration,
	})

	if err == nil {
		log.Printf("[%s] NowPlaying: %s by %s\n", api.Name(), title, artist)
	}

	return err
}
