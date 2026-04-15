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
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"

	"go-discord-music/youtube"
)

// Session holds the entire playback state for a single Discord guild (server).
// One Session exists per guild and is created on first use by GetSession.
type Session struct {
	GuildID        string
	VoiceClient    *discordgo.VoiceConnection // Active WebSocket connection to a voice channel.
	VoiceChannelID string                     // ID of the channel we're connected to; used for auto-reconnect.
	Queue          []*youtube.Track
	CurrentTrack   *youtube.Track
	History        []*youtube.Track  // Tracks that have already played; used by !previous.
	SearchMemory   []*youtube.Track  // Holds the last !search results so the user can pick with !p <number>.
	IsPlaying      bool
	TextChannel    string
	Volume         int // Volume percentage in the range [1, 500]; applied via FFmpeg AudioFilter.

	Mu       sync.Mutex
	Stream   *dca.StreamingSession
	stopChan chan bool
	skipChan chan bool
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
		GuildID:      guildID,
		Queue:        []*youtube.Track{},
		History:      []*youtube.Track{},
		SearchMemory: []*youtube.Track{},
		Volume:       15,
		stopChan: make(chan bool, 1),
		skipChan: make(chan bool, 1),
	}
	Sessions[guildID] = sess
	return sess
}

// Join connects the bot to a Discord voice channel and stores the channel ID
// so the session can auto-reconnect if the voice WebSocket drops between tracks.
func (s *Session) Join(sctx *discordgo.Session, guildID, voiceChannelID string) error {
	// ChannelVoiceJoin is a blocking call; do not hold Mu across it.
	vc, err := sctx.ChannelVoiceJoin(guildID, voiceChannelID, false, false)
	if err != nil {
		return err
	}
	s.Mu.Lock()
	s.VoiceClient = vc
	s.VoiceChannelID = voiceChannelID
	s.Mu.Unlock()
	return nil
}

// reconnect attempts to re-establish the voice WebSocket connection to the channel
// the bot was last joined to. Called automatically when VoiceClient.Ready is false
// at the start of a track (typically after a Discord voice server migration).
//
// Returns an error if VoiceChannelID is not set or ChannelVoiceJoin fails.
func (s *Session) reconnect(sctx *discordgo.Session) error {
	s.Mu.Lock()
	channelID := s.VoiceChannelID
	guildID := s.GuildID
	s.Mu.Unlock()

	if channelID == "" {
		return fmt.Errorf("no voice channel recorded for this session")
	}

	log.Printf("[player] voice not ready — reconnecting to channel %s in guild %s", channelID, guildID)
	vc, err := sctx.ChannelVoiceJoin(guildID, channelID, false, false)
	if err != nil {
		return fmt.Errorf("ChannelVoiceJoin failed: %w", err)
	}

	s.Mu.Lock()
	s.VoiceClient = vc
	s.Mu.Unlock()
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
	s.History = []*youtube.Track{}
	s.SearchMemory = []*youtube.Track{}
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
	s.History = []*youtube.Track{}
	s.SearchMemory = []*youtube.Track{}
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

// Move structurally slices precise sequence payloads natively translating execution tracks sequentially linearly!
func (s *Session) Move(from, to int) (*youtube.Track, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	
	if from < 0 || from >= len(s.Queue) || to < 0 || to >= len(s.Queue) {
		return nil, false
	}
	
	track := s.Queue[from]
	s.Queue = append(s.Queue[:from], s.Queue[from+1:]...)
	s.Queue = append(s.Queue[:to], append([]*youtube.Track{track}, s.Queue[to:]...)...)
	
	return track, true
}

// Remove cleanly directly effectively rips explicitly mapped elements statically natively out of sequence boundaries structurally.
func (s *Session) Remove(idx int) (*youtube.Track, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	
	if idx < 0 || idx >= len(s.Queue) {
		return nil, false
	}
	
	track := s.Queue[idx]
	s.Queue = append(s.Queue[:idx], s.Queue[idx+1:]...)
	
	return track, true
}

// Skip seamlessly writes into the underlying channels mathematically slicing off the active DCA stream securely.
func (s *Session) Skip() bool {
	if s.IsPlaying {
		s.skipChan <- true
		return true
	}
	return false
}

// Previous strictly reverses the array explicitly mapping tracking streams structurally dynamically.
func (s *Session) Previous() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	
	if len(s.History) == 0 {
		return false
	}
	
	// Map the immediate previous payload inherently securely
	prev := s.History[len(s.History)-1]
	s.History = s.History[:len(s.History)-1]
	
	// Reposition the active cleanly back over identically sequentially
	if s.CurrentTrack != nil {
		s.Queue = append([]*youtube.Track{s.CurrentTrack}, s.Queue...)
	}
	
	// Physically prepend the reversed array immediately onto the stack
	s.Queue = append([]*youtube.Track{prev}, s.Queue...)
	s.CurrentTrack = nil 
	
	if s.IsPlaying {
		s.skipChan <- true
	}
	
	return true
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

// sendError sends an error message to the Discord text channel and logs it.
func (s *Session) sendError(sctx *discordgo.Session, msg string) {
	log.Println("[player] error:", msg)
	if sctx != nil && s.TextChannel != "" {
		sctx.ChannelMessageSend(s.TextChannel, "⚠️ "+msg)
	}
}

func (s *Session) PlayQueue(sctx *discordgo.Session) {
	// Guard with mutex to prevent a race between two concurrent callers
	// (e.g. playlist streamer + a !play command arriving simultaneously).
	s.Mu.Lock()
	if s.IsPlaying {
		s.Mu.Unlock()
		return
	}
	s.IsPlaying = true
	s.Mu.Unlock()

	go func() {
		for {
			s.Mu.Lock()
			if len(s.Queue) == 0 || !s.IsPlaying {
				s.IsPlaying = false
				s.CurrentTrack = nil
				s.Mu.Unlock()
				break
			}
			track := s.Queue[0]
			if s.CurrentTrack != nil {
				s.History = append(s.History, s.CurrentTrack)
			}
			s.Queue = s.Queue[1:]
			s.CurrentTrack = track
			s.Mu.Unlock()

			s.playTrack(sctx, track)
		}
	}()
}

func (s *Session) playTrack(sctx *discordgo.Session, track *youtube.Track) {
	// Voice WebSocket can drop between tracks due to Discord server migrations.
	// Attempt an automatic reconnect before downloading anything — it's cheap
	// compared to aborting the queue and requiring the user to !join again.
	if s.VoiceClient == nil || !s.VoiceClient.Ready {
		if sctx != nil && s.TextChannel != "" {
			sctx.ChannelMessageSend(s.TextChannel, "🔄 Voice disconnected — attempting reconnect...")
		}
		if err := s.reconnect(sctx); err != nil {
			s.sendError(sctx, fmt.Sprintf("Could not reconnect to voice (%v) — stopping playback.", err))
			s.Mu.Lock()
			s.IsPlaying = false
			s.CurrentTrack = nil
			s.Queue = []*youtube.Track{}
			s.Mu.Unlock()
			return
		}

		// Give the voice WebSocket a moment to complete the handshake
		// before we check Ready and attempt to send audio.
		time.Sleep(2 * time.Second)

		if !s.VoiceClient.Ready {
			s.sendError(sctx, "Reconnected but voice is still not ready — stopping playback.")
			s.Mu.Lock()
			s.IsPlaying = false
			s.CurrentTrack = nil
			s.Queue = []*youtube.Track{}
			s.Mu.Unlock()
			return
		}

		// Reconnect succeeded — put this track back at the front of the queue
		// and return. PlayQueue's loop will dequeue it and call playTrack again
		// with a live voice connection.
		s.Mu.Lock()
		s.Queue = append([]*youtube.Track{track}, s.Queue...)
		s.CurrentTrack = nil
		s.Mu.Unlock()
		log.Printf("[player] reconnected — retrying: %s", track.Title)
		if sctx != nil && s.TextChannel != "" {
			sctx.ChannelMessageSend(s.TextChannel, fmt.Sprintf("✅ Reconnected — retrying **%s**.", track.Title))
		}
		return
	}

	options := dca.StdEncodeOptions
	options.RawOutput = true
	options.Bitrate = 96
	options.Application = "audio"
	// Set volume=256 so dca passes -vol 256, which our ffmpeg-wrapper strips (it's deprecated
	// in FFmpeg 6+). We apply the actual volume through the AudioFilter instead.
	options.Volume = 256
	options.AudioFilter = fmt.Sprintf("volume=%f", float64(s.Volume)/100.0)

	target := track.Webpage
	if target == "" {
		target = track.URL
	}

	// Download to a local cache file first. Direct piping is unreliable because
	// YouTube frequently blocks ffmpeg's user-agent on streaming URLs.
	if err := os.MkdirAll("cache", os.ModePerm); err != nil {
		s.sendError(sctx, fmt.Sprintf("Cannot create cache directory: %v", err))
		return
	}
	cacheBase := fmt.Sprintf("cache/track_%d", time.Now().UnixNano())

	var ytdlpStderr bytes.Buffer
	ytdlp := exec.Command("yt-dlp", "-f", "bestaudio/best", "-q", "-o", cacheBase+".%(ext)s", target)
	ytdlp.Stderr = &ytdlpStderr
	if err := ytdlp.Run(); err != nil {
		// Extract the most useful line from yt-dlp's stderr output.
		reason := extractYtdlpReason(ytdlpStderr.String())
		s.sendError(sctx, fmt.Sprintf("Cannot download **%s**: %s", track.Title, reason))
		return
	}

	matches, _ := filepath.Glob(cacheBase + ".*")
	if len(matches) == 0 {
		s.sendError(sctx, fmt.Sprintf("yt-dlp produced no output file for **%s**.", track.Title))
		return
	}
	cacheFile := matches[0]
	defer os.Remove(cacheFile)

	dca.Logger = log.New(io.Discard, "", 0)

	encodeSession, err := dca.EncodeFile(cacheFile, options)
	if err != nil {
		s.sendError(sctx, fmt.Sprintf("Failed to encode **%s**: %v", track.Title, err))
		return
	}
	defer encodeSession.Cleanup()

	s.VoiceClient.Speaking(true)
	defer s.VoiceClient.Speaking(false)

	// Brief sleep to let Discord's E2EE (DAVE) key exchange complete before
	// sending the first Opus frame, otherwise the first second is silent.
	time.Sleep(1 * time.Second)

	done := make(chan error)
	stream := dca.NewStream(encodeSession, s.VoiceClient, done)

	s.Mu.Lock()
	s.Stream = stream
	s.Mu.Unlock()

	if s.TextChannel != "" {
		embed := &discordgo.MessageEmbed{
			Title:       "🎵 Now Playing",
			Description: track.Display(),
			Color:       0xFF0000,
			Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: track.Thumbnail},
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Uploader", Value: track.Uploader, Inline: true},
				{Name: "Duration", Value: fmt.Sprintf("%.0f seconds", track.Duration), Inline: true},
			},
			Footer: &discordgo.MessageEmbedFooter{Text: "▶ YouTube"},
		}
		sctx.ChannelMessageSendEmbed(s.TextChannel, embed)
	}

	select {
	case err := <-done:
		if err != nil && err != io.EOF {
			msg := fmt.Sprintf("Playback error for **%s**: %v", track.Title, err)
			ffmpegMsg := encodeSession.FFMPEGMessages()
			if ffmpegMsg != "" {
				msg += fmt.Sprintf("\nFFmpeg: `%s`", strings.TrimSpace(ffmpegMsg))
			}
			s.sendError(sctx, msg)
		} else {
			log.Printf("[player] finished: %s", track.Title)
		}
	case <-s.skipChan:
		log.Printf("[player] skipped: %s", track.Title)
	case <-s.stopChan:
		log.Printf("[player] stopped: %s", track.Title)
	}
}

// extractYtdlpReason picks the most human-readable line from yt-dlp stderr.
func extractYtdlpReason(stderr string) string {
	if stderr == "" {
		return "unknown error (no output from yt-dlp)"
	}
	// yt-dlp error lines start with "ERROR:" — grab the first one.
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ERROR:") {
			return strings.TrimPrefix(line, "ERROR: ")
		}
	}
	// Fall back to the last non-empty line.
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return "unknown error"
}
