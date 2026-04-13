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
	case "join":
		cmdJoin(s, m, sess)
	case "leave":
		cmdLeave(s, m, sess)
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
	sess.mu.Lock()
	defer sess.mu.Unlock()
	
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
