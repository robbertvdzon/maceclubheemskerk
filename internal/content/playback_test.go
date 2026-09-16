package content

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"math"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPlaybackValidation(t *testing.T) {
	for _, p := range []Playback{{Start: -1}, {Start: 2, End: 1}, {Start: math.NaN()}, {Chapters: []Chapter{{Time: 2, Title: ""}}}, {Chapters: []Chapter{{Time: 2, Title: "a"}, {Time: 2, Title: "b"}}}, {Start: 3, Chapters: []Chapter{{Time: 2, Title: "a"}}}} {
		if validatePlayback(&p) == nil {
			t.Fatalf("accepted invalid %+v", p)
		}
	}
	p := Playback{Start: 10, End: 100, Chapters: []Chapter{{Time: 10, Title: "  Swing  "}, {Time: 50, Title: "Mill"}}}
	if err := validatePlayback(&p); err != nil || p.Chapters[0].Title != "Swing" {
		t.Fatal(p, err)
	}
}
func testPlaybackManagement(t *testing.T, s *Store, call func(string, string, string, int) *httptest.ResponseRecorder, list func() Library) {
	t.Helper()
	ctx := context.Background()
	yt, err := s.Create(ctx, Item{Type: "video", Section: "exercise", YouTube: "abcdefghijk"}, "test", "")
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("/api/media/%d", yt.ID)
	call("PATCH", url+"/youtube", `{"revision":0,"start":10}`, 409)
	call("PATCH", url+"/youtube", fmt.Sprintf(`{"revision":%d,"start":10,"end":90,"chapters":[{"time":10,"title":"Swing"},{"time":45,"title":"Mill"}]}`, list().Revision), 200)
	find := func(id int64) Item {
		for _, item := range list().Items {
			if item.ID == id {
				return item
			}
		}
		t.Fatal("missing media", id)
		return Item{}
	}
	if p := find(yt.ID).Playback; p.Start != 10 || p.End != 90 || len(p.Chapters) != 2 {
		t.Fatal("timestamps lost", p)
	}
	name := strings.TrimSuffix(randomName(), ".jpg") + ".mp4"
	source := filepath.Join(s.videoDir, name)
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=96x64:rate=30:duration=3", "-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart", "-y", source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	item, err := s.Create(ctx, Item{Type: "video", Section: "training", videoFile: name}, "test", "")
	if err != nil {
		t.Fatal(err)
	}
	url = fmt.Sprintf("/api/media/%d", item.ID)
	call("GET", url+"/editing", "", 200)
	call("GET", url+"/source", "", 200)
	call("POST", url+"/clip", fmt.Sprintf(`{"start":0,"end":4,"thumbnailTime":1,"revision":%d}`, list().Revision), 400)
	call("POST", url+"/clip", fmt.Sprintf(`{"start":1,"end":2,"thumbnailTime":0,"revision":%d}`, list().Revision), 400)
	call("POST", url+"/clip", `{"start":0.5,"end":1.5,"thumbnailTime":0.75,"revision":0}`, 409)
	s.videos <- struct{}{}
	call("POST", url+"/clip", fmt.Sprintf(`{"start":0.5,"end":1.5,"thumbnailTime":0.75,"revision":%d}`, list().Revision), 429)
	<-s.videos
	run := func(start, end, thumb float64) {
		call("POST", url+"/clip", fmt.Sprintf(`{"start":%g,"end":%g,"thumbnailTime":%g,"revision":%d}`, start, end, thumb, list().Revision), 202)
		deadline := time.Now().Add(20 * time.Second)
		for {
			var status struct{ Status, Error string }
			w := call("GET", url+"/clip", "", 200)
			json.Unmarshal(w.Body.Bytes(), &status)
			if status.Status == "error" {
				t.Fatal(status.Error)
			}
			if status.Status == "done" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("clip timed out")
			}
			time.Sleep(20 * time.Millisecond)
		}
		s.uploadWorkers.Wait()
	}
	run(.5, 1.5, .75)
	edited := find(item.ID)
	if edited.Video == item.Video || edited.Thumbnail == "" || edited.Playback.Start != .5 {
		t.Fatal("edit not published", edited)
	}
	out := filepath.Join(s.videoDir, filepath.Base(edited.Video))
	info, err := probeVideo(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	duration, _ := strconv.ParseFloat(info.Format.Duration, 64)
	if math.Abs(duration-1) > .15 {
		t.Fatal("wrong clip duration", duration)
	}
	f, err := os.Open(filepath.Join(s.videoDir, filepath.Base(edited.Thumbnail)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = jpeg.Decode(f); err != nil {
		t.Fatal("bad thumbnail", err)
	}
	f.Close()
	call("GET", edited.Video, "", 200)
	call("GET", edited.Thumbnail, "", 200)
	call("GET", item.Video, "", 404)
	call("GET", url+"/source", "", 200)
	// Backup includes full source, shortened playback and thumbnail.
	if s.dialect == "sqlite" {
		destination := filepath.Join(t.TempDir(), "backup.zip")
		if err = s.Backup(ctx, destination); err != nil {
			t.Fatal(err)
		}
		z, err := zip.OpenReader(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer z.Close()
		for _, file := range []string{name, filepath.Base(edited.Video), filepath.Base(edited.Thumbnail)} {
			found := false
			for _, entry := range z.File {
				if entry.Name == "videos/"+file {
					found = true
				}
			}
			if !found {
				t.Fatal("backup omitted", file)
			}
		}
	}
	// The second edit can use content outside the previous clip; source is preserved.
	run(2, 2.9, 2.5)
	newItem := find(item.ID)
	if newItem.Video == edited.Video {
		t.Fatal("old output reused")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("old playback not cleaned up", err)
	}
	if _, err = os.Stat(source); err != nil {
		t.Fatal("full source lost", err)
	}
	call("DELETE", url, fmt.Sprintf(`{"revision":%d}`, list().Revision), 200)
	call("GET", newItem.Video, "", 404)
	call("GET", newItem.Thumbnail, "", 404)
	call("GET", url+"/source", "", 404)
	call("POST", url+"/restore", fmt.Sprintf(`{"revision":%d}`, list().Revision), 200)
	call("GET", newItem.Video, "", 200)
	// A failed render leaves published media and revision untouched.
	before := list().Revision
	s.processClip(ctx, item.ID, "missing.mp4", strings.Repeat("c", 32)+".mp4", strings.Repeat("c", 32)+".jpg", clipRequest{Start: 0, End: 1, ThumbnailTime: .5})
	if list().Revision != before || find(item.ID).Video != newItem.Video {
		t.Fatal("failed edit changed live media")
	}
	// Crash recovery removes only unfinished generated files, preserving the source and latest render.
	junk := strings.Repeat("b", 32)
	os.WriteFile(filepath.Join(s.videoDir, junk+".mp4"), []byte("incomplete"), 0600)
	_, err = s.db.Exec(s.bind("UPDATE media_clip_jobs SET status='processing',render_file=?,thumbnail_file=? WHERE media_id=?"), junk+".mp4", junk+".jpg", item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.recoverClips(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.videoDir, junk+".mp4")); !os.IsNotExist(err) {
		t.Fatal("unfinished clip retained")
	}
	if find(item.ID).Video != newItem.Video {
		t.Fatal("recovery changed published video")
	}
}
