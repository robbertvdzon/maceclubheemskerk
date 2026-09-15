package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 65536 - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

type videoProbe struct {
	Streams []struct {
		CodecName     string `json:"codec_name"`
		Width, Height int
		Transfer      string `json:"color_transfer"`
	}
	Format struct{ Duration string }
}

func probeVideo(ctx context.Context, path string) (videoProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"-v", "error", "-max_alloc", "67108864", "-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-use_absolute_path", "0", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height,color_transfer:format=duration", "-of", "json", path}
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var stdout, stderr boundedOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	var info videoProbe
	if err := cmd.Run(); err != nil {
		return info, errors.New("Dit bestand kan niet als MP4- of MOV-video worden gelezen.")
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil || len(info.Streams) != 1 {
		return info, errors.New("Dit bestand bevat geen leesbare video.")
	}
	return info, nil
}

// Conversion reads only the uploaded MOV/MP4 container, never playlists or network URLs.
// CPU threads, duration, dimensions, output size and wall time are bounded.
func transcodeVideo(ctx context.Context, input, output string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	info, err := probeVideo(ctx, input)
	if err != nil {
		return err
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 1800 {
		return errors.New("Kies een video van maximaal 30 minuten.")
	}
	stream := info.Streams[0]
	if stream.Width < 1 || stream.Height < 1 || stream.Width > 4096 || stream.Height > 4096 {
		return errors.New("Kies een video met een resolutie van maximaal 4K.")
	}
	filter := "scale=w='min(1920,iw)':h='min(1080,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2"
	if stream.Transfer == "arib-std-b67" || stream.Transfer == "smpte2084" {
		filter += ",zscale=t=linear:npl=100,format=gbrpf32le,tonemap=tonemap=hable:desat=0,zscale=p=bt709:t=bt709:m=bt709:r=limited"
	}
	filter += ",format=yuv420p"
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-max_alloc", "67108864", "-threads", "2", "-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-use_absolute_path", "0", "-i", input, "-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn", "-filter_threads", "1", "-vf", filter, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-maxrate", "4M", "-bufsize", "8M", "-threads", "2", "-r", "30", "-c:a", "aac", "-b:a", "128k", "-ac", "2", "-movflags", "+faststart", "-fs", strconv.FormatInt(maxChunkedVideoBytes, 10), "-f", "mp4", output}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr boundedOutput
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("De verwerking is gestopt of duurde te lang. Probeer een kortere video.")
		}
		return errors.New("Deze video kon niet worden omgezet. Het bestand is mogelijk beschadigd of gebruikt een niet ondersteund formaat.")
	}
	out, err := probeVideo(ctx, output)
	if err != nil {
		return err
	}
	actual, err := strconv.ParseFloat(out.Format.Duration, 64)
	if err != nil || math.IsNaN(actual) || actual < duration-1 {
		return errors.New("De omgezette video is onvolledig. Probeer een kortere video.")
	}
	if err = os.Chmod(output, 0600); err != nil {
		return err
	}
	f, err := os.OpenFile(output, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() > maxChunkedVideoBytes {
		return fmt.Errorf("De omgezette video is groter dan 1 GB.")
	}
	if err = validateMP4(f, st.Size()); err != nil {
		return err
	}
	return f.Sync()
}

var _ io.Writer = (*boundedOutput)(nil)
