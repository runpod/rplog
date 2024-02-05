# rplog

rplog is runpod's logging and tracing package. It provides a uniform logging implementation across all 3 of Runpod's major languages: Python, JavaScript, and Go. 


| Language | Subdirectory |
|----------|--------------|
| Python | [./py](./py) |
| JavaScript | [./js](./js) |
| Go | [./go](./go) |


The following documentation covers language-independent aspects of rplog. For language-specific documentation, see the README in the appropriate subdirectory.

## Overview: Logs
Logs are written to stderr in JSON format. All logs contain a "metadata" field that is a JSON object containing _at least_ the following fields:

| Field | Description | Example |
|-------|-------------| ------- |
| commit | The (shortened) git commit hash of the code that is running, as though with `git rev-parse --short HEAD` | 86b4b04
| env | The environment in which the code is running. | prod
| language_version | The language (python, javascript, go) and version of the code that is running. | go 1.14.2
| repo_path | The path to the git repository that the code is running from. | runpod/rplog
| instance_id | A unique identifier for the instance of the code that is running. | fac72470-068a-44a3-be0d-a43fd9c7fffd
| service_start | The time at which rplog was initialized. | 2020-06-01T00:00:00Z
| service_version | The version of the code that is running. This should line up with the git tag. | v0.0.1
| service_name | The name of the service that is running. | ai-api


### Log Levels

We provide 5 log levels:

| Level | Description | Example |
|-------|-------------| ------- |
| DEBUG | Verbose messages, disabled by default in both dev and prod. | "GET /api/v1/health" OK"
| INFO | Indications of normal operation, enabled by default in dev, disabled by default in prod. | "Starting server on port 8080"
| WARN | Indications of possible issues, missing but not critical data, etc. Enabled by default in both dev and prod. | "network storage cache disabled"
| ERROR | Indications of critical issues, missing critical data, etc. Enabled by default in both dev and prod. | "failed to connect to database"


Generally speaking, `WARN` is to be avoided. If you're logging a warning, you should probably be logging an `ERROR` instead. `DEBUG` should be used sparingly, and `INFO` should be used for anything that is not an error.


### Populating your logs with metadata

See the [buildmeta README](./go/cmd/README.md) for information on how to populate your logs with metadata.

### Logs: Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| RUNPOD_LOG_LEVEL | The minimum log level to display. | INFO |
| ENV | The environment in which the code is running. | unknown |
| RUNPOD_SERVICE_NAME | The name of the service that is running. | unknown |
| RUNPOD_SERVICE_VERSION | The version of the service that is running. | unknown |
| RUNPOD_SERVICE_COMMIT_HASH | The git commit hash of the code that is running. | unknown |



## Tracing
Traces consist of a `request_id`, a `trace_id`, and the `trace_start` timestamp. A `trace_id` should begin when an "event" starts in our system (i.e, a customer request comes in, a cron job starts, etc) and travels across services. A `request_id` is a unique identifier for a single request: i.e, within the bounds of a single service. A trace may outlive a request, but a request will always be part of a trace.

Traces are carried across service boundaries in HTTP requests via the `X-Request-Id` and `X-Trace-Id` headers. The start time of the trace is carried in the `X-Trace-Start` header, in RFC3339 format (i.e, `2020-06-01T00:00:00Z`). **All times should always be UTC: runpod is a global organization**.

