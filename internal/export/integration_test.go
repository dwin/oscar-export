package exporter

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExportsMatchOscarFixtures(t *testing.T) {
	dataDir := os.Getenv("OSCAR_DATA_DIR")
	exportDir := os.Getenv("OSCAR_EXPORT_DIR")
	if dataDir == "" || exportDir == "" {
		t.Skip("set OSCAR_DATA_DIR and OSCAR_EXPORT_DIR to run integration tests")
	}

	profileUser := os.Getenv("OSCAR_PROFILE_USER")
	if profileUser == "" {
		profileUser = "D"
	}

	cases := []struct {
		name string
		mode Mode
		from string
		to   string
		want string
	}{
		{
			name: "summary",
			mode: ModeSummary,
			from: "2026-03-03",
			to:   "2026-03-17",
			want: filepath.Join(exportDir, "OSCAR_D_Summary_2026-03-03_2026-03-17.csv"),
		},
		{
			name: "sessions",
			mode: ModeSessions,
			from: "2026-03-03",
			to:   "2026-03-17",
			want: filepath.Join(exportDir, "OSCAR_D_Sessions_2026-03-03_2026-03-17.csv"),
		},
		{
			name: "details",
			mode: ModeDetails,
			from: "2026-02-20",
			to:   "2026-03-06",
			want: filepath.Join(exportDir, "OSCAR_D_Details_2026-02-20_2026-03-06.csv"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, err := time.Parse("2006-01-02", tc.from)
			if err != nil {
				t.Fatalf("time.Parse(from) error = %v", err)
			}
			to, err := time.Parse("2006-01-02", tc.to)
			if err != nil {
				t.Fatalf("time.Parse(to) error = %v", err)
			}

			outPath := filepath.Join(t.TempDir(), tc.name+".csv")
			if err := Run(t.Context(), Config{
				Mode:        tc.mode,
				Root:        dataDir,
				ProfileUser: profileUser,
				From:        from,
				To:          to,
				Out:         outPath,
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			wantBytes, err := os.ReadFile(tc.want)
			if err != nil {
				t.Fatalf("ReadFile(want) error = %v", err)
			}
			gotBytes, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("ReadFile(got) error = %v", err)
			}

			if !bytes.Equal(gotBytes, wantBytes) {
				t.Fatalf("output mismatch\n%s", firstDifference(gotBytes, wantBytes))
			}
		})
	}
}

func firstDifference(got, want []byte) string {
	limit := len(got)
	if len(want) < limit {
		limit = len(want)
	}
	for i := 0; i < limit; i++ {
		if got[i] != want[i] {
			return "first differing byte found in fixture comparison"
		}
	}
	if len(got) != len(want) {
		return "file lengths differ"
	}
	return "unknown difference"
}
