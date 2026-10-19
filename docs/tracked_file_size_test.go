package docs_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// maxTrackedBlobBytes caps one file in the tracked tree. Source and test
// fixtures stay well below this; binaries, databases and archives belong in
// release storage rather than Git history.
const maxTrackedBlobBytes = 5 << 20

type trackedBlob struct {
	path string
	oid  string
}

// TestTrackedFileSizeHygiene reads blob sizes from the current Git index.
// Untracked files and history are out of scope: only what the next commit
// would carry is measured.
func TestTrackedFileSizeHygiene(t *testing.T) {
	root := repoRoot(t)
	blobs := trackedBlobs(t, root)
	if len(blobs) == 0 {
		t.Fatal("gate would pass vacuously: git ls-files returned no blobs")
	}

	sizes := trackedBlobSizes(t, root, blobs)
	var findings []string
	for i, blob := range blobs {
		if sizes[i] > maxTrackedBlobBytes {
			findings = append(findings, fmt.Sprintf("%s: %d bytes (%.2f MiB)",
				blob.path, sizes[i], float64(sizes[i])/(1<<20)))
		}
	}
	if len(findings) > 0 {
		t.Fatalf("tracked files exceed 5 MiB (%d bytes):\n%s",
			maxTrackedBlobBytes, strings.Join(findings, "\n"))
	}
}

func trackedBlobs(t *testing.T, root string) []trackedBlob {
	t.Helper()
	out := runGit(t, root, "", "ls-files", "-s", "-z")
	var blobs []trackedBlob
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		meta, path, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			t.Fatalf("unexpected git ls-files record %q", record)
		}
		fields := bytes.Fields(meta)
		if len(fields) != 3 {
			t.Fatalf("unexpected git ls-files metadata %q", meta)
		}
		// Only regular files and symlinks are blobs owned by this tree.
		// Gitlinks and sparse-index directory entries carry no file content.
		switch string(fields[0]) {
		case "100644", "100755", "120000":
		default:
			continue
		}
		blobs = append(blobs, trackedBlob{path: string(path), oid: string(fields[1])})
	}
	return blobs
}

func trackedBlobSizes(t *testing.T, root string, blobs []trackedBlob) []int64 {
	t.Helper()
	var input strings.Builder
	for _, blob := range blobs {
		input.WriteString(blob.oid)
		input.WriteByte('\n')
	}
	out := runGit(t, root, input.String(), "cat-file", "--batch-check")
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(blobs) {
		t.Fatalf("git cat-file returned %d lines for %d blobs", len(lines), len(blobs))
	}

	sizes := make([]int64, len(blobs))
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[1] != "blob" {
			t.Fatalf("unexpected git cat-file line for %s: %q", blobs[i].path, line)
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			t.Fatalf("parse size for %s: %v", blobs[i].path, err)
		}
		sizes[i] = size
	}
	return sizes
}

func runGit(t *testing.T, root, stdin string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return out
}
