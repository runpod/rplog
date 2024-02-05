package metadata

import (
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gitlab.com/efronlicht/enve"
)

// see https://www.notion.so/runpod/log-meeting-1-31-2024-03a18a6d6ab84b16b5806e81493cc72d for the design.
// metadata is collected once per service start and is immutable.
type Metadata struct{ Checksum, Commit, CommitTimestamp, Env, InstanceID, LanguageVersion, RepoPath, ServiceStart, ServiceName, ServiceVersion string }

var (
	m    Metadata
	once sync.Once
)

func initeager() {
	buildinfo, _ := debug.ReadBuildInfo()
	if buildinfo == nil { // just replace the nil with an empty struct: we'll fill in the defaults later
		buildinfo = &debug.BuildInfo{}
	}
	// get VCS info from the buildinfo
	const NAME, COMMIT, TAG, TIME = 0, 1, 2, 3
	var vcs [4]string
	for _, v := range buildinfo.Settings {
		switch v.Key {
		case "vcs":
			vcs[NAME] = v.Value
		case "vcs.commit":
			vcs[COMMIT] = v.Value
		case "vcs.tag":
			vcs[TAG] = v.Value
		case "vcs.time":
			vcs[TIME] = v.Value
		}
	}
	var b strings.Builder
	for _, v := range vcs {
		if v != "" {
			b.WriteString(v)
			b.WriteByte(' ')
		}
	}
	commit := strings.TrimSpace(b.String())

	instanceID, err := uuid.NewV7()
	if err != nil {
		instanceID = uuid.New()
	}

	m = Metadata{
		Commit:          commit,
		Env:             enve.StringOr("ENV", "dev"),
		InstanceID:      instanceID.String(),
		LanguageVersion: or(buildinfo.GoVersion, "unknown"),
		RepoPath:        or(buildinfo.Path, "unknown"),
		ServiceName:     enve.StringOr("RUNPOD_SERVICE_NAME", "unknown"),
		ServiceStart:    time.Now().UTC().Format(time.RFC3339),
		ServiceVersion:  buildinfo.Main.Version,
	}
}

// Get program metadata by reading environment variables and debug.BuildInfo.
// The first call initializes the package: further calls return the same metadata.
// Do not modify the returned value.
func Get() *Metadata { once.Do(initeager); return &m }

// return a if it's non-zero, otherwise b.
func or[T comparable](a, b T) T {
	var zero T
	if a == zero {
		return b
	}
	return a
}
