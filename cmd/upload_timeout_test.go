package cmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/server"
)

// A diff at the size limit takes the server seconds to accept: it reads the
// body, unescapes the JSON around the diff and parses the diff. Measured on
// the real binary, `sbnn` took about 20s end to end on a 32MB diff, so the
// call has to be given more than the five seconds the small calls get --
// otherwise the diff the server accepted comes back to the sender as a
// timeout.
func TestUploadTimeoutGrowsWithTheDiff(t *testing.T) {
	tests := []struct {
		name string
		size int
		want time.Duration
	}{
		{name: "no diff at all keeps the plain timeout", size: 0, want: 5 * time.Second},
		{name: "a negative size cannot happen but is still bounded", size: -1, want: 5 * time.Second},
		{name: "a one byte diff still buys a whole megabyte of time", size: 1, want: 7 * time.Second},
		{name: "exactly one megabyte", size: 1 << 20, want: 7 * time.Second},
		{name: "one byte over a megabyte rounds up", size: 1<<20 + 1, want: 9 * time.Second},
		{name: "a diff at the limit", size: server.MaxDiffSize, want: 69 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uploadTimeout(tt.size); got != tt.want {
				t.Errorf("uploadTimeout(%d) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

// The number that matters is not the formula but the outcome: the largest
// diff sbnn accepts must be given more time than the round trip takes. 20s was
// the measurement; the allowance is checked against a multiple of it so that a
// machine several times slower than the one it was measured on still gets an
// answer rather than a timeout.
func TestUploadTimeoutCoversTheLargestDiff(t *testing.T) {
	const measured = 20 * time.Second
	got := uploadTimeout(server.MaxDiffSize)
	if got < 3*measured {
		t.Errorf("uploadTimeout(%d) = %v, want at least %v: a diff at the limit "+
			"took %v to accept when this was measured", server.MaxDiffSize, got, 3*measured, measured)
	}
}

// A timeout that is computed and then not used is no timeout at all: the
// mutation that put the flat five seconds back in runRoot left every test
// above green. The client that carries the diff has to be the one built with
// the allowance, so that is what is checked -- in the source, because runRoot
// builds the client itself and there is nothing to ask afterwards.
func TestTheDiffIsSentWithTheGrownTimeout(t *testing.T) {
	b, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	const want = "client.New(addr(), uploadTimeout(len(content)))"
	if !strings.Contains(string(b), want) {
		t.Errorf("cmd/root.go does not build its client with %s; a diff at the "+
			"size limit takes about 20s to send and would time out", want)
	}
}
