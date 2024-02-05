""" a friendly json encoder that can handle dataclasses, enums, datetimes, bytes, and sets."""
import json
import dataclasses
import enum
from datetime import datetime

class Encoder(json.JSONEncoder):
    """ a friendly json encoder that can handle dataclasses, enums, datetimes, bytes, and sets."""
    def default(self, obj):
        if dataclasses.is_dataclass(obj):
            return dataclasses.asdict(obj)
        if isinstance(obj, enum.Enum):
            return obj.name
        if isinstance(obj, datetime):
            return obj.astimezone(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-4]+"Z"
        if isinstance(obj, bytes):
            return obj.decode("utf-8")
        if isinstance(obj, set):
            return list(obj)
        if isinstance(obj, Exception):
            return str(obj)
        
        
        return super().default(obj)