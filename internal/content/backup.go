package content

import (
	"archive/zip"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
)

// Backup produces a consistent SQLite snapshot and the immutable photos it references.
// New additions during the backup are included only if present in the SQLite snapshot.
func (s *Store) Backup(ctx context.Context, destination string) error {
	dir, err := os.MkdirTemp(s.dir, ".backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	snapshot := filepath.Join(dir, "maceclub.sqlite")
	if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", snapshot); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", snapshot)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT photo_file,video_file FROM media WHERE photo_file<>'' OR video_file<>''")
	if err != nil {
		return err
	}
	files := [][2]string{}
	for rows.Next() {
		var photo, video string
		if err = rows.Scan(&photo, &video); err != nil {
			rows.Close()
			return err
		}
		if photo != "" {
			files = append(files, [2]string{"uploads/" + photo, filepath.Join(s.dir, "uploads", photo)})
		}
		if video != "" {
			files = append(files, [2]string{"videos/" + video, filepath.Join(s.videoDir, video)})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Create the archive beside its destination for atomic rename, without overwriting backups.
	if _, err = os.Stat(destination); err == nil {
		return os.ErrExist
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".mace-backup-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	z := zip.NewWriter(temp)
	add := func(name, path string) error {
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		defer f.Close()
		w, e := z.Create(name)
		if e != nil {
			return e
		}
		_, e = io.Copy(w, f)
		return e
	}
	if err = add("maceclub.sqlite", snapshot); err != nil {
		z.Close()
		return err
	}
	for _, file := range files {
		if err = add(file[0], file[1]); err != nil {
			z.Close()
			return err
		}
	}
	if err = z.Close(); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), destination)
}
