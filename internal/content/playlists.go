package content

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

type Playlist struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

var spotifyPlaylistPath = regexp.MustCompile(`^/(?:intl-[a-z]{2}/)?playlist/([A-Za-z0-9]{22})/?$`)

func spotifyPlaylistID(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != "open.spotify.com" || u.User != nil {
		return "", errors.New("Gebruik een Spotify-playlistlink van https://open.spotify.com/playlist/…")
	}
	match := spotifyPlaylistPath.FindStringSubmatch(u.Path)
	if match == nil {
		return "", errors.New("Gebruik een Spotify-playlistlink, geen album of los nummer.")
	}
	return match[1], nil
}

func (s *Store) getPlaylists(w http.ResponseWriter, r *http.Request) {
	opts := &sql.TxOptions{ReadOnly: true}
	if s.dialect == "postgres" {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx, err := s.db.BeginTx(r.Context(), opts)
	if err != nil {
		failure(w, 503, "De playlists zijn tijdelijk niet beschikbaar.")
		return
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(r.Context(), "SELECT revision FROM content_state WHERE id=1").Scan(&revision); err != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	rows, err := tx.QueryContext(r.Context(), "SELECT spotify_id,title FROM playlists WHERE deleted_at='' ORDER BY created_at,spotify_id")
	if err != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	items := []Playlist{}
	for rows.Next() {
		var item Playlist
		if err = rows.Scan(&item.ID, &item.Title); err != nil {
			rows.Close()
			failure(w, 503, "Laden mislukt.")
			return
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	if err = tx.Commit(); err != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	send(w, 200, map[string]any{"items": items, "revision": revision})
}

func (s *Store) addPlaylist(w http.ResponseWriter, r *http.Request, _ auth.User) {
	var body struct{ URL, Title string }
	if !decodeSmall(w, r, &body) {
		return
	}
	id, err := spotifyPlaylistID(body.URL)
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if !utf8.ValidString(body.Title) || utf8.RuneCountInString(body.Title) > 100 {
		failure(w, 400, "Gebruik een titel van maximaal 100 tekens.")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	defer tx.Rollback()
	// Serialize writes before checking duplicates; the revision changes only on commit.
	if _, err = tx.ExecContext(r.Context(), "UPDATE content_state SET revision=revision+1 WHERE id=1"); err != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	var deleted string
	err = tx.QueryRowContext(r.Context(), s.bind("SELECT deleted_at FROM playlists WHERE spotify_id=?"), id).Scan(&deleted)
	switch {
	case err == nil && deleted == "":
		failure(w, 409, "Deze playlist staat al op de website.")
		return
	case err == nil:
		_, err = tx.ExecContext(r.Context(), s.bind("UPDATE playlists SET title=?,deleted_at='' WHERE spotify_id=?"), body.Title, id)
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.ExecContext(r.Context(), s.bind("INSERT INTO playlists(spotify_id,title,created_at) VALUES(?,?,?)"), id, body.Title, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if err != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	if err = tx.Commit(); err != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	send(w, 201, Playlist{ID: id, Title: body.Title})
}

func (s *Store) deletePlaylist(w http.ResponseWriter, r *http.Request, _ auth.User) {
	var body struct{ Revision int64 }
	if !decodeSmall(w, r, &body) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		failure(w, 503, "Verwijderen mislukt.")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), s.bind("UPDATE content_state SET revision=revision+1 WHERE id=1 AND revision=?"), body.Revision)
	if err != nil {
		failure(w, 503, "Verwijderen mislukt.")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		failure(w, 503, "Verwijderen mislukt.")
		return
	}
	if n == 0 {
		failure(w, 409, "De lijst is intussen gewijzigd. Probeer het opnieuw.")
		return
	}
	result, err = tx.ExecContext(r.Context(), s.bind("UPDATE playlists SET deleted_at=? WHERE spotify_id=? AND deleted_at=''"), time.Now().UTC().Format(time.RFC3339Nano), r.PathValue("id"))
	if err != nil {
		failure(w, 503, "Verwijderen mislukt.")
		return
	}
	n, err = result.RowsAffected()
	if err != nil {
		failure(w, 503, "Verwijderen mislukt.")
		return
	}
	if n == 0 {
		failure(w, 404, "Playlist niet gevonden.")
		return
	}
	if err = tx.Commit(); err != nil {
		failure(w, 503, "Verwijderen mislukt.")
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}

func migratePlaylists(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS playlists (
 spotify_id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, deleted_at TEXT NOT NULL DEFAULT ''
 );
 INSERT INTO playlists(spotify_id,title,created_at)
 VALUES('4zbpTuVArXmyTCdaBXOudH','Arbeids Vitaminen Top 2000 Hits jaren 60-70-80','2026-09-16T00:00:00Z')
 ON CONFLICT(spotify_id) DO NOTHING;`)
	return err
}
