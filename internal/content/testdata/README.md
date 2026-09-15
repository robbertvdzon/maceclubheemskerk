`sample.mp4` is a generated two-second olive frame with a 440 Hz tone (320×240,
H.264 yuv420p + AAC). It contains no personal media. Recreate with:

```sh
ffmpeg -f lavfi -i color=c=olive:s=320x240:d=2 -f lavfi -i sine=frequency=440:duration=2 -c:v libx264 -pix_fmt yuv420p -c:a aac -movflags +faststart sample.mp4
```

`phone.mov` is a synthetic HEVC 10-bit/AAC variant tagged BT.2020 HLG, used to test phone video conversion. Production uses the same FFmpeg codec support, called from Go.
