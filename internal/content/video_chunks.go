package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

const maxChunkedVideoBytes int64 = 1_000_000_000
const videoChunkBytes int64 = 8 << 20
const uploadIdleTimeout = 15 * time.Minute

type videoUpload struct {
	id, owner, path string
	size, offset    int64
	item            Item
	done            bool
	processing      bool
	cancel          context.CancelFunc
	problem         string
	timer           *time.Timer
	generation      uint64
}

func (s *Store) registerVideoChunks(mux *http.ServeMux, a *auth.Auth) {
	protect := func(next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
		return a.RequireUser(func(w http.ResponseWriter, r *http.Request, u auth.User) {
			if !a.CanEdit(u) {
				failure(w, 403, "Alleen toegestane clubaccounts mogen video's toevoegen.")
				return
			}
			next(w, r, u)
		})
	}
	mux.HandleFunc("POST /api/video-uploads", protect(s.beginVideoUpload))
	mux.HandleFunc("PUT /api/video-uploads/{id}", protect(s.appendVideoUpload))
	mux.HandleFunc("GET /api/video-uploads/{id}", protect(s.videoUploadStatus))
	mux.HandleFunc("POST /api/video-uploads/{id}/complete", protect(s.completeVideoUpload))
	mux.HandleFunc("DELETE /api/video-uploads/{id}", protect(s.cancelVideoUpload))
}

// uploadMu protects state and serializes chunks, bounding memory to one 8 MiB chunk.
// The existing videos semaphore also excludes legacy multipart uploads.
func (s *Store) beginVideoUpload(w http.ResponseWriter, r *http.Request, u auth.User) {
	var input struct {
		Section, Title, Description, Category, Filename string
		Size                                            int64
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(&input); e != nil {
		failure(w, 400, "Ongeldige video-informatie.")
		return
	}
	if input.Size < 1 || input.Size > maxChunkedVideoBytes {
		failure(w, 413, "Kies een video van maximaal 1 GB.")
		return
	}
	if !strings.EqualFold(filepath.Ext(input.Filename), ".mp4") && !strings.EqualFold(filepath.Ext(input.Filename), ".mov") {
		failure(w, 400, "Kies een MP4- of MOV-video.")
		return
	}
	id := r.Header.Get("Upload-ID")
	if len(id) != 32 || !videoPattern.MatchString(id+".mp4") {
		failure(w, 400, "Ongeldig uploadnummer.")
		return
	}
	item := Item{Section: input.Section, Type: "video", Title: input.Title, Description: input.Description, Category: input.Category, videoFile: strings.TrimSuffix(randomName(), ".jpg") + ".mp4"}
	if e := Validate(&item); e != nil {
		failure(w, 400, e.Error())
		return
	}
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	if s.chunkUploads == nil {
		s.chunkUploads = make(map[string]*videoUpload)
	}
	if existing := s.chunkUploads[id]; existing != nil {
		if existing.owner != u.ID || existing.size != input.Size || existing.item.Title != item.Title || existing.item.Section != item.Section || existing.item.Description != item.Description || existing.item.Category != item.Category {
			failure(w, 409, "Dit uploadnummer is al in gebruik.")
			return
		}
		s.touchUpload(existing)
		send(w, 200, map[string]any{"id": id, "offset": existing.offset, "chunkSize": videoChunkBytes})
		return
	}
	if len(s.chunkUploads) >= 16 {
		failure(w, 429, "Wacht even voordat je een nieuwe upload start.")
		return
	}
	select {
	case s.videos <- struct{}{}:
	default:
		failure(w, 429, "Er wordt al een video geüpload. Annuleer die upload of probeer het later opnieuw.")
		return
	}
	s.removeStaleVideoParts()
	if e := s.videoSpaceFor(input.Size + maxChunkedVideoBytes); e != nil {
		<-s.videos
		failure(w, 507, "Er is onvoldoende vrije video-opslag.")
		return
	}
	f, e := os.CreateTemp(s.videoDir, ".upload-")
	if e != nil {
		<-s.videos
		failure(w, 503, "De video-opslag is niet beschikbaar.")
		return
	}
	f.Close()
	upload := &videoUpload{id: id, owner: u.ID, path: f.Name(), size: input.Size, item: item}
	s.chunkUploads[id] = upload
	s.touchUpload(upload)
	send(w, 201, map[string]any{"id": id, "offset": 0, "chunkSize": videoChunkBytes})
}
func (s *Store) touchUpload(upload *videoUpload) {
	if upload.processing {
		return
	}
	if upload.timer != nil {
		upload.timer.Stop()
	}
	// A generation prevents a previously firing timer from expiring refreshed state.
	upload.generation++
	generation := upload.generation
	upload.timer = time.AfterFunc(uploadIdleTimeout, func() {
		s.uploadMu.Lock()
		defer s.uploadMu.Unlock()
		if current := s.chunkUploads[upload.id]; current == upload && upload.generation == generation {
			s.removeUpload(upload)
		}
	})
}
func (s *Store) removeUpload(upload *videoUpload) {
	if upload.timer != nil {
		upload.timer.Stop()
	}
	if !upload.done && upload.problem == "" {
		_ = os.Remove(upload.path)
		<-s.videos
	}
	delete(s.chunkUploads, upload.id)
}
func (s *Store) findUpload(w http.ResponseWriter, r *http.Request, u auth.User) *videoUpload {
	upload := s.chunkUploads[r.PathValue("id")]
	if upload == nil || upload.owner != u.ID {
		failure(w, 404, "Deze upload is verlopen of onderbroken door een herstart. Klik opnieuw op Toevoegen.")
		return nil
	}
	return upload
}
func (s *Store) appendVideoUpload(w http.ResponseWriter, r *http.Request, u auth.User) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	upload := s.findUpload(w, r, u)
	if upload == nil {
		return
	}
	if upload.done || upload.processing || upload.problem != "" {
		failure(w, 409, "Deze video is al opgeslagen.")
		return
	}
	offset, e := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if e != nil || offset < 0 || offset > upload.offset {
		failure(w, 409, "Het uploaddeel staat niet op de verwachte positie.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, videoChunkBytes)
	data, e := io.ReadAll(r.Body)
	if e != nil {
		var limit *http.MaxBytesError
		if errors.As(e, &limit) {
			failure(w, 413, "Het uploaddeel is te groot.")
		} else {
			failure(w, 400, "Het uploaddeel is onderbroken. Probeer opnieuw.")
		}
		return
	}
	if len(data) == 0 || int64(len(data)) > upload.size-offset {
		failure(w, 400, "Het uploaddeel heeft een ongeldige grootte.")
		return
	}
	f, e := os.OpenFile(upload.path, os.O_RDWR, 0600)
	if e != nil {
		failure(w, 503, "De upload kon niet worden geopend.")
		return
	}
	defer f.Close()
	if offset < upload.offset {
		// Retrying an acknowledged/lost response must not append the same bytes twice.
		if offset+int64(len(data)) > upload.offset {
			failure(w, 409, "Overlappende uploaddelen.")
			return
		}
		stored := make([]byte, len(data))
		if _, e = f.ReadAt(stored, offset); e != nil || !bytes.Equal(data, stored) {
			failure(w, 409, "Dit uploaddeel verschilt van het al ontvangen deel.")
			return
		}
	} else {
		if _, e = f.WriteAt(data, offset); e == nil {
			e = f.Sync()
		}
		if e != nil {
			_ = f.Truncate(offset)
			failure(w, 507, "Dit uploaddeel kon niet worden opgeslagen. Controleer de vrije opslagruimte.")
			return
		}
		upload.offset += int64(len(data))
	}
	s.touchUpload(upload)
	send(w, 200, map[string]int64{"offset": upload.offset})
}
func (s *Store) videoUploadStatus(w http.ResponseWriter, r *http.Request, u auth.User) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	upload := s.findUpload(w, r, u)
	if upload == nil {
		return
	}
	s.touchUpload(upload)
	s.sendUploadStatus(w, upload)
}
func (s *Store) sendUploadStatus(w http.ResponseWriter, upload *videoUpload) {
	if upload.problem != "" {
		send(w, 200, map[string]any{"state": "failed", "error": upload.problem})
		return
	}
	if upload.done {
		send(w, 200, map[string]any{"state": "complete", "item": upload.item})
		return
	}
	state := "uploading"
	if upload.processing {
		state = "processing"
	}
	send(w, 200, map[string]any{"state": state, "offset": upload.offset})
}
func (s *Store) completeVideoUpload(w http.ResponseWriter, r *http.Request, u auth.User) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	upload := s.findUpload(w, r, u)
	if upload == nil {
		return
	}
	if upload.done || upload.processing || upload.problem != "" {
		s.sendUploadStatus(w, upload)
		return
	}
	if upload.offset != upload.size {
		failure(w, 409, "De video is nog niet volledig geüpload.")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	upload.cancel = cancel
	upload.processing = true
	upload.generation++
	if upload.timer != nil {
		upload.timer.Stop()
	}
	s.uploadWorkers.Add(1)
	go s.processVideoUpload(ctx, upload)
	send(w, 202, map[string]string{"state": "processing"})
}
func (s *Store) processVideoUpload(ctx context.Context, upload *videoUpload) {
	defer s.uploadWorkers.Done()
	defer upload.cancel()
	output := upload.path + ".converted.mp4"
	err := s.convertVideo(ctx, upload.path, output)
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	defer os.Remove(upload.path)
	defer os.Remove(output)
	if err == nil {
		err = ctx.Err()
	}
	target := filepath.Join(s.videoDir, upload.item.videoFile)
	if err == nil {
		err = os.Rename(output, target)
	}
	if err == nil {
		err = syncDirectory(s.videoDir)
	}
	if err == nil {
		upload.item, err = s.Create(ctx, upload.item, upload.owner, "")
	}
	upload.processing = false
	if err != nil {
		_ = os.Remove(target)
		upload.problem = err.Error()
	} else {
		upload.done = true
	}
	<-s.videos
	s.touchUpload(upload)
}
func (s *Store) cancelVideoUpload(w http.ResponseWriter, r *http.Request, u auth.User) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	upload := s.findUpload(w, r, u)
	if upload == nil {
		return
	}
	if upload.done {
		s.sendUploadStatus(w, upload)
		return
	}
	if upload.processing {
		upload.cancel()
	} else {
		s.removeUpload(upload)
	}
	w.WriteHeader(204)
}

// Remove abandoned temporary files left by a hard process/node restart. Active
// uploads time out after 15 minutes and conversion after 30 minutes.
func (s *Store) removeStaleVideoParts() {
	entries, err := os.ReadDir(s.videoDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".upload-") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(time.Now().Add(-24*time.Hour)) {
			_ = os.Remove(filepath.Join(s.videoDir, entry.Name()))
		}
	}
}
