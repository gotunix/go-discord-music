// SPDX-License-Identifier: AGPL-3.0-or-later
// SPDX-FileCopyrightText: 2026 GOTUNIX Networks <code@gotunix.net>
// SPDX-FileCopyrightText: 2026 Justin Ovens <code@gotunix.net>
// ----------------------------------------------------------------------------------------------- //
//                 $$$$$$\   $$$$$$\ $$$$$$$$\ $$\   $$\ $$\   $$\ $$$$$$\ $$\   $$\               //
//                $$  __$$\ $$  __$$\\__$$  __|$$ |  $$ |$$$\  $$ |\_$$  _|$$ |  $$ |              //
//                $$ /  \__|$$ /  $$ |  $$ |   $$ |  $$ |$$$$\ $$ |  $$ |  \$$\ $$  |              //
//                $$ |$$$$\ $$ |  $$ |  $$ |   $$ |  $$ |$$ $$\$$ |  $$ |   \$$$$  /               //
//                $$ |\_$$ |$$ |  $$ |  $$ |   $$ |  $$ |$$ \$$$$ |  $$ |   $$  $$<                //
//                $$ |  $$ |$$ |  $$ |  $$ |   $$ |  $$ |$$ |\$$$ |  $$ |  $$  /\$$\               //
//                \$$$$$$  | $$$$$$  |  $$ |   \$$$$$$  |$$ | \$$ |$$$$$$\ $$ /  $$ |              //
//                 \______/  \______/   \__|    \______/ \__|  \__|\______|\__|  \__|              //
// ----------------------------------------------------------------------------------------------- //
// Copyright (C) GOTUNIX Networks                                                                  //
// Copyright (C) Justin Ovens                                                                      //
// ----------------------------------------------------------------------------------------------- //
// This program is free software: you can redistribute it and/or modify                            //
// it under the terms of the GNU Affero General Public License as                                  //
// published by the Free Software Foundation, either version 3 of the                              //
// License, or (at your option) any later version.                                                 //
//                                                                                                 //
// This program is distributed in the hope that it will be useful,                                 //
// but WITHOUT ANY WARRANTY; without even the implied warranty of                                  //
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the                                   //
// GNU Affero General Public License for more details.                                             //
//                                                                                                 //
// You should have received a copy of the GNU Affero General Public License                        //
// along with this program.  If not, see <https://www.gnu.org/licenses/>.                          //
// ----------------------------------------------------------------------------------------------- //

package player

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os/exec"
	"sync"
	"os"
	"time"
	"path/filepath"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	
	"go-discord-music/youtube"
)

// Session represents an entirely localized physical boundary cleanly tracking a single Discord server's execution state explicitly.
type Session struct {
	GuildID      string
	VoiceClient  *discordgo.VoiceConnection // Physical WebSocket mapping block to the Voice Channel natively.
	Queue        []*youtube.Track
	CurrentTrack *youtube.Track // Currently parsing track loop directly.
	IsPlaying    bool
	TextChannel  string
	Volume       int // Native integer [0, 500] explicitly mapping FFMPEG audio output percentages!
	
	Mu           sync.Mutex
	Stream       *dca.StreamingSession
	stopChan     chan bool
	skipChan     chan bool
}

var (
	Sessions = make(map[string]*Session)
	muMap    sync.Mutex
)

// GetSession extracts the localized physical memory pointer for a specific server (GuildID) inherently preventing all singleton block logic completely!
func GetSession(guildID string) *Session {
	muMap.Lock()
	defer muMap.Unlock()
	if sess, exists := Sessions[guildID]; exists {
		return sess
	}
	sess := &Session{
		GuildID:  guildID,
		Queue:    []*youtube.Track{},
		Volume:   100,
		stopChan: make(chan bool, 1),
		skipChan: make(chan bool, 1),
	}
	Sessions[guildID] = sess
	return sess
}

func (s *Session) Join(sctx *discordgo.Session, guildID, voiceChannelID string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	
	vc, err := sctx.ChannelVoiceJoin(guildID, voiceChannelID, false, false)
	if err != nil {
		return err
	}
	s.VoiceClient = vc
	return nil
}

func (s *Session) Leave() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	
	if s.IsPlaying {
		s.stopChan <- true
	}
	
	if s.VoiceClient != nil {
		s.VoiceClient.Disconnect()
		s.VoiceClient = nil
	}
	s.IsPlaying = false
	s.Queue = []*youtube.Track{}
	s.CurrentTrack = nil
}

// AddQueue mathematically appends a newly extracted YouTube payload strictly inside the server boundary completely.
func (s *Session) AddQueue(track *youtube.Track) {
	s.Mu.Lock()
	s.Queue = append(s.Queue, track)
	s.Mu.Unlock()
}

// ClearQueue completely nukes all subsequent array elements directly.
func (s *Session) ClearQueue() {
	s.Mu.Lock()
	s.Queue = []*youtube.Track{}
	s.Mu.Unlock()
}

// ShuffleQueue organically randomizes the elements residing within the executing Queue securely across memory.
func (s *Session) ShuffleQueue() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if len(s.Queue) > 1 {
		rand.Shuffle(len(s.Queue), func(i, j int) {
			s.Queue[i], s.Queue[j] = s.Queue[j], s.Queue[i]
		})
	}
}

// Skip seamlessly writes into the underlying channels mathematically slicing off the active DCA stream securely.
func (s *Session) Skip() bool {
	if s.IsPlaying {
		s.skipChan <- true
		return true
	}
	return false
}

// Stop cleanly cuts execution completely natively terminating the queue mappings seamlessly.
func (s *Session) Stop() {
	s.ClearQueue()
	if s.IsPlaying {
		s.stopChan <- true
	}
}

// SetPaused strictly bridges execution into the raw DCA frame array statically pausing delivery into UDP channels implicitly.
func (s *Session) SetPaused(pause bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.Stream != nil {
		s.Stream.SetPaused(pause)
	}
}

func (s *Session) PlayQueue(sctx *discordgo.Session) {
	if s.IsPlaying {
		return
	}
	s.IsPlaying = true

	go func() {
		for {
			s.Mu.Lock()
			if len(s.Queue) == 0 {
				s.IsPlaying = false
				s.CurrentTrack = nil
				s.Mu.Unlock()
				break
			}
			track := s.Queue[0]
			s.Queue = s.Queue[1:]
			s.CurrentTrack = track
			s.Mu.Unlock()

			s.playTrack(sctx, track)
		}
	}()
}

func (s *Session) playTrack(sctx *discordgo.Session, track *youtube.Track) {
	if s.VoiceClient == nil || !s.VoiceClient.Ready {
		log.Println("Voice not ready")
		return
	}

	options := dca.StdEncodeOptions
	options.RawOutput = true
	options.Bitrate = 96
	options.Application = "audio"

	// Native fix: The dca library natively passes -vol which is entirely deprecated in FFMPEG 8.0+.
	// We force the structural map to skip `-vol` by mapping 256 organically, and seamlessly inject the modern AudioFilter syntax instead!
	options.Volume = 256
	options.AudioFilter = fmt.Sprintf("volume=%f", float64(s.Volume)/100.0)

	target := track.Webpage
	if target == "" {
		target = track.URL
	}

	// Native fix: Youtube blocks FFMPEG standard pipes randomly without robust reconnect headers.
	// Bypassing directly by cleanly archiving to a local cache volume just like the python structure natively did!
	os.MkdirAll("cache", os.ModePerm)
	cacheBase := fmt.Sprintf("cache/track_%d", time.Now().UnixNano())

	ytdlp := exec.Command("yt-dlp", "-f", "bestaudio/best", "-q", "-o", cacheBase+".%(ext)s", target)
	if err := ytdlp.Run(); err != nil {
		log.Printf("yt-dlp explicitly failed downloading structural payload to cache: %v", err)
		return
	}
	
	matches, _ := filepath.Glob(cacheBase + ".*")
	if len(matches) == 0 {
		log.Printf("Failed explicitly mapping yt-dlp formatted extension map entirely!")
		return
	}
	cacheFile := matches[0]

	// Clean up local footprint upon completion automatically!
	defer os.Remove(cacheFile)

	// Explicitly completely muzzle legacy DCA telemetry structural logging completely!
	dca.Logger = log.New(io.Discard, "", 0)

	encodeSession, err := dca.EncodeFile(cacheFile, options)
	if err != nil {
		log.Printf("Failed encoding dynamically OPUS map : %v", err)
		return
	}
	defer encodeSession.Cleanup()

	// Discord officially ignores all incoming UDP packets natively unless Speaking is true.
	s.VoiceClient.Speaking(true)
	defer s.VoiceClient.Speaking(false)

	// DAVE (E2EE) mathematically requires a minor stabilization window cleanly intercepting initial keys!
	time.Sleep(1 * time.Second)

	done := make(chan error)
	stream := dca.NewStream(encodeSession, s.VoiceClient, done)

	s.Mu.Lock()
	s.Stream = stream
	s.Mu.Unlock()

	if s.TextChannel != "" {
		embed := &discordgo.MessageEmbed{
			Title: "🎵 Now Playing",
			Description: track.Display(),
			Color: 0xFF0000,
			Thumbnail: &discordgo.MessageEmbedThumbnail{URL: track.Thumbnail},
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Uploader", Value: track.Uploader, Inline: true},
				{Name: "Duration", Value: fmt.Sprintf("%.0f seconds", track.Duration), Inline: true},
			},
			Footer: &discordgo.MessageEmbedFooter{Text: "▶ YouTube • Golang Native Edition"},
		}
		sctx.ChannelMessageSendEmbed(s.TextChannel, embed)
	}

	select {
	case err := <-done:
		if err != nil {
			log.Printf("FFMPEG pipeline exited natively with error: %v", err)
			log.Printf("FFMPEG STDE: %s", encodeSession.FFMPEGMessages())
		} else {
			log.Printf("Track actively completed stream accurately.")
		}
	case <-s.skipChan:
		log.Println("Skipping track dynamically")
	case <-s.stopChan:
		log.Println("Stopping track dynamically")
	}
}
