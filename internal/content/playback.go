package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

type Chapter struct {
	Time  float64 `json:"time"`
	Title string  `json:"title"`
}
type Playback struct {
	Start         float64   `json:"start"`
	End           float64   `json:"end"`
	ThumbnailTime float64   `json:"thumbnailTime"`
	Chapters      []Chapter `json:"chapters"`
}
type clipRequest struct {
	Start, End, ThumbnailTime float64
	Revision                  int64
}

func migratePlayback(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS media_playback (
 media_id BIGINT PRIMARY KEY REFERENCES media(id), start_time DOUBLE PRECISION NOT NULL DEFAULT 0,
 end_time DOUBLE PRECISION NOT NULL DEFAULT 0, thumbnail_time DOUBLE PRECISION NOT NULL DEFAULT 0,
 render_file TEXT NOT NULL DEFAULT '', thumbnail_file TEXT NOT NULL DEFAULT '', chapters TEXT NOT NULL DEFAULT '[]');
 CREATE TABLE IF NOT EXISTS media_clip_jobs (media_id BIGINT PRIMARY KEY REFERENCES media(id),
 job_id TEXT NOT NULL, status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '',
 render_file TEXT NOT NULL, thumbnail_file TEXT NOT NULL);`)
	return err
}
func mediaID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id < 1 {
		return 0, errors.New("invalid ID")
	}
	return id, err
}
func finiteTime(t float64) bool { return !math.IsNaN(t) && !math.IsInf(t, 0) && t >= 0 && t <= 86400 }
func validatePlayback(p *Playback) error {
	if !finiteTime(p.Start) || !finiteTime(p.End) || (p.End != 0 && p.End <= p.Start) || len(p.Chapters) > 40 {
		return errors.New("Kies geldige begin- en eindtijden (maximaal 24 uur) en maximaal 40 tijdstippen.")
	}
	previous := -1.0
	for i := range p.Chapters {
		c := &p.Chapters[i]
		c.Title = strings.TrimSpace(c.Title)
		if !finiteTime(c.Time) || c.Time < p.Start || (p.End > 0 && c.Time >= p.End) || c.Time <= previous || c.Title == "" || !utf8.ValidString(c.Title) || utf8.RuneCountInString(c.Title) > 80 {
			return errors.New("Geef elke oefening een naam en een oplopend, uniek tijdstip binnen het fragment.")
		}
		previous = c.Time
	}
	return nil
}
func (s *Store) editing(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, err := mediaID(r)
	if err != nil {
		failure(w, 404, "Niet gevonden.")
		return
	}
	var file string
	if err = s.db.QueryRowContext(r.Context(), s.bind("SELECT video_file FROM media WHERE id=? AND deleted_at='' AND video_file<>''"), id).Scan(&file); err != nil {
		failure(w, 404, "Video niet gevonden.")
		return
	}
	info, err := probeVideo(r.Context(), filepath.Join(s.videoDir, file))
	if err != nil {
		failure(w, 503, err.Error())
		return
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || !finiteTime(duration) || duration <= 0 {
		failure(w, 503, "De videoduur kon niet worden gelezen.")
		return
	}
	var p Playback
	err = s.db.QueryRowContext(r.Context(), s.bind("SELECT start_time,end_time,thumbnail_time FROM media_playback WHERE media_id=?"), id).Scan(&p.Start, &p.End, &p.ThumbnailTime)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		failure(w, 503, "Laden mislukt.")
		return
	}
	send(w, 200, map[string]any{"duration": duration, "playback": p, "source": "/api/media/" + strconv.FormatInt(id, 10) + "/source"})
}
func (s *Store) sourceVideo(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, err := mediaID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var file string
	if s.db.QueryRowContext(r.Context(), s.bind("SELECT video_file FROM media WHERE id=? AND deleted_at='' AND video_file<>''"), id).Scan(&file) != nil {
		http.NotFound(w, r)
		return
	}
	s.serveVideoFile(w, r, file, "video/mp4")
}
func (s *Store) serveVideoFile(w http.ResponseWriter, r *http.Request, name, mime string) {
	f, err := os.Open(filepath.Join(s.videoDir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, name, time.Time{}, f)
}
func (s *Store) thumbnail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !photoPattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	var exists int
	if s.db.QueryRowContext(r.Context(), s.bind("SELECT 1 FROM media_playback p JOIN media m ON m.id=p.media_id WHERE p.thumbnail_file=? AND m.deleted_at=''"), name).Scan(&exists) != nil {
		http.NotFound(w, r)
		return
	}
	s.serveVideoFile(w, r, name, "image/jpeg")
}
func (s *Store) saveYouTube(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, err := mediaID(r)
	if err != nil {
		failure(w, 404, "Niet gevonden.")
		return
	}
	var body struct {
		Playback
		Revision int64
	}
	if !decodeSmall(w, r, &body) {
		return
	}
	if err = validatePlayback(&body.Playback); err != nil {
		failure(w, 400, err.Error())
		return
	}
	chapters, _ := json.Marshal(body.Chapters)
	if body.Chapters == nil {
		chapters = []byte("[]")
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), s.bind("UPDATE content_state SET revision=revision+1 WHERE id=1 AND revision=?"), body.Revision)
	if err != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		failure(w, 409, "De inhoud is gewijzigd. Open de bewerking opnieuw.")
		return
	}
	var exists int
	if tx.QueryRowContext(r.Context(), s.bind("SELECT 1 FROM media WHERE id=? AND youtube_id<>'' AND deleted_at=''"), id).Scan(&exists) != nil {
		failure(w, 404, "YouTube-video niet gevonden.")
		return
	}
	_, err = tx.ExecContext(r.Context(), s.bind("INSERT INTO media_playback(media_id,start_time,end_time,chapters) VALUES(?,?,?,?) ON CONFLICT(media_id) DO UPDATE SET start_time=excluded.start_time,end_time=excluded.end_time,chapters=excluded.chapters"), id, body.Start, body.End, string(chapters))
	if err != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	if tx.Commit() != nil {
		failure(w, 503, "Opslaan mislukt.")
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}
func (s *Store) clipStatus(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, err := mediaID(r)
	if err != nil {
		failure(w, 404, "Niet gevonden.")
		return
	}
	var job, status, message string
	err = s.db.QueryRowContext(r.Context(), s.bind("SELECT job_id,status,error FROM media_clip_jobs WHERE media_id=?"), id).Scan(&job, &status, &message)
	if errors.Is(err, sql.ErrNoRows) {
		send(w, 200, map[string]string{"status": "idle"})
		return
	}
	if err != nil {
		failure(w, 503, "Status niet beschikbaar.")
		return
	}
	send(w, 200, map[string]string{"id": job, "status": status, "error": message})
}
func (s *Store) startClip(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, err := mediaID(r)
	if err != nil {
		failure(w, 404, "Niet gevonden.")
		return
	}
	var req clipRequest
	if !decodeSmall(w, r, &req) {
		return
	}
	if !finiteTime(req.Start) || !finiteTime(req.End) || !finiteTime(req.ThumbnailTime) || req.End-req.Start < 0.1 || req.ThumbnailTime < req.Start || req.ThumbnailTime >= req.End {
		failure(w, 400, "Kies een geldig fragment en een thumbnail binnen dat fragment.")
		return
	}
	select {
	case s.videos <- struct{}{}:
	default:
		failure(w, 429, "Er wordt al een video verwerkt of geüpload. Probeer het straks opnieuw.")
		return
	}
	started := false
	defer func() {
		if !started {
			<-s.videos
		}
	}()
	var source string
	if s.db.QueryRowContext(r.Context(), s.bind("SELECT video_file FROM media WHERE id=? AND deleted_at='' AND video_file<>''"), id).Scan(&source) != nil {
		failure(w, 404, "Video niet gevonden.")
		return
	}
	info, err := probeVideo(r.Context(), filepath.Join(s.videoDir, source))
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || !finiteTime(duration) || req.End > duration+0.001 {
		failure(w, 400, "Het fragment moet binnen de volledige video liggen.")
		return
	}
	if err = s.videoSpaceFor(maxChunkedVideoBytes + (5 << 20)); err != nil {
		failure(w, 507, err.Error())
		return
	}
	job := strings.TrimSuffix(randomName(), ".jpg")
	render := job + ".mp4"
	thumb := job + ".jpg"
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		failure(w, 503, "Starten mislukt.")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), s.bind("UPDATE content_state SET revision=revision WHERE id=1 AND revision=?"), req.Revision)
	if err != nil {
		failure(w, 503, "Starten mislukt.")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		failure(w, 409, "De inhoud is gewijzigd. Open de bewerking opnieuw.")
		return
	}
	// Recheck after serializing against concurrent deletion.
	var exists int
	if tx.QueryRowContext(r.Context(), s.bind("SELECT 1 FROM media WHERE id=? AND deleted_at=''"), id).Scan(&exists) != nil {
		failure(w, 404, "Video niet gevonden.")
		return
	}
	_, err = tx.ExecContext(r.Context(), s.bind("INSERT INTO media_clip_jobs(media_id,job_id,status,error,render_file,thumbnail_file) VALUES(?,?,'processing','',?,?) ON CONFLICT(media_id) DO UPDATE SET job_id=excluded.job_id,status='processing',error='',render_file=excluded.render_file,thumbnail_file=excluded.thumbnail_file"), id, job, render, thumb)
	if err != nil {
		failure(w, 503, "Starten mislukt.")
		return
	}
	if tx.Commit() != nil {
		failure(w, 503, "Starten mislukt.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.editMu.Lock()
	s.editCancel = cancel
	s.editMu.Unlock()
	s.uploadWorkers.Add(1)
	started = true
	go func() {
		defer s.uploadWorkers.Done()
		defer cancel()
		defer func() { s.editMu.Lock(); s.editCancel = nil; s.editMu.Unlock(); <-s.videos }()
		s.processClip(ctx, id, source, render, thumb, req)
	}()
	send(w, 202, map[string]string{"id": job, "status": "processing"})
}
func (s *Store) processClip(ctx context.Context, id int64, source, render, thumb string, req clipRequest) {
	err := renderClip(ctx, filepath.Join(s.videoDir, source), filepath.Join(s.videoDir, render), filepath.Join(s.videoDir, thumb), req)
	if err == nil {
		err = syncDirectory(s.videoDir)
	}
	unlock, lockErr := s.lockVideoAssets()
	if lockErr != nil {
		err = lockErr
	} else {
		defer unlock()
	}
	if err == nil {
		err = s.publishClip(ctx, id, render, thumb, req)
	}
	if err != nil {
		os.Remove(filepath.Join(s.videoDir, render))
		os.Remove(filepath.Join(s.videoDir, thumb))
		_, _ = s.db.Exec(s.bind("UPDATE media_clip_jobs SET status='error',error=? WHERE media_id=?"), err.Error(), id)
	}
}
func (s *Store) publishClip(ctx context.Context, id int64, render, thumb string, req clipRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE content_state SET revision=revision+1 WHERE id=1"); err != nil {
		return err
	}
	var exists int
	if tx.QueryRowContext(ctx, s.bind("SELECT 1 FROM media WHERE id=? AND deleted_at=''"), id).Scan(&exists) != nil {
		return errors.New("De video is intussen verwijderd. De vorige versie is bewaard.")
	}
	var oldRender, oldThumb string
	err = tx.QueryRowContext(ctx, s.bind("SELECT render_file,thumbnail_file FROM media_playback WHERE media_id=?"), id).Scan(&oldRender, &oldThumb)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, s.bind("INSERT INTO media_playback(media_id,start_time,end_time,thumbnail_time,render_file,thumbnail_file) VALUES(?,?,?,?,?,?) ON CONFLICT(media_id) DO UPDATE SET start_time=excluded.start_time,end_time=excluded.end_time,thumbnail_time=excluded.thumbnail_time,render_file=excluded.render_file,thumbnail_file=excluded.thumbnail_file"), id, req.Start, req.End, req.ThumbnailTime, render, thumb)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.bind("UPDATE media_clip_jobs SET status='done',error='' WHERE media_id=?"), id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for _, name := range []string{oldRender, oldThumb} {
		if name != "" {
			_ = os.Remove(filepath.Join(s.videoDir, name))
		}
	}
	return nil
}

// An interrupted edit never replaces the published version or the full source.
func (s *Store) recoverClips() error {
	rows, err := s.db.Query("SELECT render_file,thumbnail_file FROM media_clip_jobs WHERE status='processing'")
	if err != nil {
		return err
	}
	for rows.Next() {
		var video, thumb string
		if err = rows.Scan(&video, &thumb); err != nil {
			rows.Close()
			return err
		}
		if videoPattern.MatchString(video) {
			os.Remove(filepath.Join(s.videoDir, video))
		}
		if photoPattern.MatchString(thumb) {
			os.Remove(filepath.Join(s.videoDir, thumb))
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE media_clip_jobs SET status='error',error='De verwerking is onderbroken door een herstart. De vorige versie is bewaard; sla je fragment opnieuw op.' WHERE status='processing'")
	return err
}

// Backup is also available as a separate CLI process. An OS file lock protects
// snapshot-referenced derivatives until the archive has finished copying them.
func (s *Store) lockVideoAssets() (func(), error) {
	s.assetMu.Lock()
	f, err := os.OpenFile(filepath.Join(s.videoDir, ".media-assets.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		s.assetMu.Unlock()
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		s.assetMu.Unlock()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close(); s.assetMu.Unlock() }, nil
}

// Call only when starting the server, never merely when opening a store for backup.
func (s *Store) RecoverVideoEdits() error {
	unlock, err := s.lockVideoAssets()
	if err != nil {
		return err
	}
	defer unlock()
	return s.recoverClips()
}
