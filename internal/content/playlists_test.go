package content

import "testing"

func TestSpotifyPlaylistURL(t *testing.T) {
	const id = "4zbpTuVArXmyTCdaBXOudH"
	for _, raw := range []string{"https://open.spotify.com/playlist/" + id, " https://open.spotify.com/playlist/" + id + "?si=shared&utm_source=whatsapp ", "https://open.spotify.com/intl-nl/playlist/" + id + "/"} {
		got, err := spotifyPlaylistID(raw)
		if err != nil || got != id {
			t.Fatal(raw, got, err)
		}
	}
	for _, raw := range []string{"http://open.spotify.com/playlist/" + id, "https://open.spotify.com.evil.test/playlist/" + id, "https://open.spotify.com@evil.test/playlist/" + id, "https://user@open.spotify.com/playlist/" + id, "https://open.spotify.com:443/playlist/" + id, "https://open.spotify.com/track/" + id, "https://open.spotify.com/playlist/short", "javascript:alert(1)", "https://open.spotify.com/playlist/" + id + "/extra"} {
		if _, err := spotifyPlaylistID(raw); err == nil {
			t.Fatal("accepted invalid playlist", raw)
		}
	}
}
