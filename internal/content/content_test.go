package content

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

func TestSQLitePersistsAndBackupRestoresWithPhotos(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "maceclub.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	file := randomName()
	data := []byte("photo contents")
	if err = writePhoto(filepath.Join(s.dir, "uploads", file), data); err != nil {
		t.Fatal(err)
	}
	_, err = s.Create(ctx, Item{Section: "training", Type: "photo", Title: "Training"}, "google-sub", file)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "backup.zip")
	if err = s.Backup(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if err = s.Backup(ctx, snapshot); !os.IsExist(err) {
		t.Fatal("backup overwrite was not rejected")
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	library, err := s.List(ctx)
	if err != nil || len(library.Items) != 1 || library.Revision != 1 || library.Items[0].Photo != "/media/"+file {
		t.Fatalf("restart lost content: %+v %v", library, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(raw, []byte("SQLite format 3")) {
		t.Fatal("not a real SQLite file")
	}
	z, err := zip.OpenReader(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	restored := t.TempDir()
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(restored, f.Name)
		os.MkdirAll(filepath.Dir(target), 0700)
		if err = os.WriteFile(target, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(restored, "maceclub.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var integrity string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal("invalid backup", err, integrity)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM media").Scan(&count); err != nil || count != 1 {
		t.Fatal("backup missing row", err)
	}
	b, err := os.ReadFile(filepath.Join(restored, "uploads", file))
	if err != nil || !bytes.Equal(b, data) {
		t.Fatal("backup missing photo")
	}
}
func TestYouTubeURLValidation(t *testing.T) {
	for _, link := range []string{"https://youtu.be/abcdefghijk?t=5", "https://www.youtube.com/watch?v=abcdefghijk", "https://youtube.com/shorts/abcdefghijk", "https://youtube.com/live/abcdefghijk"} {
		id, e := YouTubeID(link)
		if e != nil || id != "abcdefghijk" {
			t.Fatal(link, id, e)
		}
	}
	for _, link := range []string{"https://evil.example/watch?v=abcdefghijk", "https://youtube.com.evil.test/watch?v=abcdefghijk", "javascript:alert(1)", "https://youtube.com/watch?v=bad", "https://user@youtube.com/watch?v=abcdefghijk", "https://youtube.com:999/watch?v=abcdefghijk"} {
		if _, e := YouTubeID(link); e == nil {
			t.Fatal("accepted", link)
		}
	}
}
func TestHTTPAuthorizationValidationAndUpload(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "content.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessions, err := auth.OpenStore(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Close()
	cfg := auth.Config{ClientID: "test-client", Origins: []string{"https://club.test"}, SecureCookies: true, MemberEmails: []string{"member@example.test"}}
	a := auth.New(cfg, sessions, func(context.Context, string, string, string) (auth.User, error) { return auth.User{}, nil })
	member, _, _ := sessions.Create(auth.User{ID: "member", Email: "member@example.test"}, "")
	outsider, _, _ := sessions.Create(auth.User{ID: "outsider", Email: "outsider@example.test"}, "")
	mux := http.NewServeMux()
	s.Register(mux, a)
	request := func(method, path, body, token, origin, ctype string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://club.test"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", ctype)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "__Host-mch-session", Value: token})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	body := `{"section":"exercise","title":"360 swing","category":"Basis","url":"https://youtu.be/abcdefghijk"}`
	for _, c := range []struct {
		token, origin string
		want          int
	}{{"", "https://club.test", 401}, {outsider, "https://club.test", 403}, {member, "https://evil.test", 403}, {member, "", 403}} {
		w := request("POST", "/api/media", body, c.token, c.origin, "application/json")
		if w.Code != c.want {
			t.Fatalf("auth: %d != %d", w.Code, c.want)
		}
	}
	a.Config.MemberEmails = nil
	if w := request("POST", "/api/media", body, member, "https://club.test", "application/json"); w.Code != 403 {
		t.Fatal("empty allowlist allowed write")
	}
	a.Config.MemberEmails = cfg.MemberEmails
	if w := request("POST", "/api/media", body, member, "https://club.test", "application/json"); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/api/media", strings.Replace(body, "360 swing", strings.Repeat("x", 101), 1), member, "https://club.test", "application/json"); w.Code != 400 {
		t.Fatal("accepted oversized title")
	}
	makeUpload := func(contents []byte) (string, string) {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		mw.WriteField("section", "training")
		mw.WriteField("title", "Foto")
		file, _ := mw.CreateFormFile("photo", "../../untrusted.jpg")
		file.Write(contents)
		mw.Close()
		return b.String(), mw.FormDataContentType()
	}
	bad, ct := makeUpload([]byte("<svg onload=alert(1)></svg>"))
	if w := request("POST", "/api/media", bad, member, "https://club.test", ct); w.Code != 400 {
		t.Fatal("accepted unsafe image")
	}
	var picture bytes.Buffer
	// Use a bounded concrete image: no reliance on filename or claimed MIME type.
	picture.Reset()
	png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 3)))
	good, ct := makeUpload(picture.Bytes())
	w := request("POST", "/api/media", good, member, "https://club.test", ct)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var item Item
	json.Unmarshal(w.Body.Bytes(), &item)
	w = request("GET", item.Photo, "", "", "", "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal("photo not available")
	}
	if _, e := jpeg.Decode(bytes.NewReader(w.Body.Bytes())); e != nil {
		t.Fatal(e)
	}
	list, err := s.List(context.Background())
	if err != nil || len(list.Items) != 2 || list.Revision != 2 {
		t.Fatalf("invalid writes changed data: %+v %v", list, err)
	}
	a.Config.MemberEmails = nil
	if w := request("POST", "/api/media", body, member, "https://club.test", "application/json"); w.Code != 403 {
		t.Fatal("revoked member retained permission")
	}
	for _, p := range []string{"/media/sessions.json", "/media/content.sqlite", "/media/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg"} {
		if w := request("GET", p, "", "", "", ""); w.Code != 404 {
			t.Fatal("private/unknown file accessible", p)
		}
	}
}
func TestPhotoOrientationAndBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	rot := orientedImage{Image: img, orientation: 6}
	if rot.Bounds().Dx() != 3 || rot.Bounds().Dy() != 2 || rot.At(2, 0) != img.At(0, 0) {
		t.Fatal("camera rotation failed")
	}
	if _, err := normalizePhoto([]byte("not an image")); err == nil {
		t.Fatal("accepted invalid photo")
	}
}
