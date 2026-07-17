package mirror

import (
	"context"
	"os"
	"os/exec"
)

// execCommand allows overriding the execution call during unit tests.
var execCommand = exec.CommandContext

// ImageExists inspects the target registry using 'skopeo inpect' to verify if the image version exists.
//
// It returns true if the image is found, false if it does not exist,
// or an error if the command execution fails for unexpected OS reasons.
func ImageExists(ctx context.Context, image string, credentials string) (bool, error) {
	cmd := execCommand(ctx, "skopeo", "inspect", "--creds", credentials, image)

	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// CopyImage replciates an OCI image from source public registry to target private destination using 'skopeo copy'.
func CopyImage(ctx context.Context, source, destination, credentials string) error {
	cmd := execCommand(ctx, "skopeo", "copy", "--dest-creds", credentials, source, destination)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
