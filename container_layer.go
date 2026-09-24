package systats

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/dhamith93/systats/internal/fileops"
)

// ContainerLayer is the size of a container's writable layer: everything it
// has written to its root filesystem since it started, separate from the
// image it was started from. It's the SIZE column of `docker ps -s`, and
// the answer to "how much disk is this container using" that the root
// mount in Container.Mounts can't give, since that reports the whole
// backing filesystem.
//
// Only measured when SyStats.ContainerLayerSize is set - it means walking
// every file in the layer.
type ContainerLayer struct {
	// Size is the total apparent size of the layer's regular files, each
	// hard-linked file counted once.
	Size  float64 `json:"size"`
	Unit  Unit    `json:"unit"`
	Files uint64  `json:"files"`
	// Path is the layer's directory on the host (overlayfs's upperdir).
	Path string `json:"path"`
	// Available is false when the layer wasn't measured: ContainerLayerSize
	// is off, the root filesystem isn't overlayfs, or the layer couldn't be
	// read. Reading it needs root (or CAP_SYS_PTRACE and
	// CAP_DAC_READ_SEARCH), since it goes through /proc/1/root.
	Available bool `json:"available"`
}

// overlayUpperDir returns the writable layer directory of the overlayfs
// mounted at / in a /proc/<pid>/mounts listing, or "" if the root isn't
// overlayfs.
func overlayUpperDir(mounts string) string {
	for _, line := range strings.Split(mounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[1] != "/" || fields[2] != "overlay" {
			continue
		}
		for _, opt := range strings.Split(fields[3], ",") {
			if strings.HasPrefix(opt, "upperdir=") {
				return unescapeMountField(strings.TrimPrefix(opt, "upperdir="))
			}
		}
	}
	return ""
}

// readContainerLayer measures pid's writable layer. The upperdir path in
// the container's mount table is a host path, so it's resolved through
// /proc/1/root - the host's root as pid 1 sees it. That's / when running
// on the host, and still the host's root from inside a container started
// with --pid=host, with no extra volume mounts needed.
func readContainerLayer(ctx context.Context, procPath string, pid int, unit Unit, bytesPer float64) (ContainerLayer, error) {
	out := ContainerLayer{Unit: unit}
	mounts, err := fileops.ReadFileWithError(path.Join(procPath, strconv.Itoa(pid), "mounts"))
	if err != nil {
		return out, nil
	}
	upper := overlayUpperDir(mounts)
	if upper == "" {
		return out, nil
	}
	out.Path = upper

	size, files, err := layerSize(ctx, path.Join(procPath, "1", "root", upper))
	if err != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, nil // unreadable: Available stays false
	}
	out.Size = float64(size) / bytesPer
	out.Files = files
	out.Available = true
	return out, nil
}

// layerSize sums the apparent size of every regular file under dir, the
// same measure docker uses for a layer's size. Hard links are counted
// once. Whiteouts (the character devices overlayfs uses to mark deleted
// files) and other non-regular files add nothing. Checks ctx between
// entries, since a large layer can take seconds to walk.
func layerSize(ctx context.Context, dir string) (size, files uint64, err error) {
	type inode struct{ dev, ino uint64 }
	seen := map[inode]bool{}

	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err // the layer itself is unreadable
			}
			return nil // a file removed mid-walk, or one subdirectory we can't read
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
			key := inode{uint64(st.Dev), uint64(st.Ino)}
			if seen[key] {
				return nil
			}
			seen[key] = true
		}
		size += uint64(info.Size())
		files++
		return nil
	})
	return size, files, err
}
