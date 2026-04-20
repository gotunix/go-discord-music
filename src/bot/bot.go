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

package bot

import (
	"fmt"
	"math/rand"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"go-discord-music/config"
	"go-discord-music/player"
	"go-discord-music/youtube"
)

func OnMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, config.CommandPrefix) {
		return
	}

	args := strings.Split(m.Content, " ")
	cmd := strings.TrimPrefix(args[0], config.CommandPrefix)

	sess := player.GetSession(m.GuildID)
	sess.TextChannel = m.ChannelID

	switch cmd {
	case "play", "p":
		cmdPlay(s, m, args, sess, true)
	case "playing", "np":
		cmdPlaying(s, m, sess)
	case "search":
		cmdSearch(s, m, args, sess)
	case "skip", "s", "next":
		cmdSkip(s, m, sess)
	case "previous", "prev":
		cmdPrevious(s, m, sess)
	case "shuffle":
		cmdShuffle(s, m, sess)
	case "move", "m":
		cmdMove(s, m, args, sess)
	case "pause":
		cmdPause(s, m, sess)
	case "resume":
		cmdResume(s, m, sess)
	case "stop":
		cmdStop(s, m, sess)
	case "queue", "q":
		cmdQueue(s, m, args, sess)
	case "remove", "r", "rm":
		cmdRemove(s, m, args, sess)
	case "clear":
		cmdClear(s, m, sess)
	case "savequeue":
		cmdSaveQueue(s, m, args, sess)
	case "loadqueue":
		cmdLoadQueue(s, m, args, sess)
	case "savedplaylists":
		cmdSavedPlaylists(s, m, sess)
	case "volume", "vol", "v":
		cmdVolume(s, m, args, sess)
	case "join":
		cmdJoin(s, m, sess)
	case "leave":
		cmdLeave(s, m, sess)
	case "help", "h":
		cmdHelp(s, m)
	case "version":
		cmdVersion(s, m)
	}
}

func OnVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	if v.BeforeUpdate == nil {
		return
	}

	// We only care if someone leaves a channel
	if v.BeforeUpdate.ChannelID == "" || (v.VoiceState != nil && v.BeforeUpdate.ChannelID == v.ChannelID) {
		return
	}

	guildID := v.GuildID
	sess := player.GetSession(guildID)

	sess.Mu.Lock()
	vcID := sess.VoiceChannelID
	sess.Mu.Unlock()

	if vcID == "" || v.BeforeUpdate.ChannelID != vcID {
		return
	}

	// Someone left our channel. Check if anyone is left.
	g, err := s.State.Guild(guildID)
	if err != nil {
		return
	}

	count := 0
	for _, vs := range g.VoiceStates {
		if vs.ChannelID == vcID {
			count++
		}
	}

	// If count is 1, it means only the bot is left.
	if count <= 1 {
		sess.SaveCurrentState()
		sess.Leave()
		if sess.TextChannel != "" {
			s.ChannelMessageSend(sess.TextChannel, "💤 Everyone left the voice channel. Pausing and saving queue.")
		}
	}
}

// trackExceedsLimit reports whether a track's duration exceeds the configured
// maximum. Tracks with Duration == 0 (unknown) are always allowed through to
// avoid incorrectly skipping live streams or tracks yt-dlp couldn't measure.
func trackExceedsLimit(t *youtube.Track) bool {
	if config.MaxTrackDuration <= 0 || t.Duration <= 0 {
		return false
	}
	return t.Duration > config.MaxTrackDuration
}

// fmtDuration formats a duration in seconds as m:ss or h:mm:ss.
func fmtDuration(secs float64) string {
	total := int(secs)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// cmdJoin connects the bot to the caller's current voice channel.
// If there is a paused queue waiting, the user is prompted to use !resume.
func cmdJoin(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	state, err := s.State.VoiceState(m.GuildID, m.Author.ID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ You must be in a voice channel to use this.")
		return
	}

	if err := sess.Join(s, m.GuildID, state.ChannelID); err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Failed to join the voice channel.")
		return
	}

	// Check whether there is a paused queue the user might want to resume.
	sess.Mu.Lock()
	queueLen := len(sess.Queue)
	sess.Mu.Unlock()

	if queueLen > 0 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🔊 Joined! There are **%d** tracks in the queue. Use `!resume` to continue.", queueLen))
	} else {
		s.ChannelMessageSend(m.ChannelID, "🔊 Joined.")
	}
}

// cmdLeave disconnects the bot and clears the queue entirely.
func cmdLeave(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.Disconnect()
	s.ChannelMessageSend(m.ChannelID, "👋 Left the voice channel. Queue cleared.")
}

func cmdPlay(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session, shuffle bool) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!play <URL or Search>`")
		return
	}

	query := strings.Join(args[1:], " ")

	// Native search mapping parameter block mapping dynamically
	if idx, err := strconv.Atoi(query); err == nil {
		sess.Mu.Lock()
		if len(sess.SearchMemory) > 0 && idx > 0 && idx <= len(sess.SearchMemory) {
			targetTrack := sess.SearchMemory[idx-1]
			sess.SearchMemory = nil // Clean memory explicitly cleanly
			sess.Mu.Unlock()
			
			state, err2 := s.State.VoiceState(m.GuildID, m.Author.ID)
			if err2 != nil && sess.VoiceClient == nil {
				s.ChannelMessageSend(m.ChannelID, "❌ Join a voice channel first directly natively.")
				return
			}
			if sess.VoiceClient == nil {
				sess.Join(s, m.GuildID, state.ChannelID)
			}
			
			sess.AddQueue(targetTrack)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Mapped exactly right directly over search block: **%s**.", targetTrack.Title))
			sess.PlayQueue(s)
			return
		}
		sess.Mu.Unlock()
	}

	state, err := s.State.VoiceState(m.GuildID, m.Author.ID)
	if err != nil && sess.VoiceClient == nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Join a voice channel first.")
		return
	}

	if sess.VoiceClient == nil {
		sess.Join(s, m.GuildID, state.ChannelID)
	}

	s.ChannelMessageSend(m.ChannelID, "⏳ Fetching track info...")

	var tracks []*youtube.Track
	if strings.Contains(query, "playlist") || strings.Contains(query, "list=") {
		ch := make(chan *youtube.Track, 250)
		doneChan := make(chan bool)

		go youtube.ExtractPlaylistAsync(query, shuffle, ch, doneChan)
		s.ChannelMessageSend(m.ChannelID, "⏳ Loading playlist...")

		isFirst := true
		queued := 0
		skipped := 0

		go func() {
			for {
				select {
				case t := <-ch:
					if trackExceedsLimit(t) {
						// Count silently — reporting every skipped track in a
						// large playlist would flood the channel.
						skipped++
						continue
					}
					sess.AddQueue(t)
					queued++
					if isFirst {
						isFirst = false
						s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("▶️ Starting with **%s** — loading the rest...", t.Title))
						sess.PlayQueue(s)
					}
				case <-doneChan:
					msg := fmt.Sprintf("✅ Playlist loaded — **%d** tracks queued.", queued)
					if skipped > 0 {
						limit := fmtDuration(config.MaxTrackDuration)
						msg += fmt.Sprintf(" (%d skipped — over %s limit)", skipped, limit)
					}
					s.ChannelMessageSend(m.ChannelID, msg)
					return
				}
			}
		}()
		return
	} else if strings.HasPrefix(query, "http") {
		var track *youtube.Track
		track, err = youtube.Extract(query)
		if track != nil {
			tracks = append(tracks, track)
		}
	} else {
		tracks, err = youtube.Search(query, 1)
	}

	if err != nil || len(tracks) == 0 {
		s.ChannelMessageSend(m.ChannelID, "❌ Could not find that track.")
		return
	}

	// Single-track path: report rejections immediately.
	var allowed []*youtube.Track
	for _, t := range tracks {
		if trackExceedsLimit(t) {
			s.ChannelMessageSend(m.ChannelID,
				fmt.Sprintf("⏭️ Skipped **%s** (%s) — exceeds the %s limit.",
					t.Title, fmtDuration(t.Duration), fmtDuration(config.MaxTrackDuration)))
			continue
		}
		allowed = append(allowed, t)
	}

	if len(allowed) == 0 {
		return
	}
	for _, t := range allowed {
		sess.AddQueue(t)
	}
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Added **%d** track(s) to the queue.", len(allowed)))
	sess.PlayQueue(s)
}

func cmdSearch(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!search <query>`")
		return
	}
	
	query := strings.Join(args[1:], " ")
	s.ChannelMessageSend(m.ChannelID, "⏳ Scraping exact dynamic parameter mappings structurally from YouTube organically...")
	
	tracks, err := youtube.Search(query, 20)
	if err != nil || len(tracks) == 0 {
		s.ChannelMessageSend(m.ChannelID, "❌ Native execution structurally completely failed explicitly.")
		return
	}
	
	sess.Mu.Lock()
	sess.SearchMemory = tracks
	sess.Mu.Unlock()
	
	msg := "**Native YouTube Search Output:** (Call `!p <number>` seamlessly to mount the track linearly natively!)\n"
	for i, t := range tracks {
		msg += fmt.Sprintf("`%2d.` %s\n", i+1, t.Title)
		if i >= 19 {
			break
		}
	}
	s.ChannelMessageSend(m.ChannelID, msg)
}

func cmdSkip(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	if sess.Skip() {
		s.ChannelMessageSend(m.ChannelID, "⏭️ Instructed execution stream dynamically to break frame playback.")
	} else {
		s.ChannelMessageSend(m.ChannelID, "❌ Memory block is not playing.")
	}
}

func cmdPrevious(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	if sess.Previous() {
		s.ChannelMessageSend(m.ChannelID, "⏪ Structurally reversed dynamically natively! Re-queueing prior payload sequence.")
	} else {
		s.ChannelMessageSend(m.ChannelID, "❌ No previous baseline execution tracks are stored natively in the persistent array.")
	}
}

func cmdStop(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.Stop()
	s.ChannelMessageSend(m.ChannelID, "⏹️ Executed hard stop closure.")
}

func cmdQueue(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	
	if len(sess.Queue) == 0 && sess.CurrentTrack == nil {
		s.ChannelMessageSend(m.ChannelID, "💭 Queue completely empty organically.")
		return
	}
	
	showAll := false
	if len(args) > 1 && strings.ToLower(args[1]) == "all" {
		showAll = true
	}
	
	var messages []string
	currentMsg := ""
	
	if sess.CurrentTrack != nil {
		currentMsg += fmt.Sprintf("🎵 **Now Playing:** %s\n\n", sess.CurrentTrack.Title)
	}
	
	currentMsg += fmt.Sprintf("📝 **Queue** (%d frames)\n", len(sess.Queue))
	
	for i, t := range sess.Queue {
		// Limit to first 15 items unless "all" is specified.
		if !showAll && i >= 15 {
			currentMsg += fmt.Sprintf("   ... and %d more items. (Use `!queue all` to explicitly display universally natively)\n", len(sess.Queue)-15)
			break
		}
		
		line := fmt.Sprintf("`%d.` %s\n", i+1, t.Title)
		// Discord structurally aggressively caps linearly at cleanly 2000 characters identically
		if len(currentMsg)+len(line) > 1900 {
			messages = append(messages, currentMsg)
			currentMsg = ""
		}
		currentMsg += line
	}
	
	if currentMsg != "" {
		messages = append(messages, currentMsg)
	}
	
	// Rapidly deploy chunked execution arrays securely
	for _, chunk := range messages {
		s.ChannelMessageSend(m.ChannelID, chunk)
	}
}

func cmdClear(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.ClearQueue()
	s.ChannelMessageSend(m.ChannelID, "🗑️ Nuked queue list.")
}

func cmdVersion(s *discordgo.Session, m *discordgo.MessageCreate) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		s.ChannelMessageSend(m.ChannelID, "❌ Build metrics physically absent.")
		return
	}
	
	embed := &discordgo.MessageEmbed{
		Title: "📦 Music Bot Architecture",
		Description: fmt.Sprintf("Compiled utilizing Golang %s", info.GoVersion),
		Color: 0x9B59B6,
	}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

func cmdSaveQueue(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!savequeue <name>`")
		return
	}
	
	name := strings.Join(args[1:], " ")
	
	sess.Mu.Lock()
	q := make([]*youtube.Track, 0, len(sess.Queue))
	for _, t := range sess.Queue {
		q = append(q, t)
	}
	if sess.CurrentTrack != nil {
		q = append([]*youtube.Track{sess.CurrentTrack}, q...)
	}
	sess.Mu.Unlock()
	
	if len(q) == 0 {
		s.ChannelMessageSend(m.ChannelID, "❌ Queue is organically empty.")
		return
	}
	
	player.SaveQueue(m.GuildID, name, q)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("💾 Active playback map persisted cleanly to `%s`.", name))
}

func cmdLoadQueue(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!loadqueue <name>`")
		return
	}
	
	name := strings.Join(args[1:], " ")
	q := player.LoadQueue(m.GuildID, name)
	
	if len(q) == 0 {
		s.ChannelMessageSend(m.ChannelID, "❌ Could not retrieve extraction schema locally.")
		return
	}

	// Intrinsically identically dynamically fundamentally scramble the arrays structurally!
	if len(q) > 1 {
		rand.Shuffle(len(q), func(i, j int) {
			q[i], q[j] = q[j], q[i]
		})
	}

	state, err := s.State.VoiceState(m.GuildID, m.Author.ID)
	if err != nil && sess.VoiceClient == nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Drop into a voice comm channel initially.")
		return
	}

	if sess.VoiceClient == nil {
		sess.Join(s, m.GuildID, state.ChannelID)
	}

	s.ChannelMessageSend(m.ChannelID, "⏳ Asynchronously proxying fresh CDN certificates organically...")
	
	go func() {
		queued := 0
		skipped := 0
		for _, t := range q {
			if trackExceedsLimit(t) {
				skipped++
				continue
			}
			sess.AddQueue(t)
			queued++
		}
		msg := fmt.Sprintf("✅ Loaded **%d** tracks from playlist.", queued)
		if skipped > 0 {
			msg += fmt.Sprintf(" (%d skipped — over %s limit)", skipped, fmtDuration(config.MaxTrackDuration))
		}
		s.ChannelMessageSend(m.ChannelID, msg)
		sess.PlayQueue(s)
	}()
}

func cmdSavedPlaylists(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	names := player.GetPlaylists(m.GuildID)
	if len(names) == 0 {
		s.ChannelMessageSend(m.ChannelID, "💭 No customized streams organically accessible physically.")
		return
	}
	
	msg := "📁 **Persisted Stream Arrays:**\n"
	for _, n := range names {
		msg += fmt.Sprintf("• `%s`\n", n)
	}
	s.ChannelMessageSend(m.ChannelID, msg)
}

func cmdDeletePlaylist(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!deleteplaylist <name>`")
		return
	}

	name := strings.Join(args[1:], " ")
	if player.DeletePlaylist(m.GuildID, name) {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🗑️ Deleted playlist: `%s`", name))
	} else {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Playlist not found: `%s`", name))
	}
}

func cmdHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
	embed := &discordgo.MessageEmbed{
		Title: "🎵 Music Bot Commands",
		Description: "A high-performance Golang audio architecture directly bridging Discord.",
		Color: 0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "📻 Playback", Value: "`!play <URL or Search>` - Extract audio (auto-shuffles playlists)\n`!search <Query>` - Locate TOP 20 native streams organically\n`!playing` (`!np`) - Display exactly actively mapped seamlessly physical active tracker arrays natively.\n`!skip` (`!next`) - Skip cleanly across current sequence\n`!previous` (`!prev`) - Rigidly cleanly reverse payload sequence\n`!stop` - Terminate explicitly\n`!pause` - Pause cleanly\n`!resume` - Unpause linearly", Inline: false},
			{Name: "📝 Queue & State", Value: "`!queue [all]` - Output active streams (shows first 15, or all with `all`)\n`!clear` - Wipe entire queue\n`!move <From> <To>` - Move specific natively mapped sequence intuitively\n`!volume <1-100>` - Alter Audio Loudness statically\n`!shuffle` - Randomize active arrays naturally\n`!savequeue <name>` - Persist purely physically\n`!loadqueue <name>` - Load organically natively\n`!deleteplaylist <name>` - Delete a saved playlist\n`!savedplaylists` - Check persistent logs", Inline: false},
			{Name: "⚙️ Core Setup", Value: "`!join` - Mount voice\n`!leave` - Unbind voice\n`!version` - Core execution metrics\n`!help` - Output this cleanly mapped array", Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "Golang Native Edition"},
	}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

func cmdVolume(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🔊 Current volume is physically mapped to **%d%%**", sess.Volume))
		return
	}
	
	v, err := strconv.Atoi(args[1])
	if err != nil || v < 1 || v > 500 {
		s.ChannelMessageSend(m.ChannelID, "❌ Volume must be between 1 and 500 (e.g. `!volume 50`).")
		return
	}
	
	sess.Mu.Lock()
	sess.Volume = v
	sess.Mu.Unlock()
	
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🔊 Volume permanently adjusted to **%d%%**. (Will actively reflect upon subsequent track slices).", v))
}

// cmdPause pauses the currently playing audio.
func cmdPause(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.SetPaused(true)
	s.ChannelMessageSend(m.ChannelID, "⏸ Paused.")
}

// cmdResume handles two cases:
//  1. Normal !resume after !pause — unpauses the DCA stream.
//  2. Queue resume after a voice disconnect — restarts PlayQueue with the
//     preserved queue. The bot must already be in a voice channel (!join first).
func cmdResume(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	// If the stream is paused, unpause it.
	sess.Mu.Lock()
	stream := sess.Stream
	isPlaying := sess.IsPlaying
	queueLen := len(sess.Queue)
	sess.Mu.Unlock()

	if stream != nil && isPlaying {
		sess.SetPaused(false)
		s.ChannelMessageSend(m.ChannelID, "▶ Resumed.")
		return
	}

	// Not currently playing — try to restart from the preserved queue.
	if queueLen == 0 {
		s.ChannelMessageSend(m.ChannelID, "❌ Nothing in the queue to resume.")
		return
	}
	if sess.VoiceClient == nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Not connected to a voice channel. Use `!join` first.")
		return
	}
	if ok := sess.Resume(s); !ok {
		s.ChannelMessageSend(m.ChannelID, "❌ Could not resume (queue empty or not in voice).")
		return
	}
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("▶️ Resuming — **%d** tracks in queue.", queueLen))
}

// cmdShuffle randomizes the underlying array payload physically across the native Queue matrix.
func cmdShuffle(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.ShuffleQueue()
	s.ChannelMessageSend(m.ChannelID, "🔀 Queue structure completely randomized naturally.")
}

func cmdMove(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 3 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!move <from> <to>` (e.g. `!move 5 2`)")
		return
	}
	
	from, err1 := strconv.Atoi(args[1])
	to, err2 := strconv.Atoi(args[2])
	if err1 != nil || err2 != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Indices must strictly map natively to integer structs.")
		return
	}
	
	from--
	to--
	
	if track, ok := sess.Move(from, to); ok {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Actively relocated natively: **%s** from `%d` to `%d`.", track.Title, from+1, to+1))
	} else {
		s.ChannelMessageSend(m.ChannelID, "❌ Physical index falls outside absolute queue matrix bounds.")
	}
}

func cmdRemove(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!remove <index>`")
		return
	}
	
	idx, err := strconv.Atoi(args[1])
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Index parameter must legitimately map smoothly safely precisely inherently perfectly logically naturally purely to integers.")
		return
	}
	
	idx--
	
	if track, ok := sess.Remove(idx); ok {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Completely effectively organically eradicated natively: **%s** from physical slot `%d`.", track.Title, idx+1))
	} else {
		s.ChannelMessageSend(m.ChannelID, "❌ Explicit parameter outside boundaries.")
	}
}

func cmdPlaying(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.Mu.Lock()
	track := sess.CurrentTrack
	sess.Mu.Unlock()
	
	if track == nil {
		s.ChannelMessageSend(m.ChannelID, "❌ No audio streams are physically implicitly dynamically actively natively securely successfully cleanly appropriately gracefully executing linearly.")
		return
	}
	
	embed := &discordgo.MessageEmbed{
		Title: "🎵 Actively Playing",
		Description: track.Display(),
		Color: 0x2ecc71,
		Thumbnail: &discordgo.MessageEmbedThumbnail{URL: track.Thumbnail},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Uploader", Value: track.Uploader, Inline: true},
			{Name: "Duration", Value: fmt.Sprintf("%.0f seconds", track.Duration), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "▶ YouTube • Golang Native Execution"},
	}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}
