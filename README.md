# rplog

rplog is runpod's logging and tracing package. It provides a uniform logging implementation across all 3 of Runpod's major languages: Python, JavaScript, and Go. 

## LOGS
Logs are written to stderr in JSON format. All logs contain a "metadata" field that is a JSON object containing _at least_ the following fields:

| Field | Description | Example |
|-------|-------------| ------- |
| commit | The git commit hash of the code that is running.
| env | The environment in which the code is running.
| language_version | The language (python, javascript, go) and version of the code that is running. | go 1.14.2
| repo_path | The path to the git repository that the code is running from. | runpod/rplog
| instance_id | A unique identifier for the instance of the code that is running. | fac72470-068a-44a3-be0d-a43fd9c7fffd
| service_start | The time at which rplog was initialized. | 2020-06-01T00:00:00Z
| service_version | The version of the code that is running. This should line up with the git tag. | v0.0.1
| service_name | The name of the service that is running. | ai-api





## ENVIRONMENT VARIABLES

| Variable | Description | Default |
|----------|-------------|---------|
| RUNPOD_LOG_LEVEL | The minimum log level to display. | INFO |
| ENV | The environment in which the code is running. | unknown |


## Go

rplog.Logger() returns a handle to the logger. Additionally, the `rplog` 
```go

Then, you can use the `rplog` package to log messages:

```go