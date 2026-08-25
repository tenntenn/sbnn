package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/server"
)

// The answer to a diff is the parsed diff, several times the size of the patch
// that produced it: a 32MB diff comes back as about 128MB of JSON. Reading
// that through a flat io.LimitReader truncated the body and left the decoder
// to blame the JSON, so a diff the server had just accepted was reported to
// the sender as "unexpected EOF" -- the very message #155 is about, arriving
// from the other end of the wire.
func TestResponseOverrunIsReportedAsAnOverrun(t *testing.T) {
	const pad = 8 << 10

	tests := []struct {
		name    string
		body    string
		limit   int64
		wantErr string
	}{
		{
			name:  "an answer under the limit is decoded",
			body:  `{"app":"sbnn"}`,
			limit: 1 << 20,
		},
		{
			// Scaling the limit below the body, rather than writing a body
			// past the real 256MB default, keeps this to kilobytes.
			name:    "an answer past the limit names the limit, not the JSON",
			body:    `{"app":"sbnn","pad":"` + strings.Repeat("x", pad) + `"}`,
			limit:   pad / 2,
			wantErr: "the answer is larger than",
		},
		{
			name:    "genuinely malformed JSON is still reported as malformed",
			body:    `{"app":"sbn`,
			limit:   1 << 20,
			wantErr: "unexpected EOF",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			c := New(strings.TrimPrefix(srv.URL, "http://"), 5*time.Second)
			c.MaxResponse = tt.limit

			var st server.Status
			err := c.do(context.Background(), http.MethodGet, srv.URL+"/_/api/status", nil, &st)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("do() failed: %v", err)
			case tt.wantErr == "":
				return
			case err == nil:
				t.Fatalf("do() succeeded, want an error naming %q", tt.wantErr)
			case !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("do() = %v, want an error naming %q", err, tt.wantErr)
			}
		})
	}
}

// The default has to cover the answer to the largest diff sbnn accepts, or the
// case above is what every 32MB diff runs into. 128MB was the measurement for
// a diff at the limit; the default is checked against a multiple of it.
func TestDefaultResponseLimitCoversTheLargestDiff(t *testing.T) {
	const measured = 128 << 20
	if maxResponseSize < 2*measured {
		t.Errorf("maxResponseSize = %dMB, want at least %dMB: a diff at the "+
			"limit answered with %dMB when this was measured",
			maxResponseSize>>20, (2*measured)>>20, measured>>20)
	}
}
