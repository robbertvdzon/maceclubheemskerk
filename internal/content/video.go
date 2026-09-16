package content

import (
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

const maxVideoBytes int64 = 90_000_000   // Leave headroom below Cloudflare's 100 MB request limit.
const videoStorageBudget int64 = 9 << 30 // Keep headroom within the dedicated 10 GiB volume.
var videoPattern = regexp.MustCompile(`^[a-f0-9]{32}\.mp4$`)

func (s *Store) registerVideos(mux *http.ServeMux, a *auth.Auth) {
	mux.HandleFunc("POST /api/videos", a.RequireUser(func(w http.ResponseWriter, r *http.Request, u auth.User) {
		if !a.CanEdit(u) {
			failure(w, 403, "Alleen toegestane clubaccounts mogen video's toevoegen.")
			return
		}
		s.uploadVideo(w, r, u)
	}))
	mux.HandleFunc("GET /videos/{file}", s.video)
	s.registerVideoChunks(mux, a)
}

func (s *Store) videoSpace() error { return s.videoSpaceFor(maxVideoBytes) }
func (s *Store) videoSpaceFor(required int64) error {
	entries, err := os.ReadDir(s.videoDir)
	if err != nil {
		return err
	}
	var used int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		used += info.Size()
	}
	if used+required > videoStorageBudget {
		return errors.New("De video-opslag is bijna vol. Er is geen ruimte voor een nieuwe video.")
	}
	var stat syscall.Statfs_t
	if err = syscall.Statfs(s.videoDir, &stat); err != nil {
		return err
	}
	if uint64(stat.Bavail)*uint64(stat.Bsize) < uint64(required+(128<<20)) {
		return errors.New("Er is onvoldoende vrije opslagruimte voor een nieuwe video.")
	}
	return nil
}
func (s *Store) uploadVideo(w http.ResponseWriter, r *http.Request, u auth.User) {
	select {
	case s.videos <- struct{}{}:
		defer func() { <-s.videos }()
	default:
		failure(w, 429, "Er wordt al een video geüpload. Probeer het zo nog eens.")
		return
	}
	if err := s.videoSpace(); err != nil {
		failure(w, 507, "De video-opslag is bijna vol of niet beschikbaar.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxVideoBytes+(64<<10))
	// Other endpoints keep short timeouts. Video uploads stream to disk with a bounded deadline.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(10 * time.Minute))
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Minute))
	mr, err := r.MultipartReader()
	if err != nil {
		failure(w, 400, "Kies een MP4-video.")
		return
	}
	item, file, err := s.receiveVideo(mr)
	if file != "" {
		defer func() {
			if file != "" {
				_ = os.Remove(file)
			}
		}()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || errors.Is(err, errVideoTooLarge) {
			failure(w, 413, "De video is te groot. Kies een bestand van maximaal 90 MB.")
		} else {
			failure(w, 400, err.Error())
		}
		return
	}
	if err = r.Context().Err(); err != nil {
		return
	}
	name := strings.TrimSuffix(randomName(), ".jpg") + ".mp4"
	item.videoFile = name
	if err = Validate(&item); err != nil {
		failure(w, 400, err.Error())
		return
	}
	f, err := os.Open(file)
	if err != nil {
		failure(w, 503, "De video kon niet worden gelezen.")
		return
	}
	info, err := f.Stat()
	if err == nil {
		err = validateMP4(f, info.Size())
	}
	f.Close()
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	target := filepath.Join(s.videoDir, name)
	if err = os.Rename(file, target); err != nil {
		failure(w, 503, "De video kon niet worden opgeslagen.")
		return
	}
	file = target
	if err = syncDirectory(s.videoDir); err != nil {
		failure(w, 503, "De video kon niet worden opgeslagen.")
		return
	}
	created, err := s.Create(r.Context(), item, u.ID, "")
	if err != nil {
		slog.Error("video metadata storage failed", "error", err)
		failure(w, 503, "Opslaan is niet gelukt. Probeer opnieuw.")
		return
	}
	file = "" // The immutable file now belongs to a committed database row.
	send(w, 201, created)
}

var errVideoTooLarge = errors.New("video too large")

func (s *Store) receiveVideo(mr *multipart.Reader) (Item, string, error) {
	item := Item{Type: "video"}
	values := map[string]string{}
	var path string
	for count := 0; ; count++ {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return item, path, errors.New("De upload is onderbroken. Probeer opnieuw.")
		}
		if count >= 5 {
			return item, path, errors.New("Ongeldige upload.")
		}
		if part.FormName() == "video" {
			if path != "" || part.FileName() == "" {
				return item, path, errors.New("Kies precies één MP4-video.")
			}
			if !strings.EqualFold(filepath.Ext(part.FileName()), ".mp4") {
				return item, path, errors.New("Kies een MP4-bestand met H.264-video.")
			}
			f, err := os.CreateTemp(s.videoDir, ".upload-")
			if err != nil {
				return item, path, errors.New("De video-opslag is niet beschikbaar.")
			}
			path = f.Name()
			n, err := io.Copy(f, io.LimitReader(part, maxVideoBytes+1))
			if err == nil && n > maxVideoBytes {
				err = errVideoTooLarge
			}
			if err == nil && n == 0 {
				err = errors.New("Dit bestand is leeg.")
			}
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err != nil {
				return item, path, err
			}
			if closeErr != nil {
				return item, path, closeErr
			}
		} else {
			name := part.FormName()
			if name != "section" && name != "title" && name != "description" && name != "category" {
				return item, path, errors.New("Ongeldig veld in de upload.")
			}
			if _, ok := values[name]; ok {
				return item, path, errors.New("Dubbel veld in de upload.")
			}
			b, err := io.ReadAll(io.LimitReader(part, 2049))
			if err != nil {
				return item, path, err
			}
			if len(b) > 2048 {
				return item, path, errors.New("De tekst bij de video is te lang.")
			}
			values[name] = string(b)
		}
		if err = part.Close(); err != nil {
			return item, path, err
		}
	}
	if path == "" {
		return item, path, errors.New("Kies een MP4-video.")
	}
	item.Section = values["section"]
	item.Title = values["title"]
	item.Description = values["description"]
	item.Category = values["category"]
	return item, path, nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (s *Store) video(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !videoPattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), s.bind("SELECT 1 FROM media m LEFT JOIN media_playback p ON p.media_id=m.id WHERE CASE WHEN COALESCE(p.render_file,'')<>'' THEN p.render_file ELSE m.video_file END=? AND m.deleted_at=''"), name).Scan(&exists); err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.videoDir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	// Seeking on phones requires byte ranges. ServeContent handles Range, HEAD and 416.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, name, time.Time{}, f)
}
