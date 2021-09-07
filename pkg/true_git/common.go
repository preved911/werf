package true_git

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
)

func SetCommandRecordingLiveOutput(ctx context.Context, cmd *exec.Cmd) *bytes.Buffer {
	recorder := &bytes.Buffer{}

	if liveGitOutput {
		cmd.Stdout = io.MultiWriter(recorder, os.Stdout)
		cmd.Stderr = io.MultiWriter(recorder, os.Stderr)
	} else {
		cmd.Stdout = recorder
		cmd.Stderr = recorder
	}

	return recorder
}

func getCommonGitOptions() []string {
	return []string{"-c", "core.autocrlf=false"}
}
