package content

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

func decodeSmall(w http.ResponseWriter, r *http.Request, body any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(body); err != nil {
		failure(w, 400, "Ongeldig verzoek.")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		failure(w, 400, "Ongeldig verzoek.")
		return false
	}
	return true
}
func (s *Store) registerManagement(mux *http.ServeMux, a *auth.Auth) {
	member := func(next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
		return a.RequireUser(func(w http.ResponseWriter, r *http.Request, u auth.User) {
			if !a.CanEdit(u) {
				failure(w, 403, "Alleen toegestane clubaccounts mogen dit aanpassen.")
				return
			}
			next(w, r, u)
		})
	}
	for _, route := range []string{"PATCH /api/media/{id}", "DELETE /api/media/{id}", "POST /api/media/{id}/restore", "POST /api/media/{id}/move"} {
		mux.HandleFunc(route, member(s.manageMedia))
	}
	mux.HandleFunc("GET /api/media/trash", member(func(w http.ResponseWriter, r *http.Request, u auth.User) {
		rows, err := s.db.QueryContext(r.Context(), "SELECT id,title,kind,section FROM media WHERE deleted_at<>'' ORDER BY deleted_at DESC")
		if err != nil {
			failure(w, 503, "De prullenbak is tijdelijk niet beschikbaar.")
			return
		}
		defer rows.Close()
		items := []Item{}
		for rows.Next() {
			var i Item
			if err = rows.Scan(&i.ID, &i.Title, &i.Type, &i.Section); err != nil {
				failure(w, 503, "Laden mislukt.")
				return
			}
			items = append(items, i)
		}
		if rows.Err() != nil {
			failure(w, 503, "Laden mislukt.")
			return
		}
		send(w, 200, map[string]any{"items": items})
	}))
	mux.HandleFunc("GET /api/playlists", s.getPlaylists)
	mux.HandleFunc("POST /api/playlists", member(s.addPlaylist))
	mux.HandleFunc("DELETE /api/playlists/{id}", member(s.deletePlaylist))
	mux.HandleFunc("GET /api/bingo", s.getBingo)
	mux.HandleFunc("PATCH /api/bingo/{id}", member(s.updateBingo))
}
func (s *Store) manageMedia(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		failure(w, 404, "Niet gevonden.")
		return
	}
	var body struct {
		Title, Description, Direction, Section string
		Revision                               int64
	}
	if !decodeSmall(w, r, &body) {
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	body.Description = strings.TrimSpace(body.Description)
	if !utf8.ValidString(body.Title) || !utf8.ValidString(body.Description) || utf8.RuneCountInString(body.Title) > 100 || utf8.RuneCountInString(body.Description) > 500 {
		failure(w, 400, "Gebruik maximaal 100 tekens voor de titel en 500 voor de beschrijving.")
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	defer tx.Rollback()
	// Atomically compare the public revision and serialize library changes.
	result, e := tx.ExecContext(r.Context(), s.bind("UPDATE content_state SET revision=revision+1 WHERE id=1 AND revision=?"), body.Revision)
	if e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		failure(w, 409, "De inhoud is intussen gewijzigd. Vernieuw het overzicht en probeer opnieuw.")
		return
	}
	var position int64
	var deleted, section, kind string
	if e = tx.QueryRowContext(r.Context(), s.bind("SELECT sort_order,deleted_at,section,kind FROM media WHERE id=?"), id).Scan(&position, &deleted, &section, &kind); e != nil {
		failure(w, 404, "Dit item bestaat niet meer.")
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/restore"):
		_, e = tx.ExecContext(r.Context(), s.bind("UPDATE media SET deleted_at='' WHERE id=?"), id)
	case deleted != "":
		failure(w, 404, "Dit item staat in de prullenbak.")
		return
	case r.Method == "PATCH":
		if body.Section != "" {
			section = body.Section
		}
		if (section != "exercise" && section != "training") || (section == "exercise" && kind != "video") {
			failure(w, 400, "Kies een geldige pagina; oefeningen zijn alleen video's.")
			return
		}
		_, e = tx.ExecContext(r.Context(), s.bind("UPDATE media SET title=?,description=?,section=? WHERE id=?"), body.Title, body.Description, section, id)
	case r.Method == "DELETE":
		_, e = tx.ExecContext(r.Context(), s.bind("UPDATE media SET deleted_at=? WHERE id=?"), time.Now().UTC().Format(time.RFC3339Nano), id)
	case strings.HasSuffix(r.URL.Path, "/move"):
		query := "SELECT id,sort_order FROM media WHERE deleted_at='' AND section=? AND sort_order<? ORDER BY sort_order DESC LIMIT 1"
		if body.Direction == "down" {
			query = "SELECT id,sort_order FROM media WHERE deleted_at='' AND section=? AND sort_order>? ORDER BY sort_order ASC LIMIT 1"
		} else if body.Direction != "up" {
			failure(w, 400, "Kies omhoog of omlaag.")
			return
		}
		var neighbor, otherPosition int64
		e = tx.QueryRowContext(r.Context(), s.bind(query), section, position).Scan(&neighbor, &otherPosition)
		if errors.Is(e, sql.ErrNoRows) {
			send(w, 200, map[string]bool{"ok": true})
			return
		}
		if e == nil {
			_, e = tx.ExecContext(r.Context(), s.bind("UPDATE media SET sort_order=? WHERE id=?"), otherPosition, id)
			if e == nil {
				_, e = tx.ExecContext(r.Context(), s.bind("UPDATE media SET sort_order=? WHERE id=?"), position, neighbor)
			}
		}
	}
	if e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	if e = tx.Commit(); e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}

type BingoCell struct {
	ID      int    `json:"id"`
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
	Version int    `json:"version"`
}

func (s *Store) getBingo(w http.ResponseWriter, r *http.Request) {
	opts := &sql.TxOptions{ReadOnly: true}
	if s.dialect == "postgres" {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx, e := s.db.BeginTx(r.Context(), opts)
	if e != nil {
		failure(w, 503, "De bingo is tijdelijk niet beschikbaar.")
		return
	}
	defer tx.Rollback()
	var revision int64
	if e = tx.QueryRowContext(r.Context(), "SELECT revision FROM content_state WHERE id=1").Scan(&revision); e != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	rows, e := tx.QueryContext(r.Context(), "SELECT id,text,checked,version FROM bingo_cells ORDER BY id")
	if e != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	cells := []BingoCell{}
	for rows.Next() {
		var cell BingoCell
		var checked int
		if e = rows.Scan(&cell.ID, &cell.Text, &checked, &cell.Version); e != nil {
			rows.Close()
			failure(w, 503, "Laden mislukt.")
			return
		}
		cell.Checked = checked == 1
		cells = append(cells, cell)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	if e = tx.Commit(); e != nil {
		failure(w, 503, "Laden mislukt.")
		return
	}
	send(w, 200, map[string]any{"cells": cells, "revision": revision})
}
func (s *Store) updateBingo(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, e := strconv.Atoi(r.PathValue("id"))
	if e != nil || id < 1 || id > 25 {
		failure(w, 404, "Vakje niet gevonden.")
		return
	}
	var body struct {
		Text    string
		Checked bool
		Version int
	}
	if !decodeSmall(w, r, &body) {
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" || !utf8.ValidString(body.Text) || utf8.RuneCountInString(body.Text) > 80 {
		failure(w, 400, "Vul een reden van 1 tot 80 tekens in.")
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(r.Context(), "UPDATE content_state SET revision=revision+1 WHERE id=1"); e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	checked := 0
	if body.Checked {
		checked = 1
	}
	result, e := tx.ExecContext(r.Context(), s.bind("UPDATE bingo_cells SET text=?,checked=?,version=version+1 WHERE id=? AND version=?"), body.Text, checked, id, body.Version)
	if e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		failure(w, 409, "Dit vakje is intussen aangepast. Sluit het venster en probeer opnieuw.")
		return
	}
	if e = tx.Commit(); e != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	send(w, 200, BingoCell{ID: id, Text: body.Text, Checked: body.Checked, Version: body.Version + 1})
}
