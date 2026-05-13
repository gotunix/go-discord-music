# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Fixed
- Refined playlist detection logic in `!play` command to be more robust. It now correctly identifies single video URLs even when they contain a playlist context (e.g., `youtu.be` links or `watch?v=...&list=...` links), ensuring only the specific song is loaded.

### Added
- Separated `!play` (single track) and `!playlist` (full playlist) commands. `!play` now strictly queues a single track even if a playlist URL is provided.
