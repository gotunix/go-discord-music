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

package youtube

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

type Track struct {
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	Webpage   string  `json:"webpage_url"`
	Duration  float64 `json:"duration"`
	Thumbnail string  `json:"thumbnail"`
	Uploader  string  `json:"uploader"`
}

func (t *Track) Display() string {
	return fmt.Sprintf("**%s** by *%s*", t.Title, t.Uploader)
}

func buildBaseArgs() []string {
	return []string{
		"-f", "bestaudio/best",
		"--no-playlist",
		"-J", 
	}
}

func Search(query string, limit int) ([]*Track, error) {
	log.Printf("Searching YT: %s", query)
	args := buildBaseArgs()
	args = append(args, fmt.Sprintf("ytsearch%d:%s", limit, query))

	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp query execution failed setup output: %w", err)
	}

	return parseJSONLines(out)
}

func Extract(url string) (*Track, error) {
	log.Printf("Extracting URL payload streams natively: %s", url)
	args := buildBaseArgs()
	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp extract failed setup output: %w", err)
	}

	tracks, err := parseJSONLines(out)
	if len(tracks) > 0 {
		return tracks[0], nil
	}
	return nil, err
}

func ExtractPlaylistAsync(url string, shuffle bool, ch chan<- *Track, doneChan chan<- bool) {
	log.Printf("Extracting underlying parsed Playlist natively asynchronously: %s", url)
	args := []string{
		"--dump-json",
		"--flat-playlist",
		"--no-warnings",
	}
	if shuffle {
		args = append(args, "--playlist-random")
	}
	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("yt-dlp async pipe creation failed natively: %v", err)
		doneChan <- true
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("yt-dlp async wrapper invocation completely severed: %v", err)
		doneChan <- true
		return
	}

	// Spin background daemon immediately explicitly to process string streams exactly dynamically!
	go func() {
		defer func() {
			cmd.Wait()
			doneChan <- true
		}()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			
			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				continue
			}
			
			t := extractSingleTrack(raw)
			if t != nil && t.URL != "" {
				ch <- t
			}
		}
	}()
}

func parseJSONLines(data []byte) ([]*Track, error) {
	lines := strings.Split(string(data), "\n")
	var tracks []*Track
	
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		if entries, ok := raw["entries"].([]interface{}); ok {
			for _, eRaw := range entries {
				if eMap, ok := eRaw.(map[string]interface{}); ok {
					var rawEmbedBytes []byte
					rawEmbedBytes, _ = json.Marshal(eMap)
					var eRawMap map[string]interface{}
					json.Unmarshal(rawEmbedBytes, &eRawMap)
					
					t := extractSingleTrack(eRawMap)
					if t != nil && t.URL != "" {
						tracks = append(tracks, t)
					}
				}
			}
			continue
		}
		
		t := extractSingleTrack(raw)
		if t != nil && t.URL != "" {
			tracks = append(tracks, t)
		}
	}
	return tracks, nil
}

func extractSingleTrack(raw map[string]interface{}) *Track {
	t := &Track{}
	if val, ok := raw["title"].(string); ok { t.Title = val }
	if val, ok := raw["url"].(string); ok { t.URL = val }
	if val, ok := raw["webpage_url"].(string); ok { t.Webpage = val }
	if val, ok := raw["duration"].(float64); ok { t.Duration = val }
	if val, ok := raw["thumbnail"].(string); ok { t.Thumbnail = val }
	if val, ok := raw["uploader"].(string); ok { t.Uploader = val }
	
	if t.URL == "" {
		if formats, ok := raw["formats"].([]interface{}); ok {
			for i := len(formats) - 1; i >= 0; i-- {
				if fMap, ok := formats[i].(map[string]interface{}); ok && fMap["url"] != nil {
					t.URL = fMap["url"].(string)
					break
				}
			}
		}
	}
	
	return t
}
