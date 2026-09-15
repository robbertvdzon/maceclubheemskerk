package content

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
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

func sampleVideo(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/sample.mp4")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestMP4Validation(t *testing.T) {
	good := sampleVideo(t)
	if e := validateMP4(bytes.NewReader(good), int64(len(good))); e != nil {
		t.Fatal(e)
	}
	cases := [][]byte{[]byte("not an mp4"), good[:len(good)/2], bytes.ReplaceAll(good, []byte("avc1"), []byte("hvc1")), bytes.ReplaceAll(good, []byte("moov"), []byte("moof"))}
	bad := bytes.Clone(good)
	binary.BigEndian.PutUint32(bad[:4], 0xffffffff)
	cases = append(cases, bad)
	for i, b := range cases {
		if validateMP4(bytes.NewReader(b), int64(len(b))) == nil {
			t.Fatalf("accepted invalid file %d", i)
		}
	}
}
func FuzzMP4Bounds(f *testing.F) {
	b, _ := os.ReadFile("testdata/sample.mp4")
	f.Add(b)
	f.Add([]byte("bad"))
	f.Fuzz(func(t *testing.T, b []byte) { _ = validateMP4(bytes.NewReader(b), int64(len(b))) })
}

func TestVideoUploadStreamingAuthorizationAndBackup(t *testing.T) {
	dir := t.TempDir()
	s, e := OpenWithVideoDir(filepath.Join(dir, "content.sqlite"), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	sessions, e := auth.OpenStore(filepath.Join(dir, "sessions.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer sessions.Close()
	a := auth.New(auth.Config{ClientID: "test", Origins: []string{"https://club.test"}, SecureCookies: true, MemberEmails: []string{"member@example.test"}}, sessions, func(context.Context, string, string, string) (auth.User, error) { return auth.User{}, nil })
	member, _, _ := sessions.Create(auth.User{ID: "member", Email: "member@example.test"}, "")
	outsider, _, _ := sessions.Create(auth.User{ID: "outsider", Email: "outsider@example.test"}, "")
	mux := http.NewServeMux()
	s.Register(mux, a)
	upload := func(data []byte, token, origin string) *httptest.ResponseRecorder {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		mw.WriteField("section", "exercise")
		mw.WriteField("title", "Oefening")
		mw.WriteField("category", "Basis")
		mw.WriteField("description", "")
		part, _ := mw.CreateFormFile("video", "../../camera.mp4")
		part.Write(data)
		mw.Close()
		r := httptest.NewRequest("POST", "https://club.test/api/videos", &b)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		r.Header.Set("Origin", origin)
		r.AddCookie(&http.Cookie{Name: "__Host-mch-session", Value: token})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	good := sampleVideo(t)
	for _, c := range []struct {
		token, origin string
		code          int
	}{{"", "https://club.test", 401}, {outsider, "https://club.test", 403}, {member, "https://evil.test", 403}, {member, "", 403}} {
		if w := upload(good, c.token, c.origin); w.Code != c.code {
			t.Fatalf("authorization %d: %s", w.Code, w.Body)
		}
	}
	if w := upload([]byte("bad"), member, "https://club.test"); w.Code != 400 {
		t.Fatal(w.Code, w.Body)
	}
	entries, _ := os.ReadDir(s.videoDir)
	if len(entries) != 0 {
		t.Fatal("failed upload left files")
	}
	w := upload(good, member, "https://club.test")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	var item Item
	json.Unmarshal(w.Body.Bytes(), &item)
	if item.Video == "" || item.YouTube != "" {
		t.Fatalf("invalid response %+v", item)
	}
	for _, c := range []struct {
		method, rng string
		code        int
		size        int
	}{{"GET", "", 200, len(good)}, {"GET", "bytes=10-19", 206, 10}, {"HEAD", "", 200, 0}, {"GET", "bytes=999999999-", 416, -1}} {
		r := httptest.NewRequest(c.method, "https://club.test"+item.Video, nil)
		r.Header.Set("Range", c.rng)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != c.code || (c.size >= 0 && w.Body.Len() != c.size) {
			t.Fatalf("range %+v: %d size %d", c, w.Code, w.Body.Len())
		}
		if c.code == 206 && !bytes.Equal(w.Body.Bytes(), good[10:20]) {
			t.Fatal("wrong byte range")
		}
	}
	for _, path := range []string{"/videos/.upload-private", "/videos/sessions.json", "/videos/" + strings.Repeat("a", 32) + ".mp4"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatal("unpublished file accessible")
		}
	}
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if e = s.Backup(context.Background(), archive); e != nil {
		t.Fatal(e)
	}
	z, e := zip.OpenReader(archive)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	found := false
	for _, f := range z.File {
		if f.Name == strings.TrimPrefix(item.Video, "/") {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			found = bytes.Equal(b, good)
		}
	}
	if !found {
		t.Fatal("backup missing uploaded video")
	}
	// Closing the DB after the file was uploaded must clean up the final file on commit failure.
	s.Close()
	before, _ := os.ReadDir(s.videoDir)
	if w = upload(good, member, "https://club.test"); w.Code != 503 {
		t.Fatal(w.Code)
	}
	after, _ := os.ReadDir(s.videoDir)
	if len(before) != len(after) {
		t.Fatal("database failure leaked video")
	}
}
func TestVideoUploadSizeBound(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	// Stream over the limit; never allocate an oversized request in memory.
	reader, writer := io.Pipe()
	mw := multipart.NewWriter(writer)
	go func() {
		defer writer.Close()
		part, e := mw.CreateFormFile("video", "large.mp4")
		if e == nil {
			_, _ = io.CopyN(part, zeroReader{}, maxVideoBytes+1)
		}
		_ = mw.Close()
	}()
	_, path, e := s.receiveVideo(multipart.NewReader(reader, mw.Boundary()))
	reader.Close()
	if path != "" {
		defer os.Remove(path)
	}
	if e != errVideoTooLarge {
		t.Fatalf("limit: %v", e)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestMigrationPreservesExistingLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE media(id INTEGER PRIMARY KEY AUTOINCREMENT,section TEXT,kind TEXT,title TEXT,description TEXT,category TEXT,youtube_id TEXT,photo_file TEXT,created_by TEXT,created_at TEXT);
 INSERT INTO media VALUES(42,'exercise','video','Bestaande oefening','','Basis','abcdefghijk','','member','2026-09-15T12:00:00Z');
 CREATE TABLE content_state(id INTEGER PRIMARY KEY,revision INTEGER);INSERT INTO content_state VALUES(1,17);PRAGMA user_version=1;`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	lib, e := s.List(context.Background())
	if e != nil || len(lib.Items) != 1 || lib.Items[0].ID != 42 || lib.Revision != 17 {
		t.Fatalf("migration lost content %+v %v", lib, e)
	}
	i, e := s.Create(context.Background(), Item{Section: "training", Type: "video", Title: "Nieuw", videoFile: strings.Repeat("b", 32) + ".mp4"}, "member", "")
	if e != nil || i.ID != 43 {
		t.Fatalf("migration IDs %+v %v", i, e)
	}
	if _, e = s.db.Exec("PRAGMA user_version=99"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	if newer, e := Open(path); e == nil {
		newer.Close()
		t.Fatal("accepted newer schema")
	}
}
