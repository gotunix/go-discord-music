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
	"runtime/debug"
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
		cmdPlay(s, m, args, sess)
	case "skip", "s":
		cmdSkip(s, m, sess)
	case "stop":
		cmdStop(s, m, sess)
	case "queue", "q":
		cmdQueue(s, m, sess)
	case "clear":
		cmdClear(s, m, sess)
	case "savequeue":
		cmdSaveQueue(s, m, args, sess)
	case "loadqueue":
		cmdLoadQueue(s, m, args, sess)
	case "savedplaylists":
		cmdSavedPlaylists(s, m, sess)
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

func cmdJoin(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	state, err := s.State.VoiceState(m.GuildID, m.Author.ID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ You must be in a voice channel to use this.")
		return
	}
	
	err = sess.Join(s, m.GuildID, state.ChannelID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Failed to bridge into Voice Channel!")
		return
	}
	s.ChannelMessageSend(m.ChannelID, "🔊 Joined actively mapped!")
}

func cmdLeave(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.Leave()
	s.ChannelMessageSend(m.ChannelID, "👋 Unbound successfully.")
}

func cmdPlay(s *discordgo.Session, m *discordgo.MessageCreate, args []string, sess *player.Session) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "❌ Usage: `!play <URL or Search>`")
		return
	}

	state, err := s.State.VoiceState(m.GuildID, m.Author.ID)
	if err != nil && sess.VoiceClient == nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Join a voice channel first.")
		return
	}

	if sess.VoiceClient == nil {
		sess.Join(s, m.GuildID, state.ChannelID)
	}

	query := strings.Join(args[1:], " ")
	s.ChannelMessageSend(m.ChannelID, "⏳ Locating manifest streams dynamically...")

	var tracks []*youtube.Track
	if strings.Contains(query, "playlist") {
		tracks, err = youtube.ExtractPlaylist(query)
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
		s.ChannelMessageSend(m.ChannelID, "❌ Extraction execution failed structurally.")
		return
	}

	for _, t := range tracks {
		sess.AddQueue(t)
	}
	
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Mapped **%d** frames directly into queue block.", len(tracks)))
	sess.PlayQueue(s)
}

func cmdSkip(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	if sess.Skip() {
		s.ChannelMessageSend(m.ChannelID, "⏭️ Instructed execution stream dynamically to break frame playback.")
	} else {
		s.ChannelMessageSend(m.ChannelID, "❌ Memory block is not playing.")
	}
}

func cmdStop(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.Stop()
	s.ChannelMessageSend(m.ChannelID, "⏹️ Executed hard stop closure.")
}

func cmdQueue(s *discordgo.Session, m *discordgo.MessageCreate, sess *player.Session) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	
	if len(sess.Queue) == 0 && sess.CurrentTrack == nil {
		s.ChannelMessageSend(m.ChannelID, "💭 Queue completely empty organically.")
		return
	}
	
	msg := ""
	if sess.CurrentTrack != nil {
		msg += fmt.Sprintf("🎵 **Now Playing:** %s\n\n", sess.CurrentTrack.Title)
	}
	
	msg += fmt.Sprintf("📝 **Queue** (%d frames)\n", len(sess.Queue))
	for i, t := range sess.Queue {
		if i >= 15 {
			msg += fmt.Sprintf("   ... and %d more items\n", len(sess.Queue)-15)
			break
		}
		msg += fmt.Sprintf("`%2d.` %s\n", i+1, t.Title)
	}
	s.ChannelMessageSend(m.ChannelID, msg)
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
		for _, legacyTrack := range q {
			target := legacyTrack.Webpage
			if target == "" {
				target = legacyTrack.URL
			}
			fresh, _ := youtube.Extract(target)
			if fresh != nil {
				sess.AddQueue(fresh)
			} else {
				// Fallback dynamically
				sess.AddQueue(legacyTrack)
			}
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Intercepted **%d** streams directly bridging internal loops.", len(q)))
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

func cmdHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
	embed := &discordgo.MessageEmbed{
		Title: "🎵 Music Bot Commands",
		Description: "A high-performance Golang audio architecture directly bridging Discord.",
		Color: 0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "📻 Playback", Value: "`!play <URL or Search>` - Extract audio\n`!skip` - Skip current track\n`!stop` - Stop execution completely", Inline: false},
			{Name: "📝 Queue & State", Value: "`!queue` - Output active streams\n`!clear` - Wipe entire queue\n`!savequeue <name>` - Persist dynamically\n`!loadqueue <name>` - Mount natively\n`!savedplaylists` - Output stored tracks", Inline: false},
			{Name: "⚙️ Core Setup", Value: "`!join` - Mount voice\n`!leave` - Unbind voice\n`!version` - Core execution metrics\n`!help` - Output this array", Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "Golang Native Edition"},
	}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}
