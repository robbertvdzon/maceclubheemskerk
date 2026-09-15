package content

import (
	"database/sql"
	"errors"
)

const mediaSchema = `CREATE TABLE media (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 section TEXT NOT NULL CHECK(section IN ('exercise','training')),
 kind TEXT NOT NULL CHECK(kind IN ('video','photo')),
 title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT '',
 youtube_id TEXT NOT NULL DEFAULT '', photo_file TEXT NOT NULL DEFAULT '', video_file TEXT NOT NULL DEFAULT '',
 created_by TEXT NOT NULL, created_at TEXT NOT NULL,
 CHECK((kind='video' AND length(youtube_id)=11 AND photo_file='' AND video_file='')
 OR (kind='video' AND youtube_id='' AND photo_file='' AND video_file<>'')
 OR (kind='photo' AND youtube_id='' AND photo_file<>'' AND video_file='')),
 CHECK(section<>'exercise' OR kind='video')
)`

// Version 2 adds uploaded videos without changing existing IDs, content or sessions.
func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 2 {
		return errors.New("database schema is newer than this application")
	}
	if version == 0 {
		if _, err = tx.Exec(mediaSchema); err != nil {
			return err
		}
	} else if version == 1 {
		if _, err = tx.Exec("ALTER TABLE media RENAME TO media_v1"); err != nil {
			return err
		}
		if _, err = tx.Exec(mediaSchema); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO media(id,section,kind,title,description,category,youtube_id,photo_file,created_by,created_at)
   SELECT id,section,kind,title,description,category,youtube_id,photo_file,created_by,created_at FROM media_v1;
   DROP TABLE media_v1;`); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS content_state (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL);
 INSERT OR IGNORE INTO content_state VALUES(1,0); PRAGMA user_version=2;`); err != nil {
		return err
	}
	return tx.Commit()
}
