package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

// The PostgreSQL URL must point to an empty, disposable test database.
func TestManagementAndMigration(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "club.sqlite")
			driver := "sqlite"
			if dialect == "postgres" {
				target = os.Getenv("MCH_TEST_POSTGRES")
				driver = "pgx"
				if target == "" {
					t.Skip("set MCH_TEST_POSTGRES to an empty disposable database")
				}
			}
			db, err := sql.Open(driver, target)
			if err != nil {
				t.Fatal(err)
			}
			// Reproduce the previous production schema before running the new migration.
			schema := mediaSchemaSQLite
			if dialect == "postgres" {
				schema = mediaSchemaPostgres
			}
			for _, q := range []string{schema, "CREATE TABLE content_state (id INTEGER PRIMARY KEY, revision BIGINT NOT NULL)", "INSERT INTO content_state VALUES(1,2)", `INSERT INTO media(id,section,kind,title,category,youtube_id,created_by,created_at) VALUES(1,'exercise','video','Old title','Basis','abcdefghijk','test','2026-01-01'),(2,'training','video','Second','','abcdefghijk','test','2026-01-02')`} {
				if _, err = db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			if dialect == "sqlite" {
				db.Exec("PRAGMA user_version=2")
			} else {
				db.Exec("SELECT setval(pg_get_serial_sequence('media','id'),2,true)")
			}
			db.Close()
			s, err := OpenWithVideoDir(target, filepath.Join(dir, "videos"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			s.dir = dir
			os.MkdirAll(filepath.Join(dir, "uploads"), 0700)
			ctx := context.Background()
			list := func() Library {
				v, e := s.List(ctx)
				if e != nil {
					t.Fatal(e)
				}
				return v
			}
			v := list()
			if len(v.Items) != 2 || v.Items[0].ID != 2 || v.Items[1].Title != "Old title" || v.Revision != 2 {
				t.Fatalf("migration changed old media: %+v", v)
			}
			sessions, e := auth.OpenStore(filepath.Join(dir, "sessions.json"))
			if e != nil {
				t.Fatal(e)
			}
			defer sessions.Close()
			a := auth.New(auth.Config{ClientID: "test-client", Origins: []string{"https://club.test"}, SecureCookies: true, MemberEmails: []string{"member@example.test"}}, sessions, func(context.Context, string, string, string) (auth.User, error) { return auth.User{}, nil })
			member, _, _ := sessions.Create(auth.User{ID: "member", Email: "member@example.test"}, "")
			outsider, _, _ := sessions.Create(auth.User{ID: "other", Email: "other@example.test"}, "")
			mux := http.NewServeMux()
			s.Register(mux, a)
			request := func(method, path, body, token, origin string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "https://club.test"+path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
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
			for _, route := range []struct{ method, path string }{{"PATCH", "/api/media/1"}, {"DELETE", "/api/media/1"}, {"POST", "/api/media/1/move"}, {"POST", "/api/media/1/restore"}, {"PATCH", "/api/bingo/1"}, {"GET", "/api/media/trash"}, {"GET", "/api/media/1/source"}, {"GET", "/api/media/1/editing"}, {"GET", "/api/media/1/clip"}, {"POST", "/api/media/1/clip"}, {"PATCH", "/api/media/1/youtube"}, {"POST", "/api/playlists"}, {"DELETE", "/api/playlists/4zbpTuVArXmyTCdaBXOudH"}} {
				for _, who := range []struct {
					token string
					want  int
				}{{"", 401}, {outsider, 403}} {
					w := request(route.method, route.path, `{}`, who.token, "https://club.test")
					if w.Code != who.want {
						t.Fatalf("%s unauthorized status %d", route.path, w.Code)
					}
				}
				if route.method != "GET" {
					for _, origin := range []string{"", "https://evil.test"} {
						w := request(route.method, route.path, `{}`, member, origin)
						if w.Code != 403 {
							t.Fatal("unsafe origin allowed", route.path, w.Code)
						}
					}
				}
			}
			call := func(method, path, body string, want int) *httptest.ResponseRecorder {
				w := request(method, path, body, member, "https://club.test")
				if w.Code != want {
					t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
				}
				return w
			}
			w := call("POST", "/api/media", `{"section":"training","title":"","description":"","url":"https://youtu.be/abcdefghijk"}`, 201)
			var blank Item
			json.Unmarshal(w.Body.Bytes(), &blank)
			if blank.Title != "" {
				t.Fatal("title unexpectedly required")
			}
			mutate := func(method, path, fields string, want int) {
				revision := list().Revision
				call(method, path, fmt.Sprintf(`{"revision":%d%s}`, revision, fields), want)
			}
			call("POST", "/api/media", `{"section":"exercise","title":"","category":"","url":"https://youtu.be/abcdefghijk"}`, 201)
			call("POST", "/api/media", `{"section":"training","title":"Album","url":"https://youtu.be/abcdefghijk"}`, 201)
			sections := func(section string) []int64 {
				var ids []int64
				for _, item := range list().Items {
					if item.Section == section {
						ids = append(ids, item.ID)
					}
				}
				return ids
			}
			albumOrder := fmt.Sprint(sections("training"))
			mutate("PATCH", "/api/media/1", `,"title":"Updated","description":"New text"`, 200)
			call("PATCH", "/api/media/1", `{"revision":2,"title":"stale"}`, 409)
			mutate("POST", "/api/media/1/move", `,"direction":"up"`, 200)
			v = list()
			if fmt.Sprint(sections("exercise")) != "[1 4]" || fmt.Sprint(sections("training")) != albumOrder {
				t.Fatal("order not changed", v)
			}
			mutate("POST", "/api/media/1/move", `,"direction":"sideways"`, 400)
			mutate("PATCH", "/api/media/1", `,"title":"","description":""`, 200)
			mutate("PATCH", "/api/media/1", `,"title":"","section":"training"`, 200)
			if fmt.Sprint(sections("exercise")) != "[4]" {
				t.Fatal("section edit did not move video")
			}
			mutate("PATCH", "/api/media/1", `,"title":"","section":"exercise"`, 200)
			mutate("PATCH", "/api/media/1", `,"section":"invalid"`, 400)
			os.WriteFile(filepath.Join(s.videoDir, strings.Repeat("a", 32)+".mp4"), []byte("existing test asset"), 0600)
			uploaded, e := s.Create(ctx, Item{Section: "exercise", Type: "video", videoFile: strings.Repeat("a", 32) + ".mp4"}, "member", "")
			if e != nil || uploaded.Section != "exercise" {
				t.Fatal("uploaded exercise lost section", e)
			}
			if _, e = s.Create(ctx, Item{Section: "exercise", Type: "photo"}, "member", randomName()); e == nil {
				t.Fatal("exercise accepted photo")
			}

			photo := randomName()
			os.WriteFile(filepath.Join(dir, "uploads", photo), []byte("test image"), 0600)
			p, e := s.Create(ctx, Item{Section: "training", Type: "photo"}, "member", photo)
			if e != nil {
				t.Fatal(e)
			}
			path := fmt.Sprintf("/api/media/%d", p.ID)
			mutate("PATCH", path, `,"section":"exercise"`, 400)
			mutate("DELETE", path, "", 200)
			if w := request("GET", p.Photo, "", "", ""); w.Code != 404 {
				t.Fatal("deleted photo still public")
			}
			call("GET", "/api/media/trash", "", 200)
			mutate("POST", path+"/restore", "", 200)
			if w := request("GET", p.Photo, "", "", ""); w.Code != 200 {
				t.Fatal("restored photo not public", w.Code)
			}
			mutate("DELETE", path, "", 200)
			bingo := func() []BingoCell {
				w := request("GET", "/api/bingo", "", "", "")
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				var data struct{ Cells []BingoCell }
				json.Unmarshal(w.Body.Bytes(), &data)
				return data.Cells
			}
			cells := bingo()
			if len(cells) != 25 || cells[12].Checked || cells[12].Text != "Mijn mace ligt nog in de auto" || cells[0].Text != "Te druk" {
				t.Fatal("seed missing", cells)
			}
			call("PATCH", "/api/bingo/1", `{"text":"Mijn mace slaapt nog","checked":true,"version":1}`, 200)
			before := list().Revision
			call("PATCH", "/api/bingo/1", `{"text":"stale","checked":false,"version":1}`, 409)
			if list().Revision != before {
				t.Fatal("conflict changed revision")
			}
			call("PATCH", "/api/bingo/1", `{"text":"","version":2}`, 400)
			expected := list()
			s.Close()
			s, e = OpenWithVideoDir(target, filepath.Join(dir, "videos"))
			if e != nil {
				t.Fatal(e)
			}
			s.dir = dir
			mux = http.NewServeMux()
			s.Register(mux, a)
			got := list()
			if fmt.Sprint(got) != fmt.Sprint(expected) {
				t.Fatalf("restart changed library: %+v != %+v", got, expected)
			}
			cells = bingo()
			if !cells[0].Checked || cells[0].Text != "Mijn mace slaapt nog" || cells[0].Version != 2 {
				t.Fatal("bingo edits lost after restart", cells[0])
			}
			call("PATCH", "/api/bingo/1", `{"text":"Mijn mace slaapt nog","checked":false,"version":2}`, 200)
			if bingo()[0].Checked {
				t.Fatal("unchecking failed")
			}

			playlists := func() []Playlist {
				w := request("GET", "/api/playlists", "", "", "")
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				var result struct{ Items []Playlist }
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				return result.Items
			}
			seed := playlists()
			if len(seed) != 1 || seed[0].ID != "4zbpTuVArXmyTCdaBXOudH" {
				t.Fatal("missing initial playlist", seed)
			}
			const addedURL = "https://open.spotify.com/playlist/abcdefghijklmnopqrstuv?si=tracking"
			payload := `{"url":"` + addedURL + `","title":"Clubmuziek"}`
			before = list().Revision
			call("POST", "/api/playlists", `{"url":"https://evil.test/playlist/abcdefghijklmnopqrstuv"}`, 400)
			if list().Revision != before {
				t.Fatal("invalid playlist changed revision")
			}
			call("POST", "/api/playlists", payload, 201)
			call("POST", "/api/playlists", payload, 409)
			if list().Revision != before+1 || len(playlists()) != 2 {
				t.Fatal("playlist insert/duplicate failed")
			}
			call("DELETE", "/api/playlists/abcdefghijklmnopqrstuv", `{"revision":0}`, 409)
			mutate("DELETE", "/api/playlists/abcdefghijklmnopqrstuv", "", 200)
			if len(playlists()) != 1 {
				t.Fatal("playlist not hidden")
			}
			call("POST", "/api/playlists", payload, 201)
			if got := playlists(); len(got) != 2 || got[1].Title != "Clubmuziek" {
				t.Fatal("playlist not restored", got)
			}
			// A removed initial playlist must not be reseeded by startup/migration.
			mutate("DELETE", "/api/playlists/4zbpTuVArXmyTCdaBXOudH", "", 200)
			s.Close()
			s, e = OpenWithVideoDir(target, filepath.Join(dir, "videos"))
			if e != nil {
				t.Fatal(e)
			}
			s.dir = dir
			mux = http.NewServeMux()
			s.Register(mux, a)
			if got := playlists(); len(got) != 1 || got[0].ID != "abcdefghijklmnopqrstuv" {
				t.Fatal("playlist state lost on restart", got)
			}

			testPlaybackManagement(t, s, call, list)

			// Upgrade a schema-3 board without touching other cells or custom center text.
			for _, center := range []struct {
				text    string
				checked int
			}{{"Vrij vak", 1}, {"Vrij vak", 0}, {"Mijn eigen excuus", 1}} {
				query := "UPDATE bingo_cells SET text=?,checked=?,version=4 WHERE id=13"
				versionQuery := "PRAGMA user_version=3"
				if dialect == "postgres" {
					query = "UPDATE bingo_cells SET text=$1,checked=$2,version=4 WHERE id=13"
					versionQuery = "UPDATE club_schema SET version=3 WHERE id=1"
				}
				if _, err := s.db.Exec(query, center.text, center.checked); err != nil {
					t.Fatal(err)
				}
				if _, err := s.db.Exec(versionQuery); err != nil {
					t.Fatal(err)
				}
				beforeCells, beforeRevision := bingo(), list().Revision
				if err := migrate(s.db, dialect); err != nil {
					t.Fatal(err)
				}
				want := beforeCells[12]
				wantRevision := beforeRevision
				if center.text == "Vrij vak" {
					want.Text, want.Checked, want.Version = "Mijn mace ligt nog in de auto", false, 5
					wantRevision++
				}
				for i, cell := range bingo() {
					expected := beforeCells[i]
					if i == 12 {
						expected = want
					}
					if cell != expected {
						t.Fatalf("migration changed cell incorrectly: %+v != %+v", cell, expected)
					}
				}
				if list().Revision != wantRevision {
					t.Fatal("incorrect migration revision")
				}
				if err := migrate(s.db, dialect); err != nil {
					t.Fatal(err)
				}
				if bingo()[12] != want || list().Revision != wantRevision {
					t.Fatal("migration repeated on restart")
				}
			}
		})
	}
}
