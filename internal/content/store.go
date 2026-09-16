// Package content stores the public club library in SQLite and photos on the same volume.
package content

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Item struct {
	ID          int64  `json:"id"`
	Section     string `json:"section"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	YouTube     string `json:"youtube,omitempty"`
	Photo       string `json:"photo,omitempty"`
	CreatedAt   string `json:"createdAt"`
	Video       string `json:"video,omitempty"`
	videoFile   string
}
type Library struct {
	Items    []Item `json:"items"`
	Revision int64  `json:"revision"`
}
type Store struct {
	db            *sql.DB
	dir           string
	dialect       string
	photos        chan struct{}
	videos        chan struct{}
	videoDir      string
	uploadMu      sync.Mutex
	chunkUploads  map[string]*videoUpload
	uploadWorkers sync.WaitGroup
	convertVideo  func(context.Context, string, string) error
}

func Open(path string) (*Store, error) {
	return OpenWithVideoDir(path, filepath.Join(filepath.Dir(path), "videos"))
}

// OpenWithVideoDir opens PostgreSQL when DATABASE_URL is a postgres URL. SQLite is
// retained only for local tools and for reading the historical migration source.
func OpenWithVideoDir(databaseURL, videoDir string) (*Store, error) {
	dialect := "sqlite"
	dir := filepath.Dir(databaseURL)
	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		dialect = "postgres"
		dir = "/data"
	} else {
		absolute, err := filepath.Abs(databaseURL)
		if err != nil {
			return nil, err
		}
		databaseURL = absolute
		dir = filepath.Dir(absolute)
		if err = os.MkdirAll(filepath.Join(dir, "uploads"), 0700); err != nil {
			return nil, err
		}
	}

	var db *sql.DB
	var err error
	if dialect == "postgres" {
		db, err = sql.Open("pgx", databaseURL)
	} else {
		u := url.URL{Scheme: "file", Path: databaseURL}
		q := u.Query()
		for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)", "foreign_keys(ON)", "temp_store(MEMORY)"} {
			q.Add("_pragma", p)
		}
		u.RawQuery = q.Encode()
		db, err = sql.Open("sqlite", u.String())
	}
	if err != nil {
		return nil, err
	}
	if dialect == "sqlite" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, dir: dir, dialect: dialect, photos: make(chan struct{}, 1), videos: make(chan struct{}, 1), videoDir: videoDir, convertVideo: transcodeVideo}
	if err = migrate(db, dialect); err != nil {
		db.Close()
		return nil, err
	}
	if dialect == "sqlite" {
		if err = os.Chmod(databaseURL, 0600); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err = os.MkdirAll(s.videoDir, 0700); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) bind(query string) string {
	if s.dialect != "postgres" {
		return query
	}
	var b strings.Builder
	n := 1
	for _, ch := range query {
		if ch == '?' {
			fmt.Fprintf(&b, "$%d", n)
			n++
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func (s *Store) Close() error {
	s.uploadMu.Lock()
	for _, upload := range s.chunkUploads {
		if upload.processing {
			upload.cancel()
		} else {
			s.removeUpload(upload)
		}
	}
	s.uploadMu.Unlock()
	s.uploadWorkers.Wait()
	s.uploadMu.Lock()
	for _, upload := range s.chunkUploads {
		s.removeUpload(upload)
	}
	s.uploadMu.Unlock()
	return s.db.Close()
}
func (s *Store) Revision(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT revision FROM content_state WHERE id=1").Scan(&n)
	return n, err
}
func (s *Store) List(ctx context.Context) (Library, error) {
	out := Library{Items: []Item{}}
	opts := &sql.TxOptions{ReadOnly: true}
	if s.dialect == "postgres" {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM content_state WHERE id=1").Scan(&out.Revision); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,section,kind,title,description,category,youtube_id,photo_file,created_at,video_file FROM media WHERE deleted_at='' ORDER BY sort_order,id DESC")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var i Item
		var file string
		if err = rows.Scan(&i.ID, &i.Section, &i.Type, &i.Title, &i.Description, &i.Category, &i.YouTube, &file, &i.CreatedAt, &i.videoFile); err != nil {
			rows.Close()
			return out, err
		}
		if file != "" {
			i.Photo = "/media/" + file
		}
		if i.videoFile != "" {
			i.Video = "/videos/" + i.videoFile
		}
		out.Items = append(out.Items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

var youtubePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func YouTubeID(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Port() != "" {
		return "", errors.New("Gebruik een volledige YouTube-videolink.")
	}
	var id string
	switch strings.ToLower(u.Hostname()) {
	case "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	case "youtube.com", "www.youtube.com", "m.youtube.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live") {
				id = parts[1]
			}
		}
	}
	if !youtubePattern.MatchString(id) {
		return "", errors.New("Deze link herkennen we niet. Plak een YouTube-videolink.")
	}
	return id, nil
}
func Validate(i *Item) error {
	i.Title = strings.TrimSpace(i.Title)
	i.Description = strings.TrimSpace(i.Description)
	if !utf8.ValidString(i.Title) || !utf8.ValidString(i.Description) || utf8.RuneCountInString(i.Title) > 100 || utf8.RuneCountInString(i.Description) > 500 {
		return errors.New("Gebruik maximaal 100 tekens voor de titel en 500 voor de beschrijving.")
	}
	if i.Section != "exercise" && i.Section != "training" {
		return errors.New("Kies oefeningen of trainingen.")
	}
	if i.Type != "video" && i.Type != "photo" {
		return errors.New("Kies een video of foto.")
	}
	if i.Section == "exercise" && i.Type != "video" {
		return errors.New("Voeg een oefening toe als YouTube-video of videobestand.")
	}
	i.Category = ""
	if i.Type == "video" && !youtubePattern.MatchString(i.YouTube) && !videoPattern.MatchString(i.videoFile) {
		return errors.New("Ongeldige video.")
	}
	return nil
}
func (s *Store) Create(ctx context.Context, i Item, author, file string) (Item, error) {
	if err := Validate(&i); err != nil {
		return i, err
	}
	if i.Type == "photo" && !photoPattern.MatchString(file) {
		return i, errors.New("invalid photo filename")
	}
	if author == "" {
		return i, errors.New("missing author")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return i, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE content_state SET revision=revision+1 WHERE id=1"); err != nil {
		return i, err
	}
	var position int64
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MIN(sort_order),0)-1 FROM media").Scan(&position); err != nil {
		return i, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM media").Scan(&count); err != nil {
		return i, err
	}
	if count >= 10000 {
		return i, errors.New("De bibliotheek is vol.")
	}
	i.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.dialect == "postgres" {
		err = tx.QueryRowContext(ctx, s.bind(`INSERT INTO media(section,kind,title,description,category,youtube_id,photo_file,created_by,created_at,video_file,sort_order) VALUES(?,?,?,?,?,?,?,?,?,?,?) RETURNING id`), i.Section, i.Type, i.Title, i.Description, i.Category, i.YouTube, file, author, i.CreatedAt, i.videoFile, position).Scan(&i.ID)
	} else {
		res, execErr := tx.ExecContext(ctx, `INSERT INTO media(section,kind,title,description,category,youtube_id,photo_file,created_by,created_at,video_file,sort_order) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, i.Section, i.Type, i.Title, i.Description, i.Category, i.YouTube, file, author, i.CreatedAt, i.videoFile, position)
		if execErr == nil {
			i.ID, execErr = res.LastInsertId()
		}
		err = execErr
	}
	if err != nil {
		return i, err
	}

	if err = tx.Commit(); err != nil {
		return i, err
	}
	if file != "" {
		i.Photo = "/media/" + file
	}
	if i.videoFile != "" {
		i.Video = "/videos/" + i.videoFile
	}
	return i, nil
}
func randomName() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x.jpg", b)
}
