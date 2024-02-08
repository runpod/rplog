// The buildmeta command generates a file which exports the metadata of the current build in a variety of formats.
// Usage: buildmeta [-dir <dir>] -env <env> -service <service> [-format <format>] [-revision <revision>]
// Where:
//
//	-dir: optional: directory to run git commands in (default ".")
//	-env: mandatory: the environment to build for. usually 'dev' or 'prod'
//	-service: mandatory: the name of the service
//	-format: output format: env, json, python, javascript (default "env")
//	-revision: optional: git revision to check (default "HEAD")
//
// The output is written to stdout. Use standard shell redirection to save it to a file: e.g, buildmeta -env dev -service myservice -format python > metadata.py
package main

import (
	"context"
	_ "embed"
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

//go:embed uuid7.js
var uuid7js []byte

//go:embed uuid7.py
var uuid7py []byte

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
	VCS          struct{ Name, Commit, Tag, Time string } // Version Control System, e.g. git
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
		flag.StringVar(&outputFormat, "format", "env", "output format: env, json, python, javascript")
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
		commitOffset, err := strconv.Atoi(run("git", "show", "-s", "--format=%at", revision))
		if err != nil {
			panic(fmt.Errorf("could not parse git commit time: %w", err))
		}

		o.VCS.Time = time.Unix(int64(commitOffset), 0).UTC().Format(time.RFC3339)
	}
	// print output to stdout.
	switch strings.ToLower(outputFormat) {
	case "env", "sh":
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
	case "rust", "rs":
		// no need to inline the uuid7.rs file. just export it as a rust module.
		fmt.Printf(`SERVICE: &str = %q;
ENV: &str = %q;
VCS_COMMIT: &str = %q;
VCS_TAG: &str = %q;
VCS_TIME: &str = %q;
VCS_NAME: &str = %q;
`, o.Service, o.Env, o.VCS.Commit, o.VCS.Tag, o.VCS.Time, o.VCS.Name)
	case "json":
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		e.Encode(o)
	case "python", "py":
		// first, we inline the uuid7.py file, then we print the metadata as a python dictionary.
		const format = `%s #
import datetime
def as_rfc3339(dt: datetime.datetime) -> str:
    return dt.astimezone(datetime.timezone.utc).strftime("%%Y-%%m-%%dT%%H:%%M:%%S.%%f")[:-4]+"Z"
metadata = {
	"instance_id": str(uuid7()),
	"service_name": %q,
	"service_env": %q,
	"service_vcs_commit": %q,
	"service_vcs_tag": %q,
	"service_vcs_time": as_rfc3339(datetime.datetime.fromisoformat(%q)),
	"service_vcs_name": %q,
}
`
		fmt.Printf(format, uuid7py, o.Service, o.Env, o.VCS.Commit, o.VCS.Tag, o.VCS.Time, o.VCS.Name)
	case "js", "javascript":
		// first, we inline the uuid7.js file, then we print the metadata as a javascript object.
		const format = `%s
import { uuidv7 } from "uuidv7"
export const metadata = {
	instance_id: uuidv7(),
	service_name: %q,
	service_env: %q,
	service_vcs_commit: %q,
	service_vcs_tag: %q,
	service_vcs_time: (new Date(%q)).toISOString(),
	service_vcs_name: %q,
}
`
		fmt.Printf(format, uuid7js, o.Service, o.Env, o.VCS.Commit, o.VCS.Tag, o.VCS.Time, o.VCS.Name)
	default:
		log.Fatalf("unknown output format %q", outputFormat)
	}
}
