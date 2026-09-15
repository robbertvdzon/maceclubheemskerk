package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxPhotoBytes = 10 << 20

func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, status int, message string) {
	send(w, status, map[string]string{"error": message})
}
func (s *Store) Register(mux *http.ServeMux, a *auth.Auth) {
	mux.HandleFunc("GET /api/media", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.List(r.Context())
		if e != nil {
			failure(w, 503, "De bibliotheek is tijdelijk niet beschikbaar.")
			return
		}
		send(w, 200, v)
	})
	mux.HandleFunc("POST /api/media", a.RequireUser(func(w http.ResponseWriter, r *http.Request, u auth.User) {
		if !a.CanEdit(u) {
			failure(w, 403, "Alleen toegestane clubaccounts mogen content toevoegen.")
			return
		}
		s.create(w, r, u)
	}))
	mux.HandleFunc("GET /media/{file}", s.photo)
}
func (s *Store) create(w http.ResponseWriter, r *http.Request, u auth.User) {
	// Bound decoded-image memory and concurrent uploads before reading the request.
	select {
	case s.photos <- struct{}{}:
		defer func() { <-s.photos }()
	default:
		failure(w, 429, "Er wordt al iets toegevoegd. Probeer het zo nog eens.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+(64<<10))
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		failure(w, 400, "Ongeldig verzoek.")
		return
	}
	var item Item
	var data []byte
	if mediaType == "application/json" {
		var body struct {
			Section     string `json:"section"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Category    string `json:"category"`
			URL         string `json:"url"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&body); err != nil {
			failure(w, 400, "Ongeldige videogegevens.")
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			failure(w, 400, "Ongeldige videogegevens.")
			return
		}
		item = Item{Section: body.Section, Type: "video", Title: body.Title, Description: body.Description, Category: body.Category}
		item.YouTube, err = YouTubeID(body.URL)
		if err != nil {
			failure(w, 400, err.Error())
			return
		}
	} else if mediaType == "multipart/form-data" {
		// Keep bounded multipart data in memory: the production root filesystem is read-only.
		if err = r.ParseMultipartForm(maxPhotoBytes + (64 << 10)); err != nil {
			failure(w, 400, "Upload maximaal één foto van 10 MB.")
			return
		}
		defer r.MultipartForm.RemoveAll()
		item = Item{Section: r.FormValue("section"), Type: "photo", Title: r.FormValue("title"), Description: r.FormValue("description")}
		if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["photo"]) != 1 {
			failure(w, 400, "Kies één foto.")
			return
		}
		file, _, e := r.FormFile("photo")
		if e != nil {
			failure(w, 400, "Kies een foto.")
			return
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, maxPhotoBytes+1))
		if err != nil || len(data) > maxPhotoBytes {
			failure(w, 400, "Kies een foto van maximaal 10 MB.")
			return
		}
	} else {
		failure(w, 415, "Gebruik een YouTube-link of een foto.")
		return
	}
	if err = Validate(&item); err != nil {
		failure(w, 400, err.Error())
		return
	}
	var name string
	if item.Type == "photo" {
		normalized, e := normalizePhoto(data)
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		name = randomName()
		path := filepath.Join(s.dir, "uploads", name)
		if e = writePhoto(path, normalized); e != nil {
			slog.Error("photo storage failed", "error", e)
			failure(w, 503, "De foto kon niet worden opgeslagen. Probeer opnieuw.")
			return
		}
	}
	created, err := s.Create(r.Context(), item, u.ID, name)
	if err != nil {
		if name != "" {
			_ = os.Remove(filepath.Join(s.dir, "uploads", name))
		}
		slog.Error("content storage failed", "error", err)
		failure(w, 503, "Opslaan is niet gelukt. Probeer opnieuw.")
		return
	}
	send(w, 201, created)
}
func normalizePhoto(data []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") {
		return nil, errors.New("Kies een geldige JPG-, PNG- of WebP-foto.")
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return nil, errors.New("De foto is te groot. Gebruik een afbeelding van maximaal 16 megapixels.")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("Deze foto kan niet worden gelezen.")
	}
	img = orientedImage{Image: img, orientation: jpegOrientation(data)}
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	if width > 2048 || height > 2048 {
		if width >= height {
			height = max(1, height*2048/width)
			width = 2048
		} else {
			width = max(1, width*2048/height)
			height = 2048
		}
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.BiLinear.Scale(canvas, canvas.Bounds(), img, img.Bounds(), draw.Over, nil)
	var out bytes.Buffer
	if err = jpeg.Encode(&out, canvas, &jpeg.Options{Quality: 88}); err != nil {
		return nil, errors.New("De foto kon niet worden verwerkt.")
	}
	return out.Bytes(), nil
}
func writePhoto(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return err
	}
	ok = true
	return nil
}

var photoPattern = regexp.MustCompile(`^[a-f0-9]{32}\.jpg$`)

func (s *Store) photo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !photoPattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), "SELECT 1 FROM media WHERE photo_file=?", name).Scan(&exists); err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.dir, "uploads", name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "inline; filename="+strconv.Quote(name))
	http.ServeContent(w, r, name, time.Time{}, f)
}
