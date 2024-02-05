
import uuid7
import time
def as_rfc3339(dt: datetime.datetime) -> str:
    return dt.astimezone(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-4]+"Z"
metadata = {
	"instance_id": str(uuid7.uuid7()),
	"service_name": "ai-api",
	"service_env": "prod",
	"service_vcs_commit": "ec205e4edf36d030c7728041f1362df8e36bdba8",
	"service_vcs_tag": "",
	"service_vcs_time": as_rfc3339(datetime.datetime.fromisoformat("2024-02-05T16:16:13Z")),
	"service_vcs_name": "git",
}
