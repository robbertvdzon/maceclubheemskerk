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
	rows, err := db.QueryContext(ctx, "SELECT photo_file FROM media WHERE photo_file<>''")
	if err != nil {
		return err
	}
	files := []string{}
	for rows.Next() {
		var f string
		if err = rows.Scan(&f); err != nil {
			rows.Close()
			return err
		}
		files = append(files, f)
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
		if err = add("uploads/"+file, filepath.Join(s.dir, "uploads", file)); err != nil {
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
