package content

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
)

// MigrateSQLite copies the historical SQLite content into an empty PostgreSQL
// database. IDs, revisions and media filenames remain unchanged so the files on
// the PVC and all URLs continue to work.
func MigrateSQLite(ctx context.Context, sourceFile, databaseURL string) (int, int64, error) {
	u := url.URL{Scheme: "file", Path: sourceFile}
	q := u.Query()
	q.Set("mode", "ro")
	u.RawQuery = q.Encode()
	source, err := sql.Open("sqlite", u.String())
	if err != nil {
		return 0, 0, err
	}
	defer source.Close()
	if err = source.PingContext(ctx); err != nil {
		return 0, 0, err
	}
	target, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return 0, 0, err
	}
	defer target.Close()
	if err = target.PingContext(ctx); err != nil {
		return 0, 0, err
	}
	if err = migrate(target, "postgres"); err != nil {
		return 0, 0, err
	}
	var existing int
	if err = target.QueryRowContext(ctx, "SELECT count(*) FROM media").Scan(&existing); err != nil {
		return 0, 0, err
	}
	if existing != 0 {
		return 0, 0, errors.New("PostgreSQL-doeldatabase is niet leeg")
	}
	var revision int64
	if err = source.QueryRowContext(ctx, "SELECT revision FROM content_state WHERE id=1").Scan(&revision); err != nil {
		return 0, 0, fmt.Errorf("lees SQLite-revisie: %w", err)
	}
	rows, err := source.QueryContext(ctx, `SELECT id,section,kind,title,description,category,youtube_id,photo_file,created_by,created_at,video_file FROM media ORDER BY id`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	count := 0
	for rows.Next() {
		var id int64
		var section, kind, title, description, category, youtube, photo, author, createdAt, video string
		if err = rows.Scan(&id, &section, &kind, &title, &description, &category, &youtube, &photo, &author, &createdAt, &video); err != nil {
			return 0, 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO media(id,section,kind,title,description,category,youtube_id,photo_file,created_by,created_at,video_file,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, section, kind, title, description, category, youtube, photo, author, createdAt, video, -id); err != nil {
			return 0, 0, err
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE content_state SET revision=$1 WHERE id=1", revision); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, "SELECT setval(pg_get_serial_sequence('media','id'), COALESCE((SELECT max(id) FROM media),1), true)"); err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	var verified int
	if err = target.QueryRowContext(ctx, "SELECT count(*) FROM media").Scan(&verified); err != nil || verified != count {
		if err == nil {
			err = errors.New("aantal gemigreerde records wijkt af")
		}
		return 0, 0, err
	}
	return count, revision, nil
}
