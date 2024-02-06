# buildmeta

This is a simple tool to generate environment-specific metadata for runpod projects. Use it as part of your deployment process to inject the build-time metadata into your application.

## Usage:
```sh
buildmeta -env prod|dev -service <service-name> -format env|json [-dir <git-directory>] [-revision <git-revision>]
```

## Help:
```sh
buildmeta -help
```
```
Usage of /tmp/go-build3654420915/b001/exe/buildmeta:
  -dir string
        optional: directory to run git commands in (default ".")
  -env string
        mandatory: the environment to build for. usually 'dev' or 'prod'
  -format string
        output format: env, json, python, javascript
  -revision string
        optional: git revision to check (default "HEAD")
  -service string
        mandatory: the name of the service
```

## Examples

### JSON
#### IN:
```sh
buildmeta -env prod -service log-example -format json
```
#### OUT:
```json
{
  "VCS": {
    "Name": "git",
    "Commit": "86b4b04f252fbe9193aeb218dce8f33ba929fd06",
    "Tag": "v1.9.1",
    "Time": "2024-02-03T15:20:42Z"
  },
  "Env": "prod",
  "Service": "testservice"
}
```
### SHELL
#### IN:
```sh
buildmeta -env dev -service log-example -format env
```
#### OUT:
```sh
export "RUNPOD_SERVICE_NAME"="testservice"
export "RUNPOD_ENV"="dev"
export "RUNPOD_SERVICE_VCS_COMMIT"="86b4b04f252fbe9193aeb218dce8f33ba929fd06"
export "RUNPOD_SERVICE_VCS_TAG"="v1.9.1"
export "RUNPOD_SERVICE_VCS_TIME"="2024-02-03T15:20:42Z"
export "RUNPOD_SERVICE_VCS_NAME"="git"
```

buildmeta -env dev -service log-example -format env

```sh

## Usage

```bash
buildmeta -env prod -service ai-api > metadata.env
chmod a+x metadata.env
```