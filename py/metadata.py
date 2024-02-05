import datetime
import json
import typing
from dataclasses import dataclass
from os import environ

import uuid7

T = typing.TypeVar('T')


def as_rfc3339(dt: datetime.datetime) -> str:
    return dt.astimezone(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-4]+"Z"

def from_jsons(jsons: str) -> dict[str, str]:
    """load metadata from a JSON string, injecting the instance_id."""
    meta = json.loads(jsons)
    meta["instance_id"] = INSTANCE_ID
    return meta


# per-execution metadata
INSTANCE_ID = uuid7.uuid7()
def from_json_file(filename: str) -> dict[str, str]:
    """load metadata from a JSON file, injecting the instance_id."""
    with open(filename, "r") as f:
        meta = json.load(f)
    meta["instance_id"] = INSTANCE_ID
    return meta


def from_env(filename: str) -> dict[str, str]:
    meta = {"instance_id": INSTANCE_ID}
    for env, k in [
        ("RUNPORT_SERVICE_NAME", "service_name"),
        ("RUNPORT_SERVICE_VERSION", "service_version"),
        ("ENV", "env"),
        ("RUNPOD_SERVICE_VCS_COMMIT", "vcs_commit"),
        ("RUNPORT_SERVICE_VCS_TAG", "vcs_tag"),
        ("RUNPORT_SERVICE_VCS_TIME", "vcs_time"),
        ("RUNPOD_SERVICE_VCS_NAME", "vcs_name"),
    ]:
        if env in environ:
            meta[k] = environ[env]
    return meta
