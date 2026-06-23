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
