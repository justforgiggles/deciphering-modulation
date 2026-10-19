package demodulation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteVerifiedPayloads exclusively creates a result directory. Files are
// written only after decoding; any write failure removes this new directory.
func WriteVerifiedPayloads(ctx context.Context, directory string, frames []VerifiedFrame) (err error) {
	if len(frames) == 0 {
		return errors.New("no verified frames to write")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Mkdir(directory, 0755); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(directory))
		}
	}()
	for i, frame := range frames {
		if err = ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(directory, fmt.Sprintf("frame-%03d.bin", i+1))
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if openErr != nil {
			return openErr
		}
		_, writeErr := f.Write(frame.Payload)
		err = errors.Join(writeErr, f.Close())
		if err != nil {
			return err
		}
	}
	return nil
}
