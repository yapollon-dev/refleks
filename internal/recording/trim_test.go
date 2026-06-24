package recording

import "testing"

func TestTrimEncoderConfigForUsesSelectedEncoder(t *testing.T) {
	cfg := trimEncoderConfigFor("h264_nvenc")
	if cfg.Name != "h264_nvenc" {
		t.Fatalf("encoder = %q, want h264_nvenc", cfg.Name)
	}
	if len(cfg.Args) == 0 {
		t.Fatal("selected encoder has no ffmpeg args")
	}
}

func TestTrimEncoderConfigForFallsBackToCPUEncoder(t *testing.T) {
	for _, name := range []string{"", "unknown-encoder"} {
		cfg := trimEncoderConfigFor(name)
		if cfg.Name != "libx264" {
			t.Fatalf("encoder for %q = %q, want libx264", name, cfg.Name)
		}
	}
}

func TestTrimSharedOutputArgsIncludesSeekKeyframes(t *testing.T) {
	args := trimSharedOutputArgs("clip.mp4")
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-force_key_frames" && args[i+1] == "expr:gte(t,n_forced*2)" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("trim args = %#v, want forced seek keyframes", args)
	}
}
