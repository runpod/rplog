// The buildmeta command generates a file which exports the metadata of the current build as environment variables (i.e, a shell script).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func run(cmd string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	exe := exec.CommandContext(ctx, cmd, args...)
	exe.Stderr = os.Stderr
	b, err := exe.Output()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(b))
}

type Output struct {
	VCS          struct{ Name, Commit, Tag, Time string }
	Env, Service string
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("buildmeta: ")
	var o Output
	var revision, dir, outputFormat string
	{ // parse & validate flags
		flag.StringVar(&dir, "dir", ".", "optional: directory to run git commands in")
		flag.StringVar(&o.Env, "env", "", "mandatory: the environment to build for. usually 'dev' or 'prod'")
		flag.StringVar(&o.Service, "service", "", "mandatory: the name of the service")
		flag.StringVar(&outputFormat, "format", "env", "output format: env or json")
		flag.StringVar(&revision, "revision", "HEAD", "optional: git revision to check")
		flag.Parse()
		switch {
		case o.Env == "":
			flag.Usage()
			log.Fatal("missing required flag -env")
		case o.Service == "":
			flag.Usage()
			log.Fatal("missing required flag -service")
		case o.Service == "":
			o.Service = filepath.Base(dir)
		}
	}

	dir, err := filepath.Abs(dir)
	if err != nil {
		panic(fmt.Errorf("could not get absolute path of -dir: %w", err))
	}
	{ // lookup VCS (git) info
		os.Chdir(dir)

		o.VCS.Name = "git"
		o.VCS.Commit = run("git", "rev-parse", revision)
		o.VCS.Tag = run("git", "tag", "--points-at", revision)
		const layout = "2024-02-03 07:20:42 -0800"
		commitOffset, err := strconv.Atoi(run("git", "show", "-s", "--format=%at", revision))
		if err != nil {
			panic(fmt.Errorf("could not parse git commit time: %w", err))
		}

		o.VCS.Time = time.Unix(int64(commitOffset), 0).UTC().Format(time.RFC3339)
	}
	// print output to stdout.
	switch outputFormat {
	case "env":
		for _, v := range [...]struct{ key, val string }{
			{"RUNPOD_SERVICE_NAME", o.Service},
			{"RUNPOD_ENV", o.Env},
			{"RUNPOD_SERVICE_VCS_COMMIT", o.VCS.Commit},
			{"RUNPOD_SERVICE_VCS_TAG", o.VCS.Tag},
			{"RUNPOD_SERVICE_VCS_TIME", o.VCS.Time},
			{"RUNPOD_SERVICE_VCS_NAME", o.VCS.Name},
		} {
			if v.val == "" {
				continue
			}
			fmt.Printf("export %q=%q\n", v.key, strings.TrimSpace(v.val))
		}
	case "json":
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		e.Encode(o)
	}
}
