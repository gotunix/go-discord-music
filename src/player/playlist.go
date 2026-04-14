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
	"encoding/json"
	"os"
	"sync"
	
	"go-discord-music/youtube"
)

var fileMu sync.Mutex

func getPlaylistsPath() string {
	return "playlists.json"
}

func LoadPlaylists() map[string]map[string][]*youtube.Track {
	fileMu.Lock()
	defer fileMu.Unlock()
	
	data, err := os.ReadFile(getPlaylistsPath())
	if err != nil {
		return make(map[string]map[string][]*youtube.Track)
	}
	
	var pl map[string]map[string][]*youtube.Track
	if err := json.Unmarshal(data, &pl); err != nil {
		return make(map[string]map[string][]*youtube.Track)
	}
	return pl
}

func SavePlaylists(pl map[string]map[string][]*youtube.Track) {
	fileMu.Lock()
	defer fileMu.Unlock()
	
	data, err := json.MarshalIndent(pl, "", "  ")
	if err == nil {
		os.WriteFile(getPlaylistsPath(), data, 0644)
	}
}

func SaveQueue(guildID, name string, q []*youtube.Track) {
	pl := LoadPlaylists()
	if pl[guildID] == nil {
		pl[guildID] = make(map[string][]*youtube.Track)
	}
	pl[guildID][name] = q
	SavePlaylists(pl)
}

func LoadQueue(guildID, name string) []*youtube.Track {
	pl := LoadPlaylists()
	return pl[guildID][name]
}

func GetPlaylists(guildID string) []string {
	pl := LoadPlaylists()
	var names []string
	for k := range pl[guildID] {
		names = append(names, k)
	}
	return names
}
