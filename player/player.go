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
	"log"
	"os/exec"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	
	"go-discord-music/youtube"
)

type Session struct {
	GuildID      string
	VoiceClient  *discordgo.VoiceConnection
	Queue        []*youtube.Track
	CurrentTrack *youtube.Track
	IsPlaying    bool
	TextChannel  string
	Volume       int
	
	Mu           sync.Mutex
	Stream       *dca.StreamingSession
	stopChan     chan bool
	skipChan     chan bool
}

var (
	Sessions = make(map[string]*Session)
	muMap    sync.Mutex
)

func GetSession(guildID string) *Session {
	muMap.Lock()
	defer muMap.Unlock()
	if sess, exists := Sessions[guildID]; exists {
		return sess
	}
	sess := &Session{
		GuildID:  guildID,
		Queue:    []*youtube.Track{},
		Volume:   5,
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

func (s *Session) AddQueue(track *youtube.Track) {
	s.Mu.Lock()
	s.Queue = append(s.Queue, track)
	s.Mu.Unlock()
}

func (s *Session) ClearQueue() {
	s.Mu.Lock()
	s.Queue = []*youtube.Track{}
	s.Mu.Unlock()
}

func (s *Session) Skip() bool {
	if s.IsPlaying {
		s.skipChan <- true
		return true
	}
	return false
}

func (s *Session) Stop() {
	s.ClearQueue()
	if s.IsPlaying {
		s.stopChan <- true
	}
}

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
	options.Volume = int((float64(s.Volume) / 100.0) * 256)

	target := track.Webpage
	if target == "" {
		target = track.URL
	}

	ytdlp := exec.Command("yt-dlp", "-f", "bestaudio/best", "-q", "-o", "-", target)
	stdout, err := ytdlp.StdoutPipe()
	if err != nil {
		log.Printf("Failed configuring stdout proxy: %v", err)
		return
	}

	if err := ytdlp.Start(); err != nil {
		log.Printf("Failed invoking yt-dlp physical wrapper: %v", err)
		return
	}
	defer func() {
		if ytdlp.Process != nil {
			ytdlp.Process.Kill()
		}
	}()

	encodeSession, err := dca.EncodeMem(stdout, options)
	if err != nil {
		log.Printf("Failed encoding dynamically OPUS map : %v", err)
		return
	}
	defer encodeSession.Cleanup()

	// Discord officially ignores all incoming UDP packets natively unless Speaking is true.
	s.VoiceClient.Speaking(true)
	defer s.VoiceClient.Speaking(false)

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
		} else {
			log.Printf("Track actively completed stream accurately.")
		}
	case <-s.skipChan:
		log.Println("Skipping track dynamically")
	case <-s.stopChan:
		log.Println("Stopping track dynamically")
	}
}
