package jeepity

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

type OggMp3Converter struct {
	inputPath  string
	outputPath string
}

func NewOggMp3Converter(input, output string) *OggMp3Converter {
	return &OggMp3Converter{
		inputPath:  input,
		outputPath: output,
	}
}

func (f *OggMp3Converter) Command(ctx context.Context) *exec.Cmd {
	args := []string{
		"-progress", "pipe:1", // print key-value progress information to stderr
		"-stats_period", "1", // period at which encoding progress/statistics are updated
		"-nostats", // do not print encoding progress/statistics
		"-y",       // overwrite output files without asking
		"-i", f.inputPath,
		"-acodec", "libmp3lame",
		"-qscale:a", "4",
		f.outputPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Env = os.Environ()

	// Run ffmpeg in a separate process group so that we can kill it gracefully.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}

	return cmd
}
