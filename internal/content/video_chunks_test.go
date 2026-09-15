package content

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestChunkedUploadRetryOwnershipAndConversion(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	sessions, e := auth.OpenStore(filepath.Join(t.TempDir(), "sessions"))
	if e != nil {
		t.Fatal(e)
	}
	defer sessions.Close()
	a := auth.New(auth.Config{ClientID: "test", Origins: []string{"https://club.test"}, SecureCookies: true, MemberEmails: []string{"a@test.test", "b@test.test"}}, sessions, func(context.Context, string, string, string) (auth.User, error) { return auth.User{}, nil })
	owner, _, _ := sessions.Create(auth.User{ID: "a", Email: "a@test.test"}, "")
	other, _, _ := sessions.Create(auth.User{ID: "b", Email: "b@test.test"}, "")
	mux := http.NewServeMux()
	s.Register(mux, a)
	id := strings.Repeat("c", 32)
	url := "/api/video-uploads/" + id
	call := func(method, path string, body []byte, token, offset string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://club.test"+path, bytes.NewReader(body))
		r.Header.Set("Origin", "https://club.test")
		r.Header.Set("Upload-ID", id)
		r.Header.Set("Upload-Offset", offset)
		r.AddCookie(&http.Cookie{Name: "__Host-mch-session", Value: token})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	data, e := os.ReadFile("testdata/phone.mov")
	if e != nil {
		t.Fatal(e)
	}
	// Add a valid large free box to reproduce >90 MB without storing large fixtures in Git.
	large := make([]byte, 91_000_000)
	copy(large, data)
	n := len(large) - len(data)
	large[len(data)] = byte(uint32(n) >> 24)
	large[len(data)+1] = byte(uint32(n) >> 16)
	large[len(data)+2] = byte(uint32(n) >> 8)
	large[len(data)+3] = byte(n)
	copy(large[len(data)+4:], "free")
	metadata := []byte(`{"section":"training","title":"Phone","filename":"phone.mov","size":` + strconv.Itoa(len(large)) + `}`)
	for _, code := range []int{201, 200} {
		if w := call("POST", "/api/video-uploads", metadata, owner, ""); w.Code != code {
			t.Fatal(w.Code, w.Body)
		}
	}
	if w := call("PUT", url, large[:10], other, "0"); w.Code != 404 {
		t.Fatal("other member owns upload", w.Code)
	}
	if w := call("POST", url+"/complete", nil, owner, ""); w.Code != 409 {
		t.Fatal("incomplete committed")
	}
	for off := 0; off < len(large); {
		end := min(len(large), off+int(videoChunkBytes))
		chunk := large[off:end]
		w := call("PUT", url, chunk, owner, strconv.Itoa(off))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		if off == 0 {
			w = call("PUT", url, chunk, owner, "0")
			if w.Code != 200 {
				t.Fatal("retry failed", w.Body)
			}
			bad := bytes.Clone(chunk)
			bad[10] ^= 1
			if w = call("PUT", url, bad, owner, "0"); w.Code != 409 {
				t.Fatal("conflicting retry accepted")
			}
		}
		off = end
	}
	if w := call("POST", url+"/complete", nil, owner, ""); w.Code != 202 {
		t.Fatal(w.Code, w.Body)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		w := call("GET", url, nil, owner, "")
		var state struct {
			State, Error string
			Item         Item
		}
		if e = json.Unmarshal(w.Body.Bytes(), &state); e != nil {
			t.Fatal(e)
		}
		if state.State == "failed" {
			t.Fatal(state.Error)
		}
		if state.State == "complete" {
			if state.Item.Video == "" {
				t.Fatal("missing video")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("conversion timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if w := call("POST", url+"/complete", nil, owner, ""); w.Code != 200 {
		t.Fatal("complete retry failed")
	}
	lib, _ := s.List(context.Background())
	if len(lib.Items) != 1 {
		t.Fatal("duplicate records")
	}
	entries, _ := os.ReadDir(s.videoDir)
	if len(entries) != 1 {
		t.Fatal("temporary files leaked", len(entries))
	}
	// New upload cancellation removes temporary data and frees the single upload slot.
	id = strings.Repeat("d", 32)
	url = "/api/video-uploads/" + id
	if w := call("POST", "/api/video-uploads", metadata, owner, ""); w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if w := call("DELETE", url, nil, other, ""); w.Code != 404 {
		t.Fatal("other member cancelled upload")
	}
	if w := call("DELETE", url, nil, owner, ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := call("GET", url, nil, owner, ""); w.Code != 404 {
		t.Fatal("cancelled upload retained")
	}
}
func TestTranscodeRejectsInvalidVideo(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	os.WriteFile(input, []byte("not a video"), 0600)
	if e := transcodeVideo(context.Background(), input, filepath.Join(dir, "out.mp4")); e == nil {
		t.Fatal("invalid accepted")
	}
}
